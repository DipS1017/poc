package hls

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// Session holds a running ffmpeg process that transcodes an incoming WebM stream
// into HLS segments on disk.
type Session struct {
	RoomID    string
	OutputDir string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	mu        sync.Mutex
}

// Write pipes raw video bytes into ffmpeg's stdin. Thread-safe.
func (s *Session) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stdin == nil {
		return 0, fmt.Errorf("session is closed")
	}
	return s.stdin.Write(p)
}

func (s *Session) stop() {
	s.mu.Lock()
	if s.stdin != nil {
		s.stdin.Close()
		s.stdin = nil
	}
	s.mu.Unlock()
	s.cmd.Wait()
}

// Manager creates and manages one ffmpeg HLS session per room.
type Manager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
	BaseDir  string // temp directory holding all rooms' HLS segments
}

// NewManager creates the temp directory and returns a Manager.
// Call Cleanup() when the server shuts down.
func NewManager() (*Manager, error) {
	if err := checkFFmpeg(); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "gostream-hls-*")
	if err != nil {
		return nil, fmt.Errorf("create hls temp dir: %w", err)
	}
	log.Printf("[hls] segments dir: %s", dir)
	return &Manager{
		sessions: make(map[string]*Session),
		BaseDir:  dir,
	}, nil
}

// Start launches an ffmpeg process for the room and returns the session.
// Any existing session for the room is stopped first.
func (m *Manager) Start(roomID string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop any existing session
	if old, ok := m.sessions[roomID]; ok {
		old.stop()
		delete(m.sessions, roomID)
	}

	outDir := filepath.Join(m.BaseDir, roomID)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}

	// Remove leftover files from a previous stream so hls.js doesn't see stale segments
	_ = filepath.Walk(outDir, func(path string, _ os.FileInfo, _ error) error {
		if path != outDir {
			os.Remove(path)
		}
		return nil
	})

	playlistPath := filepath.Join(outDir, "index.m3u8")

	// ffmpeg reads a continuous WebM stream from stdin and outputs HLS.
	//
	// Why these flags:
	//
	//   -fflags +genpts+discardcorrupt  (input)
	//       genpts:        regenerate PTS from DTS when absent.
	//       discardcorrupt: drop any malformed VP8/Opus packets at chunk
	//                      boundaries rather than stalling the pipeline.
	//
	//   -vsync vfr  (output)
	//       The publisher sends 250 ms WebM chunks via sequential HTTP POSTs.
	//       Each chunk arrives as a burst of ~7–8 frames all read within a few
	//       milliseconds. The default vsync mode (cfr) sees those frames as
	//       "all at the same time" and drops all but one, producing ~4 fps
	//       output (visible in ffmpeg's "drop=N" counter).
	//       vfr passes frames through with their original timestamps — no
	//       dropping, no duplicating — giving the true frame rate.
	//
	//   -af aresample=async=1000  (output)
	//       Opus audio timestamps from the browser have ~1 ms of jitter across
	//       chunk boundaries, producing "Non-monotonic DTS" warnings in the AAC
	//       output stream. aresample=async absorbs up to 1000 samples (~21 ms)
	//       of drift by inserting or dropping audio samples, eliminating the
	//       backwards-timestamp warnings without audible artefacts.
	//
	//   -force_key_frames expr:gte(t,n_forced*2)
	//       Forces an I-frame every 2 s (matching hls_time=2) so each HLS
	//       segment starts with a keyframe. Without this, the H.264 encoder
	//       only places keyframes at the interval set by -g, which can land
	//       in the middle of a segment and cause seek artefacts.
	cmd := exec.Command("ffmpeg",
		"-y",
		// ---- input options (must precede -i) ----
		//
		// -f webm: declare input format explicitly so ffmpeg never re-probes.
		// -fflags +genpts+discardcorrupt: regenerate missing PTS; drop corrupt packets.
		"-f", "webm",
		"-fflags", "+genpts+discardcorrupt",
		"-i", "pipe:0",
		// ---- video ----
		//
		// -vf settb=AVTB,setpts=PTS-STARTPTS
		//     PRIMARY FIX for green-artifact tearing.
		//     WebM cluster timestamps are 16-bit integers (max ~65535 ms ≈ 65 s).
		//     After 65 s the browser's MediaRecorder wraps them back to zero, causing
		//     a massive backward jump in PTS. H.264 then can't resolve P/B-frame
		//     references and the decoder emits green pixels (YUV 0,128,128).
		//     settb=AVTB resets the timebase; setpts=PTS-STARTPTS rebuilds PTS
		//     monotonically from 0, discarding the overflowing browser timestamps.
		//
		// -fps_mode cfr / -r 24: constant-frame-rate output.
		//     Gives the encoder a stable, predictable timeline instead of the
		//     bursty variable-rate chunks arriving from the browser.
		//
		// -g 48 -keyint_min 48 -sc_threshold 0
		//     Force an I-frame every 48 frames (24 fps × 2 s = 48), aligned with
		//     hls_time=2. sc_threshold=0 disables scene-cut keyframes so they never
		//     land mid-segment and cause seek artefacts.
		"-vf", "settb=AVTB,setpts=PTS-STARTPTS",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-crf", "23",
		"-fps_mode", "cfr",
		"-r", "24",
		"-g", "48",
		"-keyint_min", "48",
		"-sc_threshold", "0",
		"-force_key_frames", "expr:gte(t,n_forced*2)",
		// ---- audio ----
		//
		// aresample=async=1000: absorbs up to ~21 ms of Opus timestamp jitter at
		// chunk boundaries, eliminating non-monotonic DTS warnings.
		"-c:a", "aac",
		"-ar", "44100",
		"-ac", "2",
		"-b:a", "128k",
		"-af", "aresample=async=1000",
		// ---- HLS muxer ----
		"-f", "hls",
		"-hls_time", "2",
		"-hls_list_size", "6",
		"-hls_flags", "delete_segments+append_list+split_by_time",
		"-hls_segment_type", "mpegts",
		"-hls_allow_cache", "0",
		playlistPath,
	)

	cmd.Stderr = os.Stderr // surface ffmpeg logs

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ffmpeg (is ffmpeg installed?): %w", err)
	}

	session := &Session{
		RoomID:    roomID,
		OutputDir: outDir,
		cmd:       cmd,
		stdin:     stdin,
	}
	m.sessions[roomID] = session
	log.Printf("[hls] started session for room %s", roomID)
	return session, nil
}

// GetSession returns the active session for a room, if any.
func (m *Manager) GetSession(roomID string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[roomID]
	return s, ok
}

// Stop gracefully shuts down the ffmpeg process for a room.
func (m *Manager) Stop(roomID string) {
	m.mu.Lock()
	session, ok := m.sessions[roomID]
	if ok {
		delete(m.sessions, roomID)
	}
	m.mu.Unlock()

	if ok {
		session.stop()
		log.Printf("[hls] stopped session for room %s", roomID)
	}
}

// IsLive reports whether a room currently has an active ffmpeg session.
func (m *Manager) IsLive(roomID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.sessions[roomID]
	return ok
}

// Cleanup removes the temp directory. Call on server shutdown.
func (m *Manager) Cleanup() {
	os.RemoveAll(m.BaseDir)
}

func checkFFmpeg() error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found in PATH — install it first (https://ffmpeg.org/download.html)")
	}
	return nil
}
