package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/yourname/dreamina-pw/queue"
	"github.com/yourname/dreamina-pw/scraper"
)

type Server struct {
	q      *queue.Queue
	apiKey string
	port   string
}

func New(q *queue.Queue, apiKey, port string) *Server {
	return &Server{q: q, apiKey: apiKey, port: port}
}

func (s *Server) Run() error {
	mux := http.NewServeMux()

	// ── Endpoints ────────────────────────────────────────────────────
	//
	// POST /v1/videos/generations  → submit job
	// GET  /v1/videos/:id          → poll status
	// GET  /v1/videos/:id/download → download video file
	// GET  /health                 → healthcheck

	mux.HandleFunc("/v1/videos/generations", s.auth(s.handleGenerate))
	mux.HandleFunc("/v1/videos/", s.auth(s.handleVideoRoute))
	mux.HandleFunc("/health", s.handleHealth)

	addr := fmt.Sprintf(":%s", s.port)
	fmt.Printf("\n  API server running on http://localhost%s\n", addr)
	fmt.Printf("  API key: %s\n\n", s.apiKey)

	return http.ListenAndServe(addr, mux)
}

// ── Handlers ──────────────────────────────────────────────────────────────

// POST /v1/videos/generations
func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Prompt      string `json:"prompt"`
		Duration    int    `json:"duration"`
		Resolution  string `json:"resolution"`
		AspectRatio string `json:"aspect_ratio"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Prompt == "" {
		jsonError(w, "prompt is required", http.StatusBadRequest)
		return
	}

	// Defaults
	if req.Duration == 0 {
		req.Duration = 4
	}
	if req.Resolution == "" {
		req.Resolution = "720p"
	}
	if req.AspectRatio == "" {
		req.AspectRatio = "16:9"
	}

	jobID := s.q.Submit(scraper.GenerateOptions{
		Prompt:      req.Prompt,
		Duration:    req.Duration,
		Resolution:  req.Resolution,
		AspectRatio: req.AspectRatio,
	})

	jsonOK(w, map[string]interface{}{
		"id":         jobID,
		"status":     "pending",
		"created_at": time.Now().Unix(),
	})
}

// GET /v1/videos/:id  OR  GET /v1/videos/:id/download
func (s *Server) handleVideoRoute(w http.ResponseWriter, r *http.Request) {
	// Parse path: /v1/videos/{id} or /v1/videos/{id}/download
	path := strings.TrimPrefix(r.URL.Path, "/v1/videos/")
	parts := strings.SplitN(path, "/", 2)
	jobID := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}

	job, ok := s.q.Get(jobID)
	if !ok {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}

	if action == "download" {
		s.handleDownload(w, r, job)
		return
	}

	// Status response
	resp := map[string]interface{}{
		"id":         job.ID,
		"status":     job.Status,
		"created_at": job.CreatedAt.Unix(),
		"updated_at": job.UpdatedAt.Unix(),
	}
	if job.Status == queue.StatusDone {
		resp["download_url"] = fmt.Sprintf("/v1/videos/%s/download", job.ID)
		resp["video_url"] = job.VideoURL
	}
	if job.Status == queue.StatusFailed {
		resp["error"] = job.Error
	}

	jsonOK(w, resp)
}

// GET /v1/videos/:id/download
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request, job *queue.Job) {
	if job.Status != queue.StatusDone {
		jsonError(w, fmt.Sprintf("job not ready (status: %s)", job.Status), http.StatusConflict)
		return
	}
	if job.VideoPath == "" {
		jsonError(w, "video file not available", http.StatusInternalServerError)
		return
	}

	f, err := os.Open(job.VideoPath)
	if err != nil {
		jsonError(w, "could not open video file", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="seedance_%s.mp4"`, job.ID[:8]))

	io.Copy(w, f)
}

// GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "ok"})
}

// ── Middleware ────────────────────────────────────────────────────────────

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Skip auth if API key is not configured
		if s.apiKey == "" {
			next(w, r)
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token != s.apiKey {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
