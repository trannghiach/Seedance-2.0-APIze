# Seedance-2.0-APIze

> Unofficial REST API wrapper for [Dreamina](https://dreamina.capcut.com) / Seedance 2.0 — no official API needed.

Since ByteDance hasn't released a public API for Seedance 2.0 yet, this project reverse-wraps the Dreamina web UI using Playwright and exposes it as a clean REST API you can call from any language or tool.

Built by [@trannghiach](https://github.com/trannghiach) a.k.a @lilsadfoqs

---

## How it works

```
POST /v1/videos/generations
        ↓
  Playwright controls Chromium
        ↓
  Dreamina UI generates video (Seedance 2.0 Fast, default settings)
        ↓
GET /v1/videos/{id}/download
```

No API keys from ByteDance needed. Just a Dreamina account.

---

## Quickstart

**Requirements:** 
- Go 1.22+
- A Dreamina account with an active plan or at least free trial at [dreamina.capcut.com](https://dreamina.capcut.com)

> Free accounts get ~225 credits/day. One 4s video costs ~140 credits.

```bash
git clone https://github.com/trannghiach/Seedance-2.0-APIze
cd Seedance-2.0-APIze
go mod tidy

# 1. Login once — saves session to .dreamina-session/
go run . login

# 2. Start the API server
go run . serve --port 8080

# 3. Generate a video
curl -X POST http://localhost:8080/v1/videos/generations \
  -H "Content-Type: application/json" \
  -d '{"prompt": "a cat sitting on the cloud, cinematic"}'

# 3.6. Only if you need to verify the whole Playwright process
go run . test "me and the boys at the pool party flirting girls"
```


---

## API Reference

### `POST /v1/videos/generations`

Submit a video generation job. Returns immediately with a job ID.

Video is generated using Dreamina's `AI Video` mode default settings.

**Request body:**
```json
{
  "prompt": "a cat walking in rain, cinematic lighting"
}
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending",
  "created_at": 1234567890
}
```

---

### `GET /v1/videos/{id}`

Poll job status.

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "done",
  "download_url": "/v1/videos/550e8400.../download",
  "created_at": 1234567890,
  "updated_at": 1234567950
}
```

| Status | Meaning |
|---|---|
| `pending` | Queued, not started yet |
| `processing` | Dreamina is generating |
| `done` | Ready to download |
| `failed` | Generation failed |

---

### `GET /v1/videos/{id}/download`

Download the generated `.mp4` file.

```bash
curl http://localhost:8080/v1/videos/{id}/download -o video.mp4
```

---

### `GET /health`

```json
{"status": "ok"}
```

---

## CLI Commands

```bash
# Login (run once per account)
go run . login

# Start API server (headless by default)
go run . serve --port 8080 --key mysecretkey

# Show browser window (useful for debugging)
go run . serve --port 8080 --show

# Quick test — generate 1 video from terminal
go run . test "a futuristic city at night"
```

---

## Using an API key (Untested seriously, if you need this you can test it yourself and report any issues you got. Thanks :D)

```bash
go run . serve --port 8080 --key mysecretkey
```

```bash
curl -X POST http://localhost:8080/v1/videos/generations \
  -H "Authorization: Bearer mysecretkey" \
  -H "Content-Type: application/json" \
  -d '{"prompt": "..."}'
```

---

## Project structure

```
Seedance-2.0-APIze/
├── main.go              # Entry point: login / serve / test
├── browser/manager.go   # Playwright setup, persistent session
├── scraper/scraper.go   # UI automation — core logic
├── queue/queue.go       # Async job queue
└── server/server.go     # HTTP API server
```

---

## Notes

- Session is saved to `.dreamina-session/` — gitignored, never commit it
- Default concurrency is 1 (one video at a time) to avoid rate limiting
- Generation takes ~30–90 seconds depending on Dreamina server load
- Tested on Windows

---

## Disclaimer

This is an **unofficial** project not affiliated with ByteDance or Dreamina. It may break at any time if Dreamina updates their UI. Use at your own risk.

---

## License

MIT