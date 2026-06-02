# MediaMTX Livestream POC

Minimal **RTMP ingest → HLS playback** livestream stack, the same pattern Twitch/YouTube Live use under the hood.

```
  OBS / ffmpeg ──RTMP──▶ MediaMTX ──HLS──▶ browser
   (publish)            (repackage)       (<video> + hls.js)
```

## Components

| Service    | Role                                                        | Port(s)            |
|------------|-------------------------------------------------------------|--------------------|
| `mediamtx` | Ingests RTMP/WebRTC, serves live HLS, runs ffmpeg packager  | 1935, 8889, 8888, 9997 |
| `app`      | Go: player pages, MediaMTX API proxy, **S3 uploader + VOD** | 8080               |
| `shared-minio` | Existing external MinIO (S3) — HLS chunks land in `hls` bucket | 9000, 9001    |

## Live → HLS → S3 pipeline

```
 publisher ─▶ MediaMTX ─▶ (runOnReady) ffmpeg ─▶ /hls_out/<path>/*.ts+m3u8 ─▶ Go uploader ─▶ MinIO (hls bucket)
             ingest       packages to H264+AAC HLS                            (minio-go SDK)        │
                                                                                                    ▼
                                                                          /vod, /vodwatch  ◀── replay from S3
```

- MediaMTX **cannot** write HLS to S3 itself, so `runOnReady` launches **ffmpeg** to package the
  stream into standard HLS (`.m3u8` + `.ts`). It **remuxes** (`-c:v copy`) — the video is *not*
  re-encoded, just re-chunked (near-zero CPU); only audio is encoded to **AAC** because HLS requires
  it. (Publishers must send **H264**: OBS does, and the browser page forces it.)
- Each publish starts a clean recording (`runOnReady` clears the path dir first; no `append_list`).
- The Go app polls `/hls_out` once per second and uploads new/changed files to MinIO via `minio-go`,
  always uploading **`.ts` segments before the `.m3u8`** so a live playlist never references a
  segment that isn't in S3 yet.
- Uses the **existing `shared-minio`** on the external `shared` Docker network. Writes only to a new
  `hls` bucket — the existing `profiles/` and `wepreach/` buckets are untouched.

## S3 / VOD endpoints

| Route                       | Purpose                                                      |
|-----------------------------|--------------------------------------------------------------|
| `/vod`                      | List recordings stored in the `hls` bucket                   |
| `/vodwatch?stream=<path>`   | Player (uses presigned playback)                             |
| `/vodplay/<path>/index.m3u8`| **Presigned mode**: playlist with signed direct-to-S3 segment URLs |
| `/vodhls/<key>`             | **Proxy mode**: streams an object through the app            |

### Two playback modes

The `hls` bucket is **private**. There are two ways to play it back:

**Proxy mode (`/vodhls/`)** — every byte flows through the Go app:
```
browser ──▶ app ──(GetObject)──▶ MinIO ──pipes bytes back──▶ browser
```
Simple, but the app is a bandwidth bottleneck and you can't put a CDN in front.

**Presigned mode (`/vodplay/`, default)** — the app only serves a rewritten playlist;
segments are pulled **straight from S3** by the browser:
```
browser ──▶ app: rewritten m3u8 (segment lines = presigned URLs)
browser ──▶ MinIO/CDN directly:  seg.ts?X-Amz-Signature=...   (heavy bytes bypass the app)
```
A **presigned URL** is a time-limited (1h here) signed link granting temporary GET access to a
private object — no credentials on the client. This is the production/CDN-friendly shape.

Two implementation details that matter:
- **Signed against the public endpoint** (`MINIO_PUBLIC_ENDPOINT=localhost:9000`), not the
  docker-internal `shared-minio:9000` — the hostname is part of the signature.
- **`MINIO_REGION` is set explicitly** so the SDK doesn't make a `GetBucketLocation` network
  call (which would fail from inside the container).

Config via env (`docker-compose.yml`): `MINIO_ENDPOINT`, `MINIO_PUBLIC_ENDPOINT`, `MINIO_REGION`,
`MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_BUCKET`, `HLS_OUT_DIR`.

## Run

```bash
docker compose up -d --build
```

Open <http://localhost:8080> for instructions and links.

## Stream from your browser (WebRTC, no OBS)

Browsers can't push RTMP, so webcam publishing uses **WebRTC/WHIP**.

1. Open the broadcaster: <http://localhost:8080/broadcast?stream=live/webcam>
2. Click **● Go Live**, allow camera + mic.
3. Share the watch link below.

```
  browser webcam ──WebRTC/WHIP──▶ MediaMTX ──HLS──▶ viewers
       :8889                                :8888
```

**Watch link to share:** <http://localhost:8080/watch?stream=live/webcam>

> `localhost` links only work on this machine. To share on your WiFi use
> `http://<LAN-IP>:8080/...`; to share over the internet, expose port 8080
> with a tunnel (cloudflared/ngrok). WebRTC publish from a *remote* device
> also needs `webrtcAdditionalHosts: [<server-ip>]` in `mediamtx.yml`.

## 1. Publish (RTMP ingest)

**OBS:** Settings → Stream → Custom — Server `rtmp://localhost:1935/live`, Stream Key `mystream`.

**ffmpeg test pattern:**
```bash
ffmpeg -re -f lavfi -i testsrc=size=1280x720:rate=30 \
       -f lavfi -i sine=frequency=440 \
       -c:v libx264 -preset veryfast -tune zerolatency \
       -c:a aac -f flv rtmp://localhost:1935/live/mystream
```

The RTMP app (`live`) + key (`mystream`) become the MediaMTX path **`live/mystream`**.

## 2. Watch (HLS playback)

- Player page: <http://localhost:8080/watch?stream=live/mystream>
- Raw HLS playlist: `http://localhost:8888/live/mystream/index.m3u8`

> Expect ~5–15s latency — normal for RTMP→HLS. Add WebRTC later if you need sub-second.

## 3. Inspect

- `GET http://localhost:8080/paths` — live stream state (proxies MediaMTX `/v3/paths/list`)
- `GET http://localhost:8080/health` — app health

## Go service (`main.go`)

| Route                    | Purpose                                  |
|--------------------------|------------------------------------------|
| `/`                      | Landing page with instructions           |
| `/watch?stream=<path>`   | HLS player (hls.js, native HLS on Safari)|
| `/paths`                 | Proxy to MediaMTX control API            |
| `/health`                | Health check                             |

## Notes

- `mediamtx.yml` grants `any` user full permissions — **POC only**, lock down for production.
- MediaMTX serves HLS behind a `cookieCheck` redirect; browsers handle it automatically (curl needs `-L -c jar -b jar`).
- Stop: `docker compose down`.
