package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"hash/fnv"
	"strings"
	"time"
)

type Peer struct {
	ID       string
	Alias    string
	Password string // shared book only, plain text in memory
	Hash     string // personal book only, opaque value set by the client

	// Reported by the RustDesk client
	Username         string
	Hostname         string
	Platform         string
	ForceAlwaysRelay bool
	RdpPort          string
	RdpUsername      string
	LoginName        string

	Tags []string
	Note string

	// Contact details, edited from the web UI only
	Company string
	Contact string
	Email   string
	Phone   string
	Address string

	CreatedAt int64
	UpdatedAt int64
}

type Tag struct {
	Name  string
	Color int64 // ARGB, as the RustDesk client stores it
}

const peerColumns = `id, alias, password_enc, hash, username, hostname, platform, tags, note,
	force_always_relay, rdp_port, rdp_username, login_name,
	company, contact, email, phone, address, created_at, updated_at`

type scanner interface{ Scan(dest ...any) error }

func (s *Store) scanPeer(row scanner) (Peer, error) {
	var p Peer
	var pwEnc, tags string
	var relay int
	err := row.Scan(&p.ID, &p.Alias, &pwEnc, &p.Hash, &p.Username, &p.Hostname, &p.Platform, &tags, &p.Note,
		&relay, &p.RdpPort, &p.RdpUsername, &p.LoginName,
		&p.Company, &p.Contact, &p.Email, &p.Phone, &p.Address, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	p.ForceAlwaysRelay = relay != 0
	p.Tags = decodeTags(tags)
	if p.Password, err = s.decrypt(pwEnc); err != nil {
		return p, err
	}
	return p, nil
}

func (s *Store) ListPeers(book string) ([]Peer, error) {
	rows, err := s.db.Query(`SELECT `+peerColumns+` FROM peers WHERE book = ?
		ORDER BY lower(CASE WHEN alias = '' THEN id ELSE alias END), id`, book)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	peers := []Peer{}
	for rows.Next() {
		p, err := s.scanPeer(rows)
		if err != nil {
			return nil, err
		}
		peers = append(peers, p)
	}
	return peers, rows.Err()
}

func (s *Store) GetPeer(book, id string) (Peer, error) {
	p, err := s.scanPeer(s.db.QueryRow(`SELECT `+peerColumns+` FROM peers WHERE book = ? AND id = ?`, book, id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func (s *Store) InsertPeer(book string, p Peer) error {
	pwEnc, err := s.encrypt(p.Password)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if exists, err := peerExists(tx, book, p.ID); err != nil {
		return err
	} else if exists {
		return ErrExists
	}
	now := time.Now().Unix()
	_, err = tx.Exec(`INSERT INTO peers (book, `+peerColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		book, p.ID, p.Alias, pwEnc, p.Hash, p.Username, p.Hostname, p.Platform, encodeTags(p.Tags), p.Note,
		boolInt(p.ForceAlwaysRelay), p.RdpPort, p.RdpUsername, p.LoginName,
		p.Company, p.Contact, p.Email, p.Phone, p.Address, now, now)
	if err != nil {
		return err
	}
	if err := ensureTags(tx, book, p.Tags); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdatePeer replaces the peer stored as oldID with p; p.ID may differ from
// oldID to change the RustDesk ID.
func (s *Store) UpdatePeer(book, oldID string, p Peer) error {
	pwEnc, err := s.encrypt(p.Password)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if p.ID != oldID {
		if exists, err := peerExists(tx, book, p.ID); err != nil {
			return err
		} else if exists {
			return ErrExists
		}
	}
	res, err := tx.Exec(`UPDATE peers SET id = ?, alias = ?, password_enc = ?, hash = ?, username = ?, hostname = ?,
		platform = ?, tags = ?, note = ?, force_always_relay = ?, rdp_port = ?, rdp_username = ?, login_name = ?,
		company = ?, contact = ?, email = ?, phone = ?, address = ?, updated_at = ?
		WHERE book = ? AND id = ?`,
		p.ID, p.Alias, pwEnc, p.Hash, p.Username, p.Hostname,
		p.Platform, encodeTags(p.Tags), p.Note, boolInt(p.ForceAlwaysRelay), p.RdpPort, p.RdpUsername, p.LoginName,
		p.Company, p.Contact, p.Email, p.Phone, p.Address, time.Now().Unix(),
		book, oldID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if err := ensureTags(tx, book, p.Tags); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeletePeers(book string, ids []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if _, err := tx.Exec(`DELETE FROM peers WHERE book = ? AND id = ?`, book, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListTags(book string) ([]Tag, error) {
	rows, err := s.db.Query(`SELECT name, color FROM tags WHERE book = ? ORDER BY lower(name)`, book)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tags := []Tag{}
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.Name, &t.Color); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// AddTag creates a tag, or changes its color if it already exists.
// A zero color picks one from the default palette.
func (s *Store) AddTag(book string, t Tag) error {
	if t.Color == 0 {
		t.Color = defaultColor(t.Name)
	}
	_, err := s.db.Exec(`INSERT INTO tags (book, name, color) VALUES (?, ?, ?)
		ON CONFLICT (book, name) DO UPDATE SET color = excluded.color`, book, t.Name, t.Color)
	return err
}

func (s *Store) SetTagColor(book, name string, color int64) error {
	res, err := s.db.Exec(`UPDATE tags SET color = ? WHERE book = ? AND name = ?`, color, book, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// RenameTag renames a tag and every reference to it; renaming onto an
// existing tag merges the two.
func (s *Store) RenameTag(book, oldName, newName string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var color int64
	err = tx.QueryRow(`SELECT color FROM tags WHERE book = ? AND name = ?`, book, oldName).Scan(&color)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM tags WHERE book = ? AND name = ?`, book, oldName); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO tags (book, name, color) VALUES (?, ?, ?)`, book, newName, color); err != nil {
		return err
	}
	err = rewriteTags(tx, book, func(tags []string) []string {
		for i, t := range tags {
			if t == oldName {
				tags[i] = newName
			}
		}
		return tags
	})
	if err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteTags removes the tags and takes them off every peer.
func (s *Store) DeleteTags(book string, names []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	drop := map[string]bool{}
	for _, n := range names {
		drop[n] = true
		if _, err := tx.Exec(`DELETE FROM tags WHERE book = ? AND name = ?`, book, n); err != nil {
			return err
		}
	}
	err = rewriteTags(tx, book, func(tags []string) []string {
		kept := tags[:0]
		for _, t := range tags {
			if !drop[t] {
				kept = append(kept, t)
			}
		}
		return kept
	})
	if err != nil {
		return err
	}
	return tx.Commit()
}

// rewriteTags applies fn to the tag list of every peer in the book and saves
// the lists that changed.
func rewriteTags(tx *sql.Tx, book string, fn func([]string) []string) error {
	rows, err := tx.Query(`SELECT id, tags FROM peers WHERE book = ?`, book)
	if err != nil {
		return err
	}
	changed := map[string]string{}
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		if updated := encodeTags(fn(decodeTags(raw))); updated != raw {
			changed[id] = updated
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for id, tags := range changed {
		if _, err := tx.Exec(`UPDATE peers SET tags = ? WHERE book = ? AND id = ?`, tags, book, id); err != nil {
			return err
		}
	}
	return nil
}

func ensureTags(tx *sql.Tx, book string, tags []string) error {
	for _, t := range tags {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO tags (book, name, color) VALUES (?, ?, ?)`,
			book, t, defaultColor(t)); err != nil {
			return err
		}
	}
	return nil
}

func peerExists(tx *sql.Tx, book, id string) (bool, error) {
	var n int
	err := tx.QueryRow(`SELECT count(*) FROM peers WHERE book = ? AND id = ?`, book, id).Scan(&n)
	return n > 0, err
}

// palette holds opaque ARGB colors for tags created without one.
var palette = []int64{
	0xFF2A8B8B, 0xFFE67E22, 0xFF3B82F6, 0xFF8B5CF6,
	0xFFEF4444, 0xFF10B981, 0xFFEAB308, 0xFFEC4899,
}

func defaultColor(name string) int64 {
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(name)))
	return palette[h.Sum32()%uint32(len(palette))]
}

// NormalizeTags trims the names, drops empty ones and duplicates.
func NormalizeTags(tags []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

func encodeTags(tags []string) string {
	b, _ := json.Marshal(NormalizeTags(tags))
	return string(b)
}

func decodeTags(raw string) []string {
	var tags []string
	if json.Unmarshal([]byte(raw), &tags) != nil || tags == nil {
		return []string{}
	}
	return tags
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
