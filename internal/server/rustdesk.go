package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alexbn71/RustDex/internal/store"
)

// Handlers for the RustDesk client (Settings > Network > API server).
// Request and response shapes follow flutter/lib/models/ab_model.dart,
// user_model.dart and src/hbbs_http/sync.rs of the RustDesk client.

func (s *Server) userPayload() map[string]any {
	return map[string]any{
		"name":         s.cfg.AdminUser,
		"display_name": s.cfg.AdminUser,
		"email":        "",
		"note":         "",
		"status":       1,
		"is_admin":     true,
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		ID       string `json:"id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	ip := s.clientIP(r)
	if wait := s.limiter.blocked(ip); wait > 0 {
		writeError(w, http.StatusTooManyRequests,
			fmt.Sprintf("too many failed attempts, try again in %d minutes", int(wait.Minutes())+1))
		return
	}
	if !s.checkCredentials(req.Username, req.Password) {
		s.limiter.fail(ip)
		log.Printf("failed login for %q from %s", req.Username, ip)
		writeError(w, http.StatusUnauthorized, "wrong username or password")
		return
	}
	s.limiter.success(ip)

	client := "web"
	if req.ID != "" {
		client = "rustdesk " + req.ID
	}
	token, err := s.newSession(client)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"type":         "access_token",
		"access_token": token,
		"user":         s.userPayload(),
	})
}

func (s *Server) handleLoginOptions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []string{})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if token := bearer(r); token != "" {
		s.st.DeleteSession(tokenHash(token))
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (s *Server) handleCurrentUser(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.userPayload())
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	resp := map[string]any{}
	if decodeJSON(r, &req) == nil && req.ID != "" {
		needSysinfo, err := s.st.Heartbeat(req.ID, time.Now().Unix())
		if err != nil {
			log.Printf("heartbeat: %v", err)
		} else if needSysinfo {
			// The presence of the key makes the client upload its sysinfo again.
			resp["sysinfo"] = true
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSysinfo(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if decodeJSON(r, &req) == nil {
		d := store.Device{
			ID:       anyString(req["id"]),
			UUID:     anyString(req["uuid"]),
			Hostname: anyString(req["hostname"]),
			Username: anyString(req["username"]),
			OS:       anyString(req["os"]),
			Version:  anyString(req["version"]),
			CPU:      anyString(req["cpu"]),
			Memory:   anyString(req["memory"]),
		}
		if d.ID != "" {
			if err := s.st.SaveSysinfo(d, time.Now().Unix()); err != nil {
				log.Printf("sysinfo: %v", err)
			}
		}
	}
	// Always acknowledged: any other answer makes the client retry forever.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("SYSINFO_UPDATED"))
}

func (s *Server) handleSysinfoVer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
}

func (s *Server) handleAbSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"max_peer_one_ab": 0})
}

func (s *Server) handleAbPersonal(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"guid": s.personalGUID})
}

func (s *Server) handleAbProfiles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"total": 1,
		"data": []map[string]any{{
			"guid":  s.sharedGUID,
			"name":  s.cfg.BookName,
			"owner": s.cfg.AdminUser,
			"note":  "",
			"rule":  3, // full control
		}},
	})
}

// clientPeer converts a peer to the client JSON. The shared book carries the
// password, the personal one the hash.
func clientPeer(p store.Peer, book string) map[string]any {
	m := map[string]any{
		"id":               p.ID,
		"alias":            p.Alias,
		"username":         p.Username,
		"hostname":         p.Hostname,
		"platform":         p.Platform,
		"tags":             p.Tags,
		"note":             p.Note,
		"forceAlwaysRelay": strconv.FormatBool(p.ForceAlwaysRelay),
		"rdpPort":          p.RdpPort,
		"rdpUsername":      p.RdpUsername,
		"loginName":        p.LoginName,
	}
	if book == store.BookShared && p.Password != "" {
		m["password"] = p.Password
	}
	if book == store.BookPersonal && p.Hash != "" {
		m["hash"] = p.Hash
	}
	return m
}

func (s *Server) handleAbPeers(w http.ResponseWriter, r *http.Request) {
	book, ok := s.bookFor(r.URL.Query().Get("ab"))
	if !ok {
		writeError(w, http.StatusNotFound, "address book not found")
		return
	}
	peers, err := s.st.ListPeers(book)
	if err != nil {
		internalError(w, err)
		return
	}
	current, size := queryInt(r, "current", 1), queryInt(r, "pageSize", 100)
	start := min((current-1)*size, len(peers))
	end := min(start+size, len(peers))
	data := make([]map[string]any, 0, end-start)
	for _, p := range peers[start:end] {
		data = append(data, clientPeer(p, book))
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": len(peers), "data": data})
}

// bookFromPath resolves the {guid} path value, answering 404 when unknown.
func (s *Server) bookFromPath(w http.ResponseWriter, r *http.Request) (string, bool) {
	book, ok := s.bookFor(r.PathValue("guid"))
	if !ok {
		writeError(w, http.StatusNotFound, "address book not found")
	}
	return book, ok
}

func (s *Server) handleAbTags(w http.ResponseWriter, r *http.Request) {
	book, ok := s.bookFromPath(w, r)
	if !ok {
		return
	}
	tags, err := s.st.ListTags(book)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(tags))
	for _, t := range tags {
		out = append(out, map[string]any{"name": t.Name, "color": t.Color})
	}
	writeJSON(w, http.StatusOK, out)
}

// applyClientFields copies the fields present in a client request onto p.
// Fields the client does not send are left untouched.
func applyClientFields(p *store.Peer, fields map[string]json.RawMessage, book string) {
	strs := map[string]*string{
		"alias":       &p.Alias,
		"username":    &p.Username,
		"hostname":    &p.Hostname,
		"platform":    &p.Platform,
		"note":        &p.Note,
		"rdpPort":     &p.RdpPort,
		"rdpUsername": &p.RdpUsername,
		"loginName":   &p.LoginName,
	}
	if book == store.BookShared {
		strs["password"] = &p.Password
	} else {
		strs["hash"] = &p.Hash
	}
	for key, dst := range strs {
		if raw, ok := fields[key]; ok {
			*dst = rawString(raw)
		}
	}
	if raw, ok := fields["tags"]; ok {
		var tags []any
		json.Unmarshal(raw, &tags)
		p.Tags = p.Tags[:0]
		for _, t := range tags {
			p.Tags = append(p.Tags, anyString(t))
		}
		p.Tags = store.NormalizeTags(p.Tags)
	}
	if raw, ok := fields["forceAlwaysRelay"]; ok {
		p.ForceAlwaysRelay = rawString(raw) == "true"
	}
}

func (s *Server) handleAbPeerAdd(w http.ResponseWriter, r *http.Request) {
	book, ok := s.bookFromPath(w, r)
	if !ok {
		return
	}
	var fields map[string]json.RawMessage
	if err := decodeJSON(r, &fields); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	id := strings.TrimSpace(rawString(fields["id"]))
	if err := validateID(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Adding a peer that already exists updates it.
	p, err := s.st.GetPeer(book, id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		p = store.Peer{ID: id, Tags: []string{}}
		applyClientFields(&p, fields, book)
		err = s.st.InsertPeer(book, p)
	case err == nil:
		applyClientFields(&p, fields, book)
		err = s.st.UpdatePeer(book, id, p)
	}
	if err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleAbPeerUpdate(w http.ResponseWriter, r *http.Request) {
	book, ok := s.bookFromPath(w, r)
	if !ok {
		return
	}
	var fields map[string]json.RawMessage
	if err := decodeJSON(r, &fields); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	id := rawString(fields["id"])
	p, err := s.st.GetPeer(book, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusOK, "peer not found")
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	applyClientFields(&p, fields, book)
	if err := s.st.UpdatePeer(book, id, p); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleAbPeerDelete(w http.ResponseWriter, r *http.Request) {
	book, ok := s.bookFromPath(w, r)
	if !ok {
		return
	}
	var ids []string
	if err := decodeJSON(r, &ids); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := s.st.DeletePeers(book, ids); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleAbTagAdd(w http.ResponseWriter, r *http.Request) {
	book, ok := s.bookFromPath(w, r)
	if !ok {
		return
	}
	var req struct {
		Name  string `json:"name"`
		Color int64  `json:"color"`
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := s.st.AddTag(book, store.Tag{Name: strings.TrimSpace(req.Name), Color: req.Color}); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleAbTagRename(w http.ResponseWriter, r *http.Request) {
	book, ok := s.bookFromPath(w, r)
	if !ok {
		return
	}
	var req struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.New) == "" {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	err := s.st.RenameTag(book, req.Old, strings.TrimSpace(req.New))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusOK, "tag not found")
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleAbTagUpdate(w http.ResponseWriter, r *http.Request) {
	book, ok := s.bookFromPath(w, r)
	if !ok {
		return
	}
	var req struct {
		Name  string `json:"name"`
		Color int64  `json:"color"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	err := s.st.SetTagColor(book, req.Name, req.Color)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusOK, "tag not found")
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleAbTagDelete(w http.ResponseWriter, r *http.Request) {
	book, ok := s.bookFromPath(w, r)
	if !ok {
		return
	}
	var names []string
	if err := decodeJSON(r, &names); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := s.st.DeleteTags(book, names); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// rawString reads a JSON value as a string; numbers and booleans are
// returned as written, null and missing values as "".
func rawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	return anyString(v)
}

func anyString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// validateID accepts RustDesk IDs (digits or custom IDs) and the IP or
// host[:port] values the client also stores as IDs.
func validateID(id string) error {
	if id == "" {
		return errors.New("the ID is required")
	}
	if len(id) > 100 {
		return errors.New("the ID is too long")
	}
	for _, c := range id {
		if c <= ' ' || c == '/' || c == '\\' || c == '?' || c == '#' || c == '%' {
			return errors.New("the ID contains characters that are not allowed")
		}
	}
	return nil
}
