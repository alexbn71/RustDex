// Package store keeps the address book in a SQLite database.
// Peer passwords are encrypted at rest with AES-256-GCM.
package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("not found")
	ErrExists   = errors.New("already exists")
)

// Address books: the personal one keeps password hashes (as the RustDesk
// client wants), the shared one keeps the passwords themselves.
const (
	BookPersonal = "personal"
	BookShared   = "shared"
)

var schema = []string{
	`CREATE TABLE IF NOT EXISTS meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS sessions (
		token_hash TEXT PRIMARY KEY,
		client     TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL,
		expires_at INTEGER NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS peers (
		book               TEXT NOT NULL,
		id                 TEXT NOT NULL,
		alias              TEXT NOT NULL DEFAULT '',
		password_enc       TEXT NOT NULL DEFAULT '',
		hash               TEXT NOT NULL DEFAULT '',
		username           TEXT NOT NULL DEFAULT '',
		hostname           TEXT NOT NULL DEFAULT '',
		platform           TEXT NOT NULL DEFAULT '',
		tags               TEXT NOT NULL DEFAULT '[]',
		note               TEXT NOT NULL DEFAULT '',
		force_always_relay INTEGER NOT NULL DEFAULT 0,
		rdp_port           TEXT NOT NULL DEFAULT '',
		rdp_username       TEXT NOT NULL DEFAULT '',
		login_name         TEXT NOT NULL DEFAULT '',
		company            TEXT NOT NULL DEFAULT '',
		contact            TEXT NOT NULL DEFAULT '',
		email              TEXT NOT NULL DEFAULT '',
		phone              TEXT NOT NULL DEFAULT '',
		address            TEXT NOT NULL DEFAULT '',
		created_at         INTEGER NOT NULL,
		updated_at         INTEGER NOT NULL,
		PRIMARY KEY (book, id)
	)`,
	`CREATE TABLE IF NOT EXISTS tags (
		book  TEXT NOT NULL,
		name  TEXT NOT NULL,
		color INTEGER NOT NULL,
		PRIMARY KEY (book, name)
	)`,
	`CREATE TABLE IF NOT EXISTS devices (
		id         TEXT PRIMARY KEY,
		uuid       TEXT NOT NULL DEFAULT '',
		hostname   TEXT NOT NULL DEFAULT '',
		username   TEXT NOT NULL DEFAULT '',
		os         TEXT NOT NULL DEFAULT '',
		version    TEXT NOT NULL DEFAULT '',
		cpu        TEXT NOT NULL DEFAULT '',
		memory     TEXT NOT NULL DEFAULT '',
		last_seen  INTEGER NOT NULL DEFAULT 0,
		sysinfo_at INTEGER NOT NULL DEFAULT 0
	)`,
}

type Store struct {
	db   *sql.DB
	aead cipher.AEAD
}

// Open creates the data directory if needed, opens the database and checks
// that the secret key matches the one the database was created with.
func Open(dataDir, secret string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory %s: %w", dataDir, err)
	}
	probe := filepath.Join(dataDir, ".write-test")
	if err := os.WriteFile(probe, nil, 0o600); err != nil {
		return nil, fmt.Errorf("data directory %s is not writable (check the owner of the mounted folder): %w", dataDir, err)
	}
	os.Remove(probe)

	key, err := loadKey(dataDir, secret)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	dsn := filepath.Join(dataDir, "rustdex.db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection: no lock contention, and the load is tiny anyway.
	db.SetMaxOpenConns(1)
	for _, stmt := range schema {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return nil, fmt.Errorf("create schema: %w", err)
		}
	}

	s := &Store{db: db, aead: aead}
	if err := s.checkKey(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

// loadKey derives the encryption key from RUSTDEX_SECRET_KEY, or reads it
// from (creating if needed) the secret.key file in the data directory.
func loadKey(dataDir, secret string) ([]byte, error) {
	if secret != "" {
		k := sha256.Sum256([]byte(secret))
		return k[:], nil
	}
	path := filepath.Join(dataDir, "secret.key")
	data, err := os.ReadFile(path)
	if err == nil {
		key, err := hex.DecodeString(strings.TrimSpace(string(data)))
		if err != nil || len(key) != 32 {
			return nil, fmt.Errorf("%s is not a valid key (64 hex characters expected)", path)
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(key)+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("write %s: %w", path, err)
	}
	return key, nil
}

// checkKey stores an encrypted marker on first start and decrypts it on the
// following ones, so a changed key is reported instead of silently losing
// every stored password.
func (s *Store) checkKey() error {
	const marker = "rustdex"
	enc, err := s.GetMeta("key_check")
	if errors.Is(err, ErrNotFound) {
		enc, err = s.encrypt(marker)
		if err != nil {
			return err
		}
		return s.SetMeta("key_check", enc)
	}
	if err != nil {
		return err
	}
	if plain, err := s.decrypt(enc); err != nil || plain != marker {
		return errors.New("the secret key does not match the one this database was created with (RUSTDEX_SECRET_KEY or secret.key changed)")
	}
	return nil
}

func (s *Store) encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := s.aead.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func (s *Store) decrypt(enc string) (string, error) {
	if enc == "" {
		return "", nil
	}
	data, err := base64.StdEncoding.DecodeString(enc)
	if err != nil || len(data) < s.aead.NonceSize() {
		return "", errors.New("corrupted encrypted value")
	}
	n := s.aead.NonceSize()
	plain, err := s.aead.Open(nil, data[:n], data[n:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (s *Store) GetMeta(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return v, err
}

func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO meta (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}
