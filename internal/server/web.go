package server

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/alexbn71/RustDex/internal/store"
)

// Web UI API: manages the shared address book, the one the RustDesk client
// shows with the RustDex name and passwords.

type webPeer struct {
	ID          string   `json:"id"`
	Alias       string   `json:"alias"`
	Company     string   `json:"company"`
	Contact     string   `json:"contact"`
	Email       string   `json:"email"`
	Phone       string   `json:"phone"`
	Address     string   `json:"address"`
	Note        string   `json:"note"`
	Tags        []string `json:"tags"`
	HasPassword bool     `json:"hasPassword"`

	// Read-only, from the RustDesk client
	Username string `json:"username"`
	Hostname string `json:"hostname"`
	Platform string `json:"platform"`
	OS       string `json:"os"`
	Version  string `json:"version"`
	LastSeen int64  `json:"lastSeen"`

	UpdatedAt int64 `json:"updatedAt"`
}

// webPeerInput is the body of create and update. A missing password keeps
// the stored one, an empty string removes it.
type webPeerInput struct {
	ID       string   `json:"id"`
	Alias    string   `json:"alias"`
	Company  string   `json:"company"`
	Contact  string   `json:"contact"`
	Email    string   `json:"email"`
	Phone    string   `json:"phone"`
	Address  string   `json:"address"`
	Note     string   `json:"note"`
	Tags     []string `json:"tags"`
	Password *string  `json:"password"`
}

func (in *webPeerInput) clean() error {
	in.ID = strings.TrimSpace(in.ID)
	if err := validateID(in.ID); err != nil {
		return err
	}
	for _, f := range []*string{&in.Alias, &in.Company, &in.Contact, &in.Email, &in.Phone, &in.Address} {
		*f = strings.TrimSpace(*f)
		if utf8.RuneCountInString(*f) > 200 {
			return errors.New("a field is longer than 200 characters")
		}
	}
	if utf8.RuneCountInString(in.Note) > 5000 {
		return errors.New("the note is longer than 5000 characters")
	}
	if in.Password != nil && len(*in.Password) > 200 {
		return errors.New("the password is too long")
	}
	in.Tags = store.NormalizeTags(in.Tags)
	for _, t := range in.Tags {
		if utf8.RuneCountInString(t) > 50 {
			return errors.New("a tag is longer than 50 characters")
		}
	}
	return nil
}

func (in *webPeerInput) apply(p *store.Peer) {
	p.ID = in.ID
	p.Alias = in.Alias
	p.Company = in.Company
	p.Contact = in.Contact
	p.Email = in.Email
	p.Phone = in.Phone
	p.Address = in.Address
	p.Note = in.Note
	p.Tags = in.Tags
	if in.Password != nil {
		p.Password = *in.Password
	}
}

func (s *Server) handleWebInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":  s.version,
		"bookName": s.cfg.BookName,
		"user":     s.cfg.AdminUser,
	})
}

func (s *Server) handleWebPeers(w http.ResponseWriter, r *http.Request) {
	peers, err := s.st.ListPeers(store.BookShared)
	if err != nil {
		internalError(w, err)
		return
	}
	devices, err := s.st.Devices()
	if err != nil {
		internalError(w, err)
		return
	}
	tags, err := s.st.ListTags(store.BookShared)
	if err != nil {
		internalError(w, err)
		return
	}

	out := make([]webPeer, 0, len(peers))
	for _, p := range peers {
		wp := webPeer{
			ID: p.ID, Alias: p.Alias, Company: p.Company, Contact: p.Contact, Email: p.Email,
			Phone: p.Phone, Address: p.Address, Note: p.Note, Tags: p.Tags, HasPassword: p.Password != "",
			Username: p.Username, Hostname: p.Hostname, Platform: p.Platform, UpdatedAt: p.UpdatedAt,
		}
		if d, ok := devices[p.ID]; ok {
			wp.OS, wp.Version, wp.LastSeen = d.OS, d.Version, d.LastSeen
		}
		out = append(out, wp)
	}
	outTags := make([]map[string]any, 0, len(tags))
	for _, t := range tags {
		outTags = append(outTags, map[string]any{"name": t.Name, "color": t.Color})
	}
	writeJSON(w, http.StatusOK, map[string]any{"peers": out, "tags": outTags})
}

func (s *Server) handleWebPeerCreate(w http.ResponseWriter, r *http.Request) {
	var in webPeerInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := in.clean(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var p store.Peer
	in.apply(&p)
	err := s.st.InsertPeer(store.BookShared, p)
	if errors.Is(err, store.ErrExists) {
		writeError(w, http.StatusConflict, "this ID is already in the address book")
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{})
}

func (s *Server) handleWebPeerUpdate(w http.ResponseWriter, r *http.Request) {
	oldID := r.PathValue("id")
	var in webPeerInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := in.clean(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := s.st.GetPeer(store.BookShared, oldID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not in the address book")
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	in.apply(&p)
	err = s.st.UpdatePeer(store.BookShared, oldID, p)
	switch {
	case errors.Is(err, store.ErrExists):
		writeError(w, http.StatusConflict, "this ID is already in the address book")
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not in the address book")
	case err != nil:
		internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{})
	}
}

func (s *Server) handleWebPeerDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeletePeers(store.BookShared, []string{r.PathValue("id")}); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (s *Server) handleWebPeerPassword(w http.ResponseWriter, r *http.Request) {
	p, err := s.st.GetPeer(store.BookShared, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not in the address book")
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"password": p.Password})
}

func (s *Server) handleWebTagCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Color int64  `json:"color"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || utf8.RuneCountInString(req.Name) > 50 {
		writeError(w, http.StatusBadRequest, "the tag name must be 1 to 50 characters")
		return
	}
	if err := s.st.AddTag(store.BookShared, store.Tag{Name: req.Name, Color: req.Color}); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{})
}

// handleWebTagUpdate renames a tag and/or changes its color.
func (s *Server) handleWebTagUpdate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req struct {
		Name  *string `json:"name"`
		Color *int64  `json:"color"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.Name != nil {
		newName := strings.TrimSpace(*req.Name)
		if newName == "" || utf8.RuneCountInString(newName) > 50 {
			writeError(w, http.StatusBadRequest, "the tag name must be 1 to 50 characters")
			return
		}
		if newName != name {
			if err := s.st.RenameTag(store.BookShared, name, newName); err != nil {
				tagError(w, err)
				return
			}
			name = newName
		}
	}
	if req.Color != nil {
		if err := s.st.SetTagColor(store.BookShared, name, *req.Color); err != nil {
			tagError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (s *Server) handleWebTagDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteTags(store.BookShared, []string{r.PathValue("name")}); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

func tagError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "tag not found")
		return
	}
	internalError(w, err)
}
