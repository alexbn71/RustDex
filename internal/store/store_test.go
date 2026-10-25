package store

import (
	"strings"
	"testing"
)

func TestPasswordsEncryptedAtRest(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.InsertPeer(BookShared, Peer{ID: "1", Password: "very-secret"}); err != nil {
		t.Fatal(err)
	}
	var raw string
	st.db.QueryRow(`SELECT password_enc FROM peers WHERE id = '1'`).Scan(&raw)
	if raw == "" || strings.Contains(raw, "very-secret") {
		t.Fatalf("password stored in clear: %q", raw)
	}
	p, err := st.GetPeer(BookShared, "1")
	if err != nil || p.Password != "very-secret" {
		t.Fatalf("read back: %+v %v", p, err)
	}
	st.Close()

	// Same key file: opens fine. Different key: refused.
	st, err = Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	if _, err := Open(dir, "another key"); err == nil {
		t.Fatal("opening with a different key must fail")
	}
}

func TestRenameTagMerges(t *testing.T) {
	st, err := Open(t.TempDir(), "k")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.InsertPeer(BookShared, Peer{ID: "1", Tags: []string{"a", "b"}})
	if err := st.RenameTag(BookShared, "a", "b"); err != nil {
		t.Fatal(err)
	}
	p, _ := st.GetPeer(BookShared, "1")
	tags, _ := st.ListTags(BookShared)
	if len(p.Tags) != 1 || p.Tags[0] != "b" || len(tags) != 1 {
		t.Fatalf("merge: %v %v", p.Tags, tags)
	}
	if err := st.RenameTag(BookShared, "missing", "x"); err != ErrNotFound {
		t.Fatalf("missing tag: %v", err)
	}
}
