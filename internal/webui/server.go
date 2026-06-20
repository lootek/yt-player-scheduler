package webui

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
)

// Server serves the web UI over HTTP.
type Server struct {
	service *Service
	logger  *log.Logger
	mux     *http.ServeMux
	tpl     *template.Template
	srv     *http.Server
	auth    struct {
		username string
		password string
		enabled  bool
	}
}

// NewServer creates a new HTTP server for the web UI.
func NewServer(service *Service, logger *log.Logger) *Server {
	s := &Server{
		service: service,
		logger:  logger,
		mux:     http.NewServeMux(),
		tpl:     templates,
	}

	cfg := service.cfg.Global.WebUI
	if cfg.Username != "" && cfg.Password != "" {
		s.auth.enabled = true
		s.auth.username = cfg.Username
		s.auth.password = cfg.Password
	} else {
		logger.Printf("webui: Basic auth disabled (username/password not set)")
	}

	s.mux.HandleFunc("GET /{$}", s.recoverer(s.withAuth(s.handleIndex)))
	s.mux.HandleFunc("POST /download", s.recoverer(s.withAuth(s.handleDownload)))
	s.mux.HandleFunc("GET /status", s.recoverer(s.withAuth(s.handleStatus)))
	s.mux.HandleFunc("GET /api/jobs", s.recoverer(s.withAuth(s.handleAPIJobs)))
	s.mux.HandleFunc("GET /api/jobs/{id}", s.recoverer(s.withAuth(s.handleAPIJob)))
	s.mux.HandleFunc("GET /log/{id}", s.recoverer(s.withAuth(s.handleLog)))
	s.mux.HandleFunc("GET /static/", s.recoverer(s.withAuth(s.handleStatic)))

	return s
}

// ListenAndServe starts the HTTP server on the given address.
func (s *Server) ListenAndServe(addr string) error {
	s.srv = &http.Server{
		Addr:    addr,
		Handler: s.mux,
	}
	return s.srv.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

func (s *Server) recoverer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Printf("webui handler panic: %v", rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next(w, r)
	}
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	if !s.auth.enabled {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || !constantTimeEqual(user, s.auth.username) || !constantTimeEqual(pass, s.auth.password) {
			w.Header().Set("WWW-Authenticate", `Basic realm="yt-rpi-player"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"DownloadDir": s.service.cfg.Global.WebUI.DownloadDir,
		"Subdir":      s.service.cfg.Global.WebUI.Subdir,
	}
	if err := s.tpl.ExecuteTemplate(w, "index.html", data); err != nil {
		s.logger.Printf("webui render index: %v", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	url := strings.TrimSpace(r.FormValue("url"))
	if url == "" {
		http.Error(w, "url required", http.StatusBadRequest)
		return
	}

	video := r.FormValue("video") == "on"
	mpd := r.FormValue("mpd") == "on"
	autoPlay := r.FormValue("autoplay") == "on"

	id, err := s.service.Enqueue(url, video, mpd, autoPlay)
	if err != nil {
		s.logger.Printf("webui enqueue failed: %v", err)
		http.Error(w, "enqueue failed", http.StatusServiceUnavailable)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/status?id=%s", id), http.StatusSeeOther)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"ID": r.URL.Query().Get("id"),
	}
	if err := s.tpl.ExecuteTemplate(w, "status.html", data); err != nil {
		s.logger.Printf("webui render status: %v", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

func (s *Server) handleAPIJobs(w http.ResponseWriter, r *http.Request) {
	jobs := s.service.List(50)
	writeJSON(w, jobs)
}

func (s *Server) handleAPIJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, ok := s.service.Get(id)
	if !ok {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	writeJSON(w, job)
}

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, ok := s.service.Get(id)
	if !ok {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(job.Log.Bytes())
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	name := "static/" + strings.TrimPrefix(r.URL.Path, "/static/")
	data, err := staticFS.ReadFile(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	contentType := "text/plain; charset=utf-8"
	if strings.HasSuffix(name, ".js") {
		contentType = "application/javascript; charset=utf-8"
	} else if strings.HasSuffix(name, ".css") {
		contentType = "text/css; charset=utf-8"
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
	}
}
