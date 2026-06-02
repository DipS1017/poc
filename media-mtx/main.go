package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	mediamtxAPI := getenv("MEDIAMTX_API", "http://mediamtx:9997")
	hlsBase := getenv("HLS_BASE", "http://localhost:8888")
	webrtcBase := getenv("WEBRTC_BASE", "http://localhost:8889")

	// S3/MinIO: mirror ffmpeg's HLS output into object storage.
	store, err := newS3Store()
	if err != nil {
		log.Fatalf("s3 init: %v", err)
	}
	go store.syncLoop(context.Background())
	log.Printf("s3 sync: %s -> bucket %q (endpoint %s)", store.dir, store.bucket, getenv("MINIO_ENDPOINT", "minio:9000"))

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// List active streams (proxies MediaMTX control API)
	mux.HandleFunc("/paths", func(w http.ResponseWriter, r *http.Request) {
		proxyToMediaMTX(w, mediamtxAPI+"/v3/paths/list")
	})

	// Broadcast page: publish webcam from the browser via WebRTC/WHIP
	mux.HandleFunc("/broadcast", func(w http.ResponseWriter, r *http.Request) {
		stream := r.URL.Query().Get("stream")
		if stream == "" {
			stream = "live/webcam"
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, broadcastHTML, stream, webrtcBase, stream, stream)
	})

	// Player page: /watch?stream=mystream
	mux.HandleFunc("/watch", func(w http.ResponseWriter, r *http.Request) {
		stream := r.URL.Query().Get("stream")
		if stream == "" {
			stream = "live/mystream"
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, playerHTML, stream, hlsBase, stream)
	})

	// VOD index: list what's stored in S3
	mux.HandleFunc("/vod", func(w http.ResponseWriter, r *http.Request) {
		keys := store.listObjects(r.Context(), "")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<!doctype html><meta charset=utf-8><title>VOD in S3</title>"+
			"<body style='font-family:system-ui;max-width:760px;margin:40px auto'>"+
			"<h1>📦 Stored in MinIO (S3)</h1>")
		if len(keys) == 0 {
			fmt.Fprint(w, "<p>Bucket is empty — publish a stream first.</p>")
		}
		playlists := map[string]bool{}
		for _, k := range keys {
			if strings.HasSuffix(k, "index.m3u8") {
				dir := strings.TrimSuffix(k, "/index.m3u8")
				playlists[dir] = true
			}
		}
		for dir := range playlists {
			fmt.Fprintf(w, "<p>▶ <a href='/vodwatch?stream=%s'>%s</a></p>", dir, dir)
		}
		fmt.Fprintf(w, "<hr><p>%d objects total:</p><pre>", len(keys))
		for _, k := range keys {
			fmt.Fprintf(w, "%s\n", k)
		}
		fmt.Fprint(w, "</pre></body>")
	})

	// VOD playback (proxy mode): stream every object through the app.
	mux.HandleFunc("/vodhls/", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/vodhls/")
		store.serveObject(w, r, key)
	})

	// VOD playback (presigned mode): app returns a rewritten playlist whose
	// segment URLs are presigned — the browser fetches segments DIRECTLY from S3.
	mux.HandleFunc("/vodplay/", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/vodplay/")
		if !strings.HasSuffix(key, ".m3u8") {
			http.Error(w, "only playlists are served here; segments come straight from S3", http.StatusBadRequest)
			return
		}
		pl, err := store.rewrittenPlaylist(r.Context(), key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Write(pl)
	})

	// VOD player page — uses presigned playback by default
	mux.HandleFunc("/vodwatch", func(w http.ResponseWriter, r *http.Request) {
		stream := r.URL.Query().Get("stream")
		if stream == "" {
			stream = "live/webcam"
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// playlist via app, segments via presigned direct-to-S3 URLs
		fmt.Fprintf(w, playerHTML, stream+" (presigned S3)", "/vodplay", stream)
	})

	// Landing page with instructions
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, indexHTML)
	})

	addr := ":8080"
	log.Printf("listening on %s | mediamtx=%s | hls=%s", addr, mediamtxAPI, hlsBase)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func proxyToMediaMTX(w http.ResponseWriter, url string) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

const indexHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>MediaMTX POC</title>
<style>body{font-family:system-ui;max-width:760px;margin:40px auto;padding:0 16px;line-height:1.6}
code{background:#f0f0f0;padding:2px 6px;border-radius:4px}
pre{background:#1e1e1e;color:#eee;padding:14px;border-radius:8px;overflow-x:auto}</style>
</head><body>
<h1>🎥 MediaMTX livestream POC</h1>
<p>Flow: <b>publish → MediaMTX (ingest) → ffmpeg remux → chunks → S3 → browser plays from S3</b></p>

<h2>📹 Stream from your browser (no OBS)</h2>
<p>👉 <a href="/broadcast?stream=live/webcam">Open broadcaster (webcam → WebRTC)</a></p>

<h2>1. Publish a stream</h2>
<p>OBS: Settings → Stream → Custom, Server <code>rtmp://localhost:1935/live</code>, Stream Key <code>mystream</code>.</p>
<p>Or test with ffmpeg (sends H264 so MediaMTX can remux without re-encoding):</p>
<pre>ffmpeg -re -f lavfi -i testsrc=size=1280x720:rate=30 \
       -f lavfi -i sine=frequency=440 \
       -c:v libx264 -preset veryfast -tune zerolatency \
       -c:a aac -f flv rtmp://localhost:1935/live/mystream</pre>

<h2>2. Watch — streamed from the S3 chunks ⭐</h2>
<p>This is the primary path: chunks are pulled straight from S3 via presigned URLs.</p>
<p>👉 <a href="/vodwatch?stream=live/mystream">Play "live/mystream" from S3</a></p>
<p>👉 <a href="/vod">Browse everything stored in S3</a> &nbsp;|&nbsp; <a href="http://localhost:9001" target="_blank">MinIO console</a> (minioadmin/minioadmin)</p>

<h2>3. (Debug) Live edge, direct from MediaMTX</h2>
<p>Lower latency, bypasses S3 — handy for debugging only.</p>
<p>👉 <a href="/watch?stream=live/mystream">Live HLS from MediaMTX :8888</a> &nbsp;|&nbsp; <a href="/paths">GET /paths</a></p>
</body></html>`

// args: stream(title), webrtcBase, stream(whip path), stream(watch link)
const broadcastHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>Broadcast %[1]s</title>
<style>body{font-family:system-ui;background:#111;color:#eee;text-align:center;margin:0;padding:24px}
video{width:90%%;max-width:720px;background:#000;border-radius:8px;transform:scaleX(-1)}
button{font-size:16px;padding:10px 22px;border-radius:8px;border:0;cursor:pointer;margin:8px}
#go{background:#22c55e;color:#fff}#stop{background:#ef4444;color:#fff}
#status{margin-top:12px;font-family:monospace}a{color:#6cf}</style>
</head><body>
<h2>📹 Broadcasting: %[1]s</h2>
<video id="preview" autoplay muted playsinline></video>
<div>
  <button id="go">● Go Live</button>
  <button id="stop" disabled>■ Stop</button>
</div>
<div id="status">idle</div>
<p>Watch link to share: <a id="watch" href="/watch?stream=%[4]s">/watch?stream=%[4]s</a></p>
<script>
const whipURL = "%[2]s/%[3]s/whip";
const status = document.getElementById("status");
let pc, stream;

document.getElementById("go").onclick = async () => {
  try {
    status.textContent = "requesting camera...";
    stream = await navigator.mediaDevices.getUserMedia({video:true, audio:true});
    document.getElementById("preview").srcObject = stream;

    pc = new RTCPeerConnection();
    stream.getTracks().forEach(t => pc.addTrack(t, stream));

    // HLS can't carry VP8/VP9 — force H264 so the video survives into HLS.
    const caps = RTCRtpSender.getCapabilities("video");
    const h264 = caps ? caps.codecs.filter(c => c.mimeType.toLowerCase() === "video/h264") : [];
    if (h264.length) {
      for (const tr of pc.getTransceivers()) {
        if (tr.sender && tr.sender.track && tr.sender.track.kind === "video") {
          tr.setCodecPreferences(h264);
        }
      }
    } else {
      status.textContent = "warning: browser has no H264 encoder — HLS will be audio-only";
    }

    const offer = await pc.createOffer();
    await pc.setLocalDescription(offer);
    // wait for ICE gathering to complete (non-trickle WHIP)
    await new Promise(res => {
      if (pc.iceGatheringState === "complete") return res();
      pc.addEventListener("icegatheringstatechange", () => {
        if (pc.iceGatheringState === "complete") res();
      });
    });

    status.textContent = "connecting (WHIP)...";
    const resp = await fetch(whipURL, {
      method: "POST",
      headers: {"Content-Type": "application/sdp"},
      body: pc.localDescription.sdp
    });
    if (!resp.ok) throw new Error("WHIP " + resp.status + " " + await resp.text());
    const answer = await resp.text();
    await pc.setRemoteDescription({type:"answer", sdp:answer});

    pc.onconnectionstatechange = () => status.textContent = "● " + pc.connectionState;
    document.getElementById("go").disabled = true;
    document.getElementById("stop").disabled = false;
  } catch (e) {
    status.textContent = "error: " + e.message;
    console.error(e);
  }
};

document.getElementById("stop").onclick = () => {
  if (pc) pc.close();
  if (stream) stream.getTracks().forEach(t => t.stop());
  status.textContent = "stopped";
  document.getElementById("go").disabled = false;
  document.getElementById("stop").disabled = true;
};
</script>
</body></html>`

// args: title stream, hlsBase, stream
const playerHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>Watch %[1]s</title>
<script src="https://cdn.jsdelivr.net/npm/hls.js@1"></script>
<style>body{font-family:system-ui;background:#111;color:#eee;text-align:center;margin:0;padding:24px}
video{width:90%%;max-width:960px;background:#000;border-radius:8px}</style>
</head><body>
<h2>Stream: %[1]s</h2>
<video id="v" controls autoplay muted playsinline></video>
<p><a href="/" style="color:#6cf">← back</a></p>
<script>
const url = "%[2]s/%[3]s/index.m3u8";
const v = document.getElementById("v");
if (v.canPlayType("application/vnd.apple.mpegurl")) {
  v.src = url; // native HLS (Safari/iOS)
} else if (Hls.isSupported()) {
  const hls = new Hls({lowLatencyMode:true});
  hls.loadSource(url);
  hls.attachMedia(v);
  hls.on(Hls.Events.ERROR, (e,d)=>console.log("hls error", d.type, d.details));
} else {
  document.body.innerHTML += "<p>HLS not supported in this browser.</p>";
}
</script>
</body></html>`
