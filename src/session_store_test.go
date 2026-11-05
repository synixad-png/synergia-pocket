package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type memoryEncryptedStore struct {
	data   map[string][]byte
	crypt  *redisCloudStore
	broken bool
}

func newMemoryStore() *memoryEncryptedStore {
	b, _ := aes.NewCipher(bytes.Repeat([]byte{7}, 32))
	g, _ := cipher.NewGCM(b)
	return &memoryEncryptedStore{data: map[string][]byte{}, crypt: &redisCloudStore{aead: g}}
}
func (s *memoryEncryptedStore) Get(k string) ([]byte, error) {
	if s.broken {
		return nil, errors.New("unavailable")
	}
	v := s.data[storeKey(k)]
	if v == nil {
		return nil, nil
	}
	return s.crypt.open(k, v)
}
func (s *memoryEncryptedStore) Set(k string, b []byte) error {
	v, e := s.crypt.seal(k, b)
	s.data[storeKey(k)] = v
	return e
}
func (s *memoryEncryptedStore) Delete(k string) error {
	if s.broken {
		return errors.New("unavailable")
	}
	delete(s.data, storeKey(k))
	return nil
}
func savedTestClient(t *testing.T, name string) *APIClient {
	t.Helper()
	c, e := newAPIClient(name)
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse("https://synergia.librus.pl/")
	c.jar.SetCookies(u, []*http.Cookie{{Name: "oauth_token", Value: "private-" + name, Path: "/", Secure: true, HttpOnly: true}})
	c.loggedAt = time.Now()
	return c
}
func TestCloudEncryptedTwoProfilesSurviveRestartAndLogoutRevokes(t *testing.T) {
	store := newMemoryStore()
	a := cloudTest(t)
	a.store = store
	cookie := cloudNewBrowser(t, a)
	s := a.sessions[cookie.Value]
	s.app.profiles[0] = savedTestClient(t, "first")
	s.app.profiles[1] = savedTestClient(t, "second")
	if e := a.persist(cookie.Value, s); e != nil {
		t.Fatal(e)
	}
	for k, b := range store.data {
		if strings.Contains(k, cookie.Value) || bytes.Contains(b, []byte("private-first")) || bytes.Contains(b, []byte("first")) {
			t.Fatal("plaintext secret persisted")
		}
	}
	a.close()
	b := cloudTest(t)
	b.store = store
	w := cloudRequest(t, b, "GET", "/api/updates/profiles", cookie, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"logged":[true,true]`) {
		t.Fatal(w.Code, w.Body)
	}
	if w.Result().Cookies()[0].MaxAge != int(cloudSessionIdle.Seconds()) {
		t.Fatal("cookie not renewed")
	}
	restored := b.sessions[cookie.Value]
	if restored.app.profiles[0].username != "first" || restored.app.profiles[1].username != "second" {
		t.Fatal("profiles crossed")
	}
	other := cloudNewBrowser(t, b)
	if other.Value == cookie.Value {
		t.Fatal("shared browser key")
	}
	store.broken = true
	w = cloudRequest(t, b, "POST", "/api/exit", cookie, b.origin)
	if w.Code != 503 || restored.revoked {
		t.Fatal("logout falsely acknowledged")
	}
	store.broken = false
	w = cloudRequest(t, b, "POST", "/api/exit", cookie, b.origin)
	if w.Code != 200 || len(store.data) != 0 {
		t.Fatal(w.Code, w.Body)
	}
	if e := b.persist(cookie.Value, restored); e != nil || len(store.data) != 0 {
		t.Fatal("inflight save resurrected logout")
	}
	c := cloudTest(t)
	c.store = store
	if w = cloudRequest(t, c, "GET", "/api/updates/profiles", cookie, ""); w.Code != 401 {
		t.Fatal("revoked session restored")
	}
}
func TestCloudRestoreStoreFailureDoesNotReplaceCookie(t *testing.T) {
	a := cloudTest(t)
	s := newMemoryStore()
	a.store = s
	cookie := cloudNewBrowser(t, a)
	a.sessions[cookie.Value].app.profiles[0] = savedTestClient(t, "one")
	if e := a.persist(cookie.Value, a.sessions[cookie.Value]); e != nil {
		t.Fatal(e)
	}
	a.close()
	b := cloudTest(t)
	b.store = s
	s.broken = true
	w := cloudRequest(t, b, "GET", "/", cookie, "")
	if w.Code != 503 || len(w.Result().Cookies()) != 0 || len(b.sessions) != 0 {
		t.Fatal("transient failure cleared/replaced session")
	}
}
func TestEncryptedRecordCannotBeMovedBetweenSessions(t *testing.T) {
	s := newMemoryStore()
	v, _ := s.crypt.seal("a", []byte("secret"))
	if _, e := s.crypt.open("b", v); e == nil {
		t.Fatal("session binding missing")
	}
	v[len(v)-1] ^= 1
	if _, e := s.crypt.open("a", v); e == nil {
		t.Fatal("corrupt record accepted")
	}
}
func TestPersistentCookieJarPreservesPathsAndExpiration(t *testing.T) {
	j := newPersistentJar()
	u, _ := url.Parse("https://synergia.librus.pl/oauth/start")
	j.SetCookies(u, []*http.Cookie{{Name: "test", Value: "secret", MaxAge: 100}})
	entries := j.snapshot()
	if len(entries) != 1 || entries[0].Cookie.Path != "/oauth" || entries[0].Cookie.MaxAge != 0 || entries[0].Cookie.Expires.IsZero() {
		t.Fatal(entries)
	}
	j.SetCookies(u, []*http.Cookie{{Name: "test", Value: "", MaxAge: -1}})
	if len(j.snapshot()) != 0 {
		t.Fatal("deleted cookie persisted")
	}
}

func TestOldCookieRequiresProviderRejectionNotLocalAge(t *testing.T) {
	a, m := fakeAccount()
	a.loggedAt = time.Now().Add(-25 * time.Hour)

	m.add("GET", dataBase+"/Me", record{200, `{"Me":{"Id":1}}`, "", ""})
	a.confirmProviderCookie(context.Background())
	m.add("GET", dataBase+"/Me", record{200, `{"Me":{"Id":1}}`, "", ""})
	if _, err := a.get(context.Background(), dataBase+"/Me", "Me"); err != nil {
		t.Fatal("valid provider cookie rejected by local clock", err)
	}
	m.add("GET", dataBase+"/Me", record{401, "expired", "", ""})
	m.add("GET", refresh, record{403, "expired", "", ""})
	if _, err := a.get(context.Background(), dataBase+"/Me", "Me"); err == nil {
		t.Fatal("provider rejection ignored")
	}
}
