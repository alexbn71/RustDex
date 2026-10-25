// Package server exposes the subset of the RustDesk API server protocol
// needed for the address book, plus the API of the RustDex web UI.
package server

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"

	"github.com/alexbn71/RustDex/internal/config"
	"github.com/alexbn71/RustDex/internal/store"
)

const maxBody = 1 << 20

type Server struct {
	cfg     config.Config
	st      *store.Store
	version string
	mux     *http.ServeMux
	limiter *loginLimiter

	adminHash    string // set when RUSTDEX_ADMIN_PASSWORD is empty
	personalGUID string
	sharedGUID   string
}

func New(cfg config.Config, st *store.Store, static fs.FS, version string) (*Server, error) {
	s := &Server{cfg: cfg, st: st, version: version, limiter: newLoginLimiter()}
	if err := s.initAdmin(); err != nil {
		return nil, err
	}
	var err error
	if s.personalGUID, err = s.bookGUID("personal_guid"); err != nil {
		return nil, err
	}
	if s.sharedGUID, err = s.bookGUID("shared_guid"); err != nil {
		return nil, err
	}
	s.routes(static)
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes(static fs.FS) {
	m := http.NewServeMux()

	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// RustDesk client
	m.HandleFunc("POST /api/login", s.handleLogin)
	m.HandleFunc("GET /api/login-options", s.handleLoginOptions)
	m.HandleFunc("POST /api/logout", s.handleLogout)
	m.HandleFunc("POST /api/currentUser", s.auth(s.handleCurrentUser))
	m.HandleFunc("POST /api/heartbeat", s.handleHeartbeat)
	m.HandleFunc("POST /api/sysinfo", s.handleSysinfo)
	m.HandleFunc("POST /api/sysinfo_ver", s.handleSysinfoVer)
	m.HandleFunc("POST /api/ab/settings", s.auth(s.handleAbSettings))
	m.HandleFunc("POST /api/ab/personal", s.auth(s.handleAbPersonal))
	m.HandleFunc("POST /api/ab/shared/profiles", s.auth(s.handleAbProfiles))
	m.HandleFunc("POST /api/ab/peers", s.auth(s.handleAbPeers))
	m.HandleFunc("POST /api/ab/tags/{guid}", s.auth(s.handleAbTags))
	m.HandleFunc("POST /api/ab/peer/add/{guid}", s.auth(s.handleAbPeerAdd))
	m.HandleFunc("PUT /api/ab/peer/update/{guid}", s.auth(s.handleAbPeerUpdate))
	m.HandleFunc("DELETE /api/ab/peer/{guid}", s.auth(s.handleAbPeerDelete))
	m.HandleFunc("POST /api/ab/tag/add/{guid}", s.auth(s.handleAbTagAdd))
	m.HandleFunc("PUT /api/ab/tag/rename/{guid}", s.auth(s.handleAbTagRename))
	m.HandleFunc("PUT /api/ab/tag/update/{guid}", s.auth(s.handleAbTagUpdate))
	m.HandleFunc("DELETE /api/ab/tag/{guid}", s.auth(s.handleAbTagDelete))
	// Features RustDex does not have, answered as empty so the client shows
	// empty tabs instead of errors.
	emptyList := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"total": 0, "data": []any{}})
	}
	m.HandleFunc("/api/users", s.auth(emptyList))
	m.HandleFunc("/api/peers", s.auth(emptyList))
	m.HandleFunc("/api/device-group/accessible", s.auth(emptyList))
	m.HandleFunc("POST /api/audit/{kind}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{})
	})
	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})

	// Web UI
	m.HandleFunc("GET /rx/api/info", s.auth(s.handleWebInfo))
	m.HandleFunc("GET /rx/api/peers", s.auth(s.handleWebPeers))
	m.HandleFunc("POST /rx/api/peers", s.auth(s.handleWebPeerCreate))
	m.HandleFunc("PUT /rx/api/peers/{id}", s.auth(s.handleWebPeerUpdate))
	m.HandleFunc("DELETE /rx/api/peers/{id}", s.auth(s.handleWebPeerDelete))
	m.HandleFunc("GET /rx/api/peers/{id}/password", s.auth(s.handleWebPeerPassword))
	m.HandleFunc("POST /rx/api/tags", s.auth(s.handleWebTagCreate))
	m.HandleFunc("PUT /rx/api/tags/{name}", s.auth(s.handleWebTagUpdate))
	m.HandleFunc("DELETE /rx/api/tags/{name}", s.auth(s.handleWebTagDelete))
	m.HandleFunc("/rx/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})

	files := http.FileServerFS(static)
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; frame-ancestors 'none'")
		h.Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})

	s.mux = m
}

// bookGUID returns the GUID of an address book, generated on first start.
func (s *Server) bookGUID(key string) (string, error) {
	guid, err := s.st.GetMeta(key)
	if err == nil {
		return guid, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	guid = fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	return guid, s.st.SetMeta(key, guid)
}

func (s *Server) bookFor(guid string) (string, bool) {
	switch guid {
	case s.personalGUID:
		return store.BookPersonal, true
	case s.sharedGUID:
		return store.BookShared, true
	}
	return "", false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError answers with {"error": msg}, the shape the RustDesk client checks.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func internalError(w http.ResponseWriter, err error) {
	log.Printf("error: %v", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func queryInt(r *http.Request, name string, def int) int {
	if n, err := strconv.Atoi(r.URL.Query().Get(name)); err == nil && n > 0 {
		return n
	}
	return def
}
