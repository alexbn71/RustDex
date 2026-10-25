package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/alexbn71/RustDex/internal/config"
	"github.com/alexbn71/RustDex/internal/store"
)

type testEnv struct {
	t     *testing.T
	srv   *Server
	st    *store.Store
	token string
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := config.Config{
		AdminUser:     "admin",
		AdminPassword: "secret",
		BookName:      "RustDex",
		SessionTTL:    24 * time.Hour,
		AutoAdd:       true,
		AutoAddTag:    "New",
	}
	static := fstest.MapFS{"index.html": {Data: []byte("<html>rustdex</html>")}}
	srv, err := New(cfg, st, static, "test")
	if err != nil {
		t.Fatal(err)
	}
	return &testEnv{t: t, srv: srv, st: st}
}

func (e *testEnv) do(method, path string, body any) (int, []byte) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	req.RemoteAddr = "192.0.2.1:1234"
	if e.token != "" {
		req.Header.Set("Authorization", "Bearer "+e.token)
	}
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func (e *testEnv) json(method, path string, body any, out any) int {
	e.t.Helper()
	code, data := e.do(method, path, body)
	if out != nil {
		// Unmarshal merges into existing maps and slices: start from zero.
		reflect.ValueOf(out).Elem().SetZero()
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			e.t.Fatalf("%s %s: invalid JSON %q: %v", method, path, data, err)
		}
	}
	return code
}

func (e *testEnv) login() {
	e.t.Helper()
	var resp struct {
		Type        string         `json:"type"`
		AccessToken string         `json:"access_token"`
		User        map[string]any `json:"user"`
	}
	code := e.json("POST", "/api/login", map[string]any{
		"username": "admin", "password": "secret", "id": "123456789", "uuid": "u", "type": "account",
		"autoLogin": true, "deviceInfo": map[string]any{"os": "windows"},
	}, &resp)
	if code != 200 || resp.Type != "access_token" || resp.AccessToken == "" || resp.User["name"] != "admin" {
		e.t.Fatalf("login failed: %d %+v", code, resp)
	}
	e.token = resp.AccessToken
}

func TestLoginAndSession(t *testing.T) {
	e := newEnv(t)

	if code := e.json("POST", "/api/currentUser", map[string]any{}, nil); code != 401 {
		t.Fatalf("currentUser without token: %d", code)
	}
	var bad map[string]string
	if code := e.json("POST", "/api/login", map[string]any{"username": "admin", "password": "nope"}, &bad); code != 401 || bad["error"] == "" {
		t.Fatalf("wrong password: %d %v", code, bad)
	}

	e.login()
	var user map[string]any
	if code := e.json("POST", "/api/currentUser", map[string]any{"id": "1", "uuid": "u"}, &user); code != 200 || user["name"] != "admin" {
		t.Fatalf("currentUser: %d %v", code, user)
	}

	e.json("POST", "/api/logout", map[string]any{}, nil)
	if code := e.json("POST", "/api/currentUser", map[string]any{}, nil); code != 401 {
		t.Fatalf("currentUser after logout: %d", code)
	}
}

func TestLoginLockout(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < maxLoginFails; i++ {
		e.json("POST", "/api/login", map[string]any{"username": "admin", "password": "nope"}, nil)
	}
	if code := e.json("POST", "/api/login", map[string]any{"username": "admin", "password": "secret"}, nil); code != 429 {
		t.Fatalf("expected lockout, got %d", code)
	}
}

func TestClientAddressBookFlow(t *testing.T) {
	e := newEnv(t)
	e.login()

	var personal struct{ GUID string }
	e.json("POST", "/api/ab/personal", nil, &personal)
	var profiles struct {
		Total int
		Data  []struct {
			GUID string
			Name string
			Rule int
		}
	}
	e.json("POST", "/api/ab/shared/profiles?current=1&pageSize=100", nil, &profiles)
	if personal.GUID == "" || profiles.Total != 1 || profiles.Data[0].Name != "RustDex" || profiles.Data[0].Rule != 3 {
		t.Fatalf("books: %+v %+v", personal, profiles)
	}
	shared := profiles.Data[0].GUID

	// Add with password and hash: the shared book keeps only the password.
	if code := e.json("POST", "/api/ab/tag/add/"+shared, map[string]any{"name": "Clienti", "color": 4278190335}, nil); code != 200 {
		t.Fatalf("tag add: %d", code)
	}
	peer := map[string]any{"id": "987654321", "alias": "PC Ufficio", "tags": []string{"Clienti"},
		"password": "pw1", "hash": "h1", "note": "n"}
	if code := e.json("POST", "/api/ab/peer/add/"+shared, peer, nil); code != 200 {
		t.Fatalf("peer add: %d", code)
	}
	e.json("PUT", "/api/ab/peer/update/"+shared, map[string]any{"id": "987654321", "password": "pw2"}, nil)
	e.json("PUT", "/api/ab/peer/update/"+shared, map[string]any{"id": "987654321", "username": "mario", "hostname": "PC-01", "platform": "Windows"}, nil)

	var peers struct {
		Total int
		Data  []map[string]any
	}
	e.json("POST", "/api/ab/peers?current=1&pageSize=100&ab="+shared, nil, &peers)
	if peers.Total != 1 {
		t.Fatalf("peers: %+v", peers)
	}
	got := peers.Data[0]
	if got["password"] != "pw2" || got["hash"] != nil || got["alias"] != "PC Ufficio" || got["hostname"] != "PC-01" ||
		got["note"] != "n" || got["forceAlwaysRelay"] != "false" {
		t.Fatalf("shared peer: %v", got)
	}

	// The personal book keeps the hash, never the password.
	e.json("POST", "/api/ab/peer/add/"+personal.GUID, map[string]any{"id": "111", "password": "x", "hash": "h"}, nil)
	e.json("POST", "/api/ab/peers?current=1&pageSize=100&ab="+personal.GUID, nil, &peers)
	if peers.Total != 1 || peers.Data[0]["hash"] != "h" || peers.Data[0]["password"] != nil {
		t.Fatalf("personal peer: %+v", peers)
	}

	// Tag rename and delete follow through to the peers.
	e.json("PUT", "/api/ab/tag/rename/"+shared, map[string]any{"old": "Clienti", "new": "Customers"}, nil)
	var tags []struct {
		Name  string
		Color int64
	}
	e.json("POST", "/api/ab/tags/"+shared, nil, &tags)
	if len(tags) != 1 || tags[0].Name != "Customers" || tags[0].Color != 4278190335 {
		t.Fatalf("tags after rename: %+v", tags)
	}
	e.json("POST", "/api/ab/peers?ab="+shared, nil, &peers)
	if tl := peers.Data[0]["tags"].([]any); len(tl) != 1 || tl[0] != "Customers" {
		t.Fatalf("peer tags after rename: %v", peers.Data[0]["tags"])
	}
	e.do("DELETE", "/api/ab/tag/"+shared, []string{"Customers"})
	e.json("POST", "/api/ab/peers?ab="+shared, nil, &peers)
	if tl := peers.Data[0]["tags"].([]any); len(tl) != 0 {
		t.Fatalf("peer tags after delete: %v", tl)
	}

	e.do("DELETE", "/api/ab/peer/"+shared, []string{"987654321"})
	e.json("POST", "/api/ab/peers?ab="+shared, nil, &peers)
	if peers.Total != 0 {
		t.Fatalf("peers after delete: %+v", peers)
	}

	if code, _ := e.do("POST", "/api/ab/peers?ab=unknown", nil); code != 404 {
		t.Fatalf("unknown book: %d", code)
	}
}

func TestWebAPI(t *testing.T) {
	e := newEnv(t)
	e.login()

	pw := "s3cret"
	in := map[string]any{"id": " 222333444 ", "alias": "Reception", "company": "ACME", "contact": "Anna",
		"email": "a@acme.test", "phone": "+39 02 1234", "address": "Via Roma 1", "note": "piano terra",
		"tags": []string{"ACME", " ACME", ""}, "password": pw}
	if code := e.json("POST", "/rx/api/peers", in, nil); code != 201 {
		t.Fatalf("create: %d", code)
	}
	if code := e.json("POST", "/rx/api/peers", in, nil); code != 409 {
		t.Fatalf("duplicate create: %d", code)
	}

	var list struct {
		Peers []webPeer
		Tags  []map[string]any
	}
	e.json("GET", "/rx/api/peers", nil, &list)
	if len(list.Peers) != 1 || list.Peers[0].ID != "222333444" || !list.Peers[0].HasPassword ||
		len(list.Peers[0].Tags) != 1 || list.Peers[0].Company != "ACME" || len(list.Tags) != 1 {
		t.Fatalf("list: %+v", list)
	}
	if strings.Contains(mustBody(e, "GET", "/rx/api/peers"), pw) {
		t.Fatal("the list must not contain passwords")
	}

	var got map[string]string
	e.json("GET", "/rx/api/peers/222333444/password", nil, &got)
	if got["password"] != pw {
		t.Fatalf("password: %v", got)
	}

	// Update without password keeps it; ID change is allowed.
	in["id"] = "555666777"
	delete(in, "password")
	if code := e.json("PUT", "/rx/api/peers/222333444", in, nil); code != 200 {
		t.Fatalf("update: %d", code)
	}
	e.json("GET", "/rx/api/peers/555666777/password", nil, &got)
	if got["password"] != pw {
		t.Fatalf("password after update: %v", got)
	}
	// Empty password removes it.
	in["password"] = ""
	e.json("PUT", "/rx/api/peers/555666777", in, nil)
	e.json("GET", "/rx/api/peers", nil, &list)
	if list.Peers[0].HasPassword {
		t.Fatal("password should be removed")
	}

	// Tag rename + color through the web API.
	color := int64(0xFF00FF00)
	e.json("PUT", "/rx/api/tags/ACME", map[string]any{"name": "Acme Spa", "color": color}, nil)
	e.json("GET", "/rx/api/peers", nil, &list)
	if list.Peers[0].Tags[0] != "Acme Spa" || int64(list.Tags[0]["color"].(float64)) != color {
		t.Fatalf("tag update: %+v", list)
	}

	e.do("DELETE", "/rx/api/peers/555666777", nil)
	e.json("GET", "/rx/api/peers", nil, &list)
	if len(list.Peers) != 0 {
		t.Fatalf("after delete: %+v", list.Peers)
	}

	if code := e.json("POST", "/rx/api/peers", map[string]any{"id": "a/b"}, nil); code != 400 {
		t.Fatalf("invalid id: %d", code)
	}

	// IDs copied from RustDesk come grouped with spaces.
	if code := e.json("POST", "/rx/api/peers", map[string]any{"id": " 71 114 852 "}, nil); code != 201 {
		t.Fatalf("spaced id: %d", code)
	}
	e.json("GET", "/rx/api/peers", nil, &list)
	if len(list.Peers) != 1 || list.Peers[0].ID != "71114852" {
		t.Fatalf("spaced id stored as: %+v", list.Peers)
	}
}

func TestHeartbeatAndSysinfo(t *testing.T) {
	e := newEnv(t)
	e.login()
	e.json("POST", "/rx/api/peers", map[string]any{"id": "123123123"}, nil)
	e.token = ""

	// Unknown device: nothing recorded, sysinfo still acknowledged.
	var hb map[string]any
	e.json("POST", "/api/heartbeat", map[string]any{"id": "999", "uuid": "u", "ver": 1}, &hb)
	if len(hb) != 0 {
		t.Fatalf("heartbeat unknown: %v", hb)
	}
	if _, body := e.do("POST", "/api/sysinfo", map[string]any{"id": "999"}); string(body) != "SYSINFO_UPDATED" {
		t.Fatalf("sysinfo: %q", body)
	}

	// Known device without sysinfo: asked to upload it.
	e.json("POST", "/api/heartbeat", map[string]any{"id": "123123123", "uuid": "u", "ver": 1}, &hb)
	if _, ok := hb["sysinfo"]; !ok {
		t.Fatalf("heartbeat should request sysinfo: %v", hb)
	}
	e.do("POST", "/api/sysinfo", map[string]any{"id": "123123123", "hostname": "PC-02", "username": "luca",
		"os": "Windows 11 Pro", "version": "1.4.3", "cpu": "x", "memory": "16GB", "uuid": "u"})
	e.json("POST", "/api/heartbeat", map[string]any{"id": "123123123"}, &hb)
	if len(hb) != 0 {
		t.Fatalf("heartbeat after sysinfo: %v", hb)
	}

	e.login()
	var list struct{ Peers []webPeer }
	e.json("GET", "/rx/api/peers", nil, &list)
	p := list.Peers[0]
	if p.LastSeen == 0 || p.Hostname != "PC-02" || p.Username != "luca" || p.Platform != "Windows" || p.Version != "1.4.3" {
		t.Fatalf("device info: %+v", p)
	}
}

func TestAutoAddFromAudit(t *testing.T) {
	e := newEnv(t)
	e.login() // our client is 123456789
	e.token = ""

	audit := func(body map[string]any) {
		body["uuid"], body["conn_id"], body["session_id"] = "u", 1, 2
		if code, _ := e.do("POST", "/api/audit/conn", body); code != 200 {
			t.Fatalf("audit: %d", code)
		}
	}
	// Before authorization (no "peer"), from a stranger, and file audits: ignored.
	audit(map[string]any{"id": "444555666", "action": "new", "ip": "192.0.2.9"})
	audit(map[string]any{"id": "444555666", "peer": []any{"999999999", "someone"}, "type": 0})
	e.do("POST", "/api/audit/file", map[string]any{"id": "444555666", "peer": []any{"123456789", "me"}})
	// Connection from our client: added once, with the tag.
	audit(map[string]any{"id": "444555666", "peer": []any{"123456789", "me"}, "type": 0})
	audit(map[string]any{"id": "444555666", "peer": []any{"123456789", "me"}, "type": 1})

	e.login()
	var list struct{ Peers []webPeer }
	e.json("GET", "/rx/api/peers", nil, &list)
	if len(list.Peers) != 1 || list.Peers[0].ID != "444555666" || len(list.Peers[0].Tags) != 1 || list.Peers[0].Tags[0] != "New" {
		t.Fatalf("auto added: %+v", list.Peers)
	}

	// An existing entry is left alone.
	e.json("PUT", "/rx/api/peers/444555666", map[string]any{"id": "444555666", "alias": "Done", "tags": []string{}}, nil)
	e.token = ""
	audit(map[string]any{"id": "444555666", "peer": []any{"123456789", "me"}, "type": 0})
	e.login()
	e.json("GET", "/rx/api/peers", nil, &list)
	if list.Peers[0].Alias != "Done" || len(list.Peers[0].Tags) != 0 {
		t.Fatalf("existing entry changed: %+v", list.Peers[0])
	}

	// Disabled: nothing is added.
	e.srv.cfg.AutoAdd = false
	e.token = ""
	audit(map[string]any{"id": "777888999", "peer": []any{"123456789", "me"}, "type": 0})
	e.login()
	e.json("GET", "/rx/api/peers", nil, &list)
	if len(list.Peers) != 1 {
		t.Fatalf("added while disabled: %+v", list.Peers)
	}
}

func TestStaticAndUnknown(t *testing.T) {
	e := newEnv(t)
	if code, body := e.do("GET", "/", nil); code != 200 || !strings.Contains(string(body), "rustdex") {
		t.Fatalf("index: %d %q", code, body)
	}
	if code, _ := e.do("GET", "/api/ab", nil); code != 404 {
		t.Fatalf("legacy ab should be 404: %d", code)
	}
	var opts []any
	if code := e.json("GET", "/api/login-options", nil, &opts); code != 200 || len(opts) != 0 {
		t.Fatalf("login-options: %d %v", code, opts)
	}
}

func TestGeneratedAdminPassword(t *testing.T) {
	hash, err := hashPassword("abc")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyHash(hash, "abc") || verifyHash(hash, "abd") || verifyHash("garbage", "abc") {
		t.Fatal("password hash verification is wrong")
	}
}

func mustBody(e *testEnv, method, path string) string {
	_, b := e.do(method, path, nil)
	return string(b)
}
