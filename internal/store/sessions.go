package store

import (
	"database/sql"
	"errors"
)

// Sessions are stored by the SHA-256 of their token, never the token itself.

func (s *Store) CreateSession(tokenHash, client string, createdAt, expiresAt int64) error {
	_, err := s.db.Exec(`INSERT INTO sessions (token_hash, client, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		tokenHash, client, createdAt, expiresAt)
	return err
}

// SessionExpiry returns the expiry time of a session, ErrNotFound if unknown.
func (s *Store) SessionExpiry(tokenHash string) (int64, error) {
	var exp int64
	err := s.db.QueryRow(`SELECT expires_at FROM sessions WHERE token_hash = ?`, tokenHash).Scan(&exp)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return exp, err
}

func (s *Store) ExtendSession(tokenHash string, expiresAt int64) error {
	_, err := s.db.Exec(`UPDATE sessions SET expires_at = ? WHERE token_hash = ?`, expiresAt, tokenHash)
	return err
}

func (s *Store) DeleteSession(tokenHash string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

func (s *Store) PurgeSessions(now int64) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, now)
	return err
}
