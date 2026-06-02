package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// s3Store wraps a MinIO/S3 client plus the local HLS folder it mirrors.
type s3Store struct {
	client  *minio.Client // talks to the internal endpoint (upload, fetch)
	presign *minio.Client // signs URLs against the browser-reachable endpoint
	bucket  string
	dir     string // local folder ffmpeg writes HLS into
}

func newS3Store() (*s3Store, error) {
	endpoint := getenv("MINIO_ENDPOINT", "minio:9000")
	// Public endpoint the BROWSER can reach. The signature embeds this host,
	// so presigned URLs must be signed against it (not the docker-internal name).
	publicEndpoint := getenv("MINIO_PUBLIC_ENDPOINT", "localhost:9000")
	access := getenv("MINIO_ACCESS_KEY", "minioadmin")
	secret := getenv("MINIO_SECRET_KEY", "minioadmin")
	bucket := getenv("MINIO_BUCKET", "hls")
	dir := getenv("HLS_OUT_DIR", "/hls_out")

	// Region must be set explicitly: otherwise PresignedGetObject does a
	// GetBucketLocation network call against the public endpoint, which from
	// inside the container resolves to container-localhost and fails.
	region := getenv("MINIO_REGION", "us-east-1")

	creds := credentials.NewStaticV4(access, secret, "")
	client, err := minio.New(endpoint, &minio.Options{Creds: creds, Secure: false, Region: region})
	if err != nil {
		return nil, err
	}
	// Presign client never makes network calls — it only computes signatures.
	presign, err := minio.New(publicEndpoint, &minio.Options{Creds: creds, Secure: false, Region: region})
	if err != nil {
		return nil, err
	}
	return &s3Store{client: client, presign: presign, bucket: bucket, dir: dir}, nil
}

// presignedURL returns a temporary signed GET URL the browser can use directly.
func (s *s3Store) presignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	u, err := s.presign.PresignedGetObject(ctx, s.bucket, key, expiry, nil)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// rewrittenPlaylist fetches an HLS media playlist and replaces each relative
// segment name with its own presigned URL, so the browser pulls segments
// straight from S3 (the app only serves this small playlist).
func (s *s3Store) rewrittenPlaylist(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, err
	}

	dir := path.Dir(key) // e.g. live/demo
	lines := strings.Split(string(data), "\n")
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "#") {
			continue // comment/tag or blank — leave as-is
		}
		signed, err := s.presignedURL(ctx, path.Join(dir, t), time.Hour)
		if err != nil {
			return nil, err
		}
		lines[i] = signed
	}
	return []byte(strings.Join(lines, "\n")), nil
}

// contentType picks the right MIME type for HLS files.
func contentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".ts":
		return "video/mp2t"
	case ".mp4", ".m4s":
		return "video/mp4"
	default:
		return "application/octet-stream"
	}
}

// syncLoop watches the local HLS folder and uploads new/changed files to S3.
// Simple poll-based sync (every second) — robust and easy to reason about for a POC.
//
// Ordering matters: a playlist (.m3u8) must never reach S3 before the segments it
// references, or a live viewer could fetch a playlist naming a segment that isn't
// in S3 yet (403/404 stall). So each tick uploads .ts segments first, .m3u8 last.
func (s *s3Store) syncLoop(ctx context.Context) {
	seen := make(map[string]int64) // path -> last modtime uploaded

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 1. Collect files that are new or changed since the last upload.
			var changed []string
			filepath.Walk(s.dir, func(p string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				if prev, ok := seen[p]; ok && prev == info.ModTime().UnixNano() {
					return nil
				}
				changed = append(changed, p)
				return nil
			})

			// 2. Upload segments before playlists (playlists sort last).
			sort.Slice(changed, func(i, j int) bool {
				iPL := strings.HasSuffix(changed[i], ".m3u8")
				jPL := strings.HasSuffix(changed[j], ".m3u8")
				if iPL != jPL {
					return !iPL // non-playlist first
				}
				return changed[i] < changed[j]
			})

			for _, p := range changed {
				info, err := os.Stat(p)
				if err != nil {
					continue
				}
				rel, err := filepath.Rel(s.dir, p)
				if err != nil {
					continue
				}
				key := filepath.ToSlash(rel)
				if _, err := s.client.FPutObject(ctx, s.bucket, key, p,
					minio.PutObjectOptions{ContentType: contentType(p)}); err != nil {
					log.Printf("s3 upload %s: %v", key, err)
					continue
				}
				seen[p] = info.ModTime().UnixNano()
				log.Printf("s3 uploaded %s -> %s/%s", rel, s.bucket, key)
			}
		}
	}
}

// serveObject streams an object straight from S3 (used by the VOD player).
// Relative segment URLs in the playlist resolve back through this same handler.
func (s *s3Store) serveObject(w http.ResponseWriter, r *http.Request, key string) {
	obj, err := s.client.GetObject(r.Context(), s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer obj.Close()

	info, err := obj.Stat()
	if err != nil {
		http.Error(w, "not found in S3: "+key, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType(key))
	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.ServeContent(w, r, key, info.LastModified, obj)
}

// listObjects returns object keys under a prefix (for the VOD index page).
func (s *s3Store) listObjects(ctx context.Context, prefix string) []string {
	var keys []string
	for o := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix: prefix, Recursive: true,
	}) {
		if o.Err == nil {
			keys = append(keys, o.Key)
		}
	}
	return keys
}
