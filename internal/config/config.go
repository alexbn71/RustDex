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
	AutoAdd       bool          // RUSTDEX_AUTO_ADD, default true
	AutoAddTag    string        // RUSTDEX_AUTO_ADD_TAG, default "New"
}

func Load() (Config, error) {
	c := Config{
		Listen:        env("RUSTDEX_LISTEN", ":21114"),
		DataDir:       env("RUSTDEX_DATA_DIR", "/data"),
		AdminUser:     env("RUSTDEX_ADMIN_USER", "admin"),
		AdminPassword: os.Getenv("RUSTDEX_ADMIN_PASSWORD"),
		SecretKey:     os.Getenv("RUSTDEX_SECRET_KEY"),
		BookName:      env("RUSTDEX_BOOK_NAME", "RustDex"),
		AutoAddTag:    env("RUSTDEX_AUTO_ADD_TAG", "New"),
	}
	// "-" disables the tag on auto-added computers.
	if c.AutoAddTag == "-" {
		c.AutoAddTag = ""
	}

	days, err := strconv.Atoi(env("RUSTDEX_SESSION_DAYS", "30"))
	if err != nil || days < 1 {
		return c, fmt.Errorf("RUSTDEX_SESSION_DAYS must be a positive number of days")
	}
	c.SessionTTL = time.Duration(days) * 24 * time.Hour

	if c.TrustProxy, err = boolEnv("RUSTDEX_TRUST_PROXY", false); err != nil {
		return c, err
	}
	if c.AutoAdd, err = boolEnv("RUSTDEX_AUTO_ADD", true); err != nil {
		return c, err
	}
	return c, nil
}

func boolEnv(name string, def bool) (bool, error) {
	switch strings.ToLower(env(name, "")) {
	case "":
		return def, nil
	case "1", "true", "yes":
		return true, nil
	case "0", "false", "no":
		return false, nil
	}
	return false, fmt.Errorf("%s must be true or false", name)
}

func env(name, def string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return def
}
