// Package config reads the RustDex settings from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Listen        string        // RUSTDEX_LISTEN, default ":21114"
	DataDir       string        // RUSTDEX_DATA_DIR, default "/data"
	AdminUser     string        // RUSTDEX_ADMIN_USER, default "admin"
	AdminPassword string        // RUSTDEX_ADMIN_PASSWORD, empty = generated on first start
	SecretKey     string        // RUSTDEX_SECRET_KEY, empty = key file in the data dir
	BookName      string        // RUSTDEX_BOOK_NAME, default "RustDex"
	SessionTTL    time.Duration // RUSTDEX_SESSION_DAYS, default 30
	TrustProxy    bool          // RUSTDEX_TRUST_PROXY, default false
}

func Load() (Config, error) {
	c := Config{
		Listen:        env("RUSTDEX_LISTEN", ":21114"),
		DataDir:       env("RUSTDEX_DATA_DIR", "/data"),
		AdminUser:     env("RUSTDEX_ADMIN_USER", "admin"),
		AdminPassword: os.Getenv("RUSTDEX_ADMIN_PASSWORD"),
		SecretKey:     os.Getenv("RUSTDEX_SECRET_KEY"),
		BookName:      env("RUSTDEX_BOOK_NAME", "RustDex"),
	}

	days, err := strconv.Atoi(env("RUSTDEX_SESSION_DAYS", "30"))
	if err != nil || days < 1 {
		return c, fmt.Errorf("RUSTDEX_SESSION_DAYS must be a positive number of days")
	}
	c.SessionTTL = time.Duration(days) * 24 * time.Hour

	switch strings.ToLower(env("RUSTDEX_TRUST_PROXY", "false")) {
	case "1", "true", "yes":
		c.TrustProxy = true
	case "0", "false", "no":
	default:
		return c, fmt.Errorf("RUSTDEX_TRUST_PROXY must be true or false")
	}
	return c, nil
}

func env(name, def string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return def
}
