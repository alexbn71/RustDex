//go:build !linux

package main

// dropPrivileges only matters in the Linux container.
func dropPrivileges(dataDir string) error { return nil }
