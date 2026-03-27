package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/trannghiach/Seedance-2.0-APIze/queue"
	"github.com/trannghiach/Seedance-2.0-APIze/scraper"
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
	mux.HandleFunc("/v1/videos/generations", s.auth(s.handleGenerate))
	mux.HandleFunc("/v1/videos/", s.auth(s.handleVideoRoute))
	mux.HandleFunc("/health", s.handleHealth)

	addr := fmt.Sprintf(":%s", s.port)
	fmt.Printf("\n  API server running on http://localhost%s\n", addr)
	if s.apiKey != "" {
		fmt.Printf("  API key: %s\n", s.apiKey)
	}
	fmt.Println()
	return http.ListenAndServe(addr, mux)
}

// POST /v1/videos/generations
// multipart/form-data fields:
//   prompt       string   (required)
//   model        string   (seedance-2.0 | seedance-2.0-fast, default: seedance-2.0-fast)
//   duration     int      (4-15, default: 5)
//   aspect_ratio string   (16:9 | 9:16 | 1:1 | 4:3 | 3:4 | 21:9, default: 16:9)
//   mode         string   (omni | start-end, default: omni)
//   references   file(s)  (omni only, max 9)
//   start_frame  file     (start-end only)
//   end_frame    file     (start-end only)
func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(100 << 20); err != nil {
		jsonError(w, "invalid multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}

	opts := scraper.GenerateOptions{
		Prompt:      r.FormValue("prompt"),
		Model:       r.FormValue("model"),
		Mode:        r.FormValue("mode"),
		AspectRatio: r.FormValue("aspect_ratio"),
	}

	if opts.Prompt == "" {
		jsonError(w, "prompt is required", http.StatusBadRequest)
		return
	}

	if d := r.FormValue("duration"); d != "" {
		dur, err := strconv.Atoi(d)
		if err != nil || dur < 4 || dur > 15 {
			jsonError(w, "duration must be an integer between 4 and 15", http.StatusBadRequest)
			return
		}
		opts.Duration = dur
	}

	if opts.Model != "" && opts.Model != "seedance-2.0" && opts.Model != "seedance-2.0-fast" {
		jsonError(w, "model must be 'seedance-2.0' or 'seedance-2.0-fast'", http.StatusBadRequest)
		return
	}

	validRatios := map[string]bool{"16:9": true, "9:16": true, "1:1": true, "4:3": true, "3:4": true, "21:9": true}
	if opts.AspectRatio != "" && !validRatios[opts.AspectRatio] {
		jsonError(w, "invalid aspect_ratio", http.StatusBadRequest)
		return
	}

	if opts.Mode != "" && opts.Mode != "omni" && opts.Mode != "start-end" {
		jsonError(w, "mode must be 'omni' or 'start-end'", http.StatusBadRequest)
		return
	}

	tmpDir, err := os.MkdirTemp("", "dreamina-upload-*")
	if err != nil {
		jsonError(w, "failed to create temp dir", http.StatusInternalServerError)
		return
	}

	// Save reference files (omni mode)
	if refs := r.MultipartForm.File["references"]; len(refs) > 0 {
		if len(refs) > 9 {
			jsonError(w, "max 9 reference files allowed", http.StatusBadRequest)
			return
		}
		for _, fh := range refs {
			f, err := fh.Open()
			if err != nil {
				jsonError(w, "failed to open reference file", http.StatusInternalServerError)
				return
			}
			path, err := saveTempFile(f, fh.Filename, tmpDir)
			f.Close()
			if err != nil {
				jsonError(w, "failed to save reference file", http.StatusInternalServerError)
				return
			}
			opts.References = append(opts.References, path)
		}
	}

	// Save start/end frame files
	if opts.Mode == "start-end" {
		if path, err := saveFormFile(r, "start_frame", tmpDir); err == nil {
			opts.StartFrame = path
		}
		if path, err := saveFormFile(r, "end_frame", tmpDir); err == nil {
			opts.EndFrame = path
		}
	}

	jobID := s.q.Submit(opts)
	jsonOK(w, map[string]interface{}{
		"id":         jobID,
		"status":     "pending",
		"created_at": time.Now().Unix(),
	})
}

// GET /v1/videos/:id  OR  GET /v1/videos/:id/download
func (s *Server) handleVideoRoute(w http.ResponseWriter, r *http.Request) {
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

	resp := map[string]interface{}{
		"id":         job.ID,
		"status":     job.Status,
		"created_at": job.CreatedAt.Unix(),
		"updated_at": job.UpdatedAt.Unix(),
	}
	if job.Status == queue.StatusDone {
		resp["download_url"] = fmt.Sprintf("/v1/videos/%s/download", job.ID)
	}
	if job.Status == queue.StatusFailed {
		resp["error"] = job.Error
	}
	jsonOK(w, resp)
}

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
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="seedance_%s.mp4"`, job.ID[:8]))
	io.Copy(w, f)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

func saveFormFile(r *http.Request, field, dir string) (string, error) {
	f, fh, err := r.FormFile(field)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return saveTempFile(f, fh.Filename, dir)
}

func saveTempFile(content io.Reader, name, dir string) (string, error) {
	ext := filepath.Ext(name)
	tmp, err := os.CreateTemp(dir, "upload-*"+ext)
	if err != nil {
		return "", err
	}
	defer tmp.Close()
	if _, err := io.Copy(tmp, content); err != nil {
		return "", err
	}
	return tmp.Name(), nil
}

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}