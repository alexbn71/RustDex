//go:build linux

package main

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// dropPrivileges runs when the container starts as root: it hands the data
// directory to PUID:PGID (default 65532, on Unraid usually 99:100) and
// switches to that user, so bind-mounted folders created by root work.
// PUID=0 keeps running as root.
func dropPrivileges(dataDir string) error {
	if os.Getuid() != 0 {
		return nil
	}
	uid, err := idEnv("PUID")
	if err != nil {
		return err
	}
	gid, err := idEnv("PGID")
	if err != nil {
		return err
	}
	if uid == 0 {
		return nil
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("create data directory %s: %w", dataDir, err)
	}
	err = filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(path, uid, gid)
	})
	if err != nil {
		return fmt.Errorf("give %s to %d:%d: %w", dataDir, uid, gid, err)
	}
	if err := syscall.Setgroups([]int{}); err != nil {
		return fmt.Errorf("setgroups: %w", err)
	}
	if err := syscall.Setgid(gid); err != nil {
		return fmt.Errorf("setgid %d: %w", gid, err)
	}
	if err := syscall.Setuid(uid); err != nil {
		return fmt.Errorf("setuid %d: %w", uid, err)
	}
	log.Printf("running as %d:%d", uid, gid)
	return nil
}

func idEnv(name string) (int, error) {
	v := os.Getenv(name)
	if v == "" {
		return 65532, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a numeric user or group id", name)
	}
	return n, nil
}
