package store

import "strings"

// Device is what a RustDesk client configured with this API server reports
// about itself (heartbeat and sysinfo). Only IDs present in an address book
// are recorded.
type Device struct {
	ID        string
	UUID      string
	Hostname  string
	Username  string
	OS        string
	Version   string
	CPU       string
	Memory    string
	LastSeen  int64
	SysinfoAt int64
}

func (s *Store) knownID(id string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM peers WHERE id = ?`, id).Scan(&n)
	return n > 0, err
}

// Heartbeat records that the device is online. needSysinfo reports that no
// system information was received yet for a known device.
func (s *Store) Heartbeat(id string, now int64) (needSysinfo bool, err error) {
	known, err := s.knownID(id)
	if err != nil || !known {
		return false, err
	}
	_, err = s.db.Exec(`INSERT INTO devices (id, last_seen) VALUES (?, ?)
		ON CONFLICT (id) DO UPDATE SET last_seen = excluded.last_seen`, id, now)
	if err != nil {
		return false, err
	}
	var at int64
	err = s.db.QueryRow(`SELECT sysinfo_at FROM devices WHERE id = ?`, id).Scan(&at)
	return at == 0, err
}

// SaveSysinfo stores the system information of a known device and fills the
// empty hostname, username and platform of its address book entries.
func (s *Store) SaveSysinfo(d Device, now int64) error {
	known, err := s.knownID(d.ID)
	if err != nil || !known {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO devices (id, uuid, hostname, username, os, version, cpu, memory, last_seen, sysinfo_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET uuid = excluded.uuid, hostname = excluded.hostname,
			username = excluded.username, os = excluded.os, version = excluded.version, cpu = excluded.cpu,
			memory = excluded.memory, last_seen = excluded.last_seen, sysinfo_at = excluded.sysinfo_at`,
		d.ID, d.UUID, d.Hostname, d.Username, d.OS, d.Version, d.CPU, d.Memory, now, now)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE peers SET
		hostname = CASE WHEN hostname = '' THEN ? ELSE hostname END,
		username = CASE WHEN username = '' THEN ? ELSE username END,
		platform = CASE WHEN platform = '' THEN ? ELSE platform END
		WHERE id = ?`, d.Hostname, d.Username, platformFromOS(d.OS), d.ID)
	return err
}

func (s *Store) Devices() (map[string]Device, error) {
	rows, err := s.db.Query(`SELECT id, uuid, hostname, username, os, version, cpu, memory, last_seen, sysinfo_at FROM devices`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Device{}
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.UUID, &d.Hostname, &d.Username, &d.OS, &d.Version, &d.CPU, &d.Memory,
			&d.LastSeen, &d.SysinfoAt); err != nil {
			return nil, err
		}
		out[d.ID] = d
	}
	return out, rows.Err()
}

// platformFromOS maps the sysinfo OS string to the platform names the
// RustDesk client uses for its icons.
func platformFromOS(os string) string {
	l := strings.ToLower(os)
	switch {
	case strings.Contains(l, "windows"):
		return "Windows"
	case strings.Contains(l, "android"):
		return "Android"
	case strings.Contains(l, "mac"), strings.Contains(l, "darwin"):
		return "Mac OS"
	case strings.Contains(l, "linux"):
		return "Linux"
	}
	return ""
}
