package main

// Encrypted, server-side persistence. Only cookies are kept, never passwords.
import (
	"bufio"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type storedCookie struct {
	URL    string
	Cookie http.Cookie
}
type persistentJar struct {
	jar    http.CookieJar
	mu     sync.Mutex
	values map[string]storedCookie
}

func newPersistentJar() *persistentJar {
	j, _ := cookiejar.New(nil)
	return &persistentJar{jar: j, values: map[string]storedCookie{}}
}
func (j *persistentJar) Cookies(u *url.URL) []*http.Cookie { return j.jar.Cookies(u) }
func (j *persistentJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.jar.SetCookies(u, cookies)
	for _, c := range cookies {
		copy := *c
		if copy.Path == "" {
			copy.Path = u.Path
			if n := strings.LastIndex(copy.Path, "/"); n >= 0 {
				copy.Path = copy.Path[:n]
			}
			if copy.Path == "" {
				copy.Path = "/"
			}
		}
		domain := strings.ToLower(strings.TrimPrefix(copy.Domain, "."))
		if domain == "" {
			domain = u.Hostname()
		}
		key := domain + "\x00" + copy.Path + "\x00" + copy.Name
		if copy.MaxAge < 0 || (!copy.Expires.IsZero() && copy.Expires.Before(time.Now())) {
			delete(j.values, key)
			continue
		}
		if copy.MaxAge > 0 {
			copy.Expires = time.Now().Add(time.Duration(copy.MaxAge) * time.Second)
			copy.MaxAge = 0
		}
		j.values[key] = storedCookie{URL: u.Scheme + "://" + u.Host + copy.Path, Cookie: copy}
	}
}
func (j *persistentJar) snapshot() []storedCookie {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := []storedCookie{}
	for _, c := range j.values {
		if c.Cookie.Expires.IsZero() || c.Cookie.Expires.After(time.Now()) {
			out = append(out, c)
		}
	}
	return out
}

type savedProfile struct {
	Username string
	LoggedAt time.Time
	Cookies  []storedCookie
}
type savedCloudSession struct {
	Version  int
	Origin   string
	Seen     time.Time
	Profiles [2]*savedProfile
}
type cloudSessionStore interface {
	Get(string) ([]byte, error)
	Set(string, []byte) error
	Delete(string) error
}
type redisCloudStore struct {
	address  string
	password string
	username string
	tls      bool
	aead     cipher.AEAD
}

func configuredCloudStore() (cloudSessionStore, error) {
	raw := strings.TrimSpace(os.Getenv("SYNERGIA_REDIS_URL"))
	key := strings.TrimSpace(os.Getenv("SYNERGIA_SESSION_FERNET_KEY"))
	if raw == "" && key == "" {
		return nil, nil
	}
	if raw == "" || key == "" {
		return nil, errors.New("session storage needs Redis and encryption key")
	}
	u, e := url.Parse(raw)
	if e != nil {
		return nil, errors.New("invalid Redis configuration")
	}
	private := u.Scheme == "redis" && os.Getenv("RENDER") == "true" && strings.HasPrefix(u.Hostname(), "red-") && u.Port() == "6379" && u.User == nil
	if (u.Scheme != "rediss" && !private) || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("Redis requires TLS or private Render network")
	}
	b, e := base64.StdEncoding.DecodeString(key)
	if e != nil || len(b) != 32 {
		return nil, errors.New("invalid session encryption key")
	}
	block, e := aes.NewCipher(b)
	if e != nil {
		return nil, e
	}
	g, e := cipher.NewGCM(block)
	if e != nil {
		return nil, e
	}
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), "6379")
	}
	s := &redisCloudStore{address: host, tls: u.Scheme == "rediss", aead: g}
	if u.User != nil {
		s.username = u.User.Username()
		s.password, _ = u.User.Password()
	}
	return s, nil
}
func storeKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return "synergia:pocket:own:session:" + hex.EncodeToString(sum[:])
}
func (s *redisCloudStore) seal(token string, data []byte) ([]byte, error) {
	n := make([]byte, s.aead.NonceSize())
	if _, e := rand.Read(n); e != nil {
		return nil, e
	}
	return s.aead.Seal(n, n, data, []byte(storeKey(token))), nil
}
func (s *redisCloudStore) open(token string, data []byte) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(data) < n {
		return nil, errors.New("invalid encrypted session")
	}
	return s.aead.Open(nil, data[:n], data[n:], []byte(storeKey(token)))
}
func redisReply(r *bufio.Reader) ([]byte, error) {
	line, e := r.ReadString('\n')
	if e != nil {
		return nil, e
	}
	if len(line) < 3 {
		return nil, errors.New("invalid Redis reply")
	}
	v := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	switch v[0] {
	case '+':
		return []byte(v[1:]), nil
	case '-':
		return nil, errors.New("Redis command failed")
	case '$':
		n, e := strconv.Atoi(v[1:])
		if e != nil || n > 2<<20 || n < -1 {
			return nil, errors.New("invalid Redis bulk size")
		}
		if n == -1 {
			return nil, nil
		}
		b := make([]byte, n+2)
		_, e = io.ReadFull(r, b)
		if e != nil {
			return nil, e
		}
		if string(b[n:]) != "\r\n" {
			return nil, errors.New("invalid Redis bulk terminator")
		}
		return b[:n], nil
	case ':':
		return []byte(v[1:]), nil
	}
	return nil, errors.New("unexpected Redis reply")
}
func writeCommand(w io.Writer, args ...[]byte) error {
	if _, e := fmt.Fprintf(w, "*%d\r\n", len(args)); e != nil {
		return e
	}
	for _, a := range args {
		if _, e := fmt.Fprintf(w, "$%d\r\n", len(a)); e != nil {
			return e
		}
		if _, e := w.Write(a); e != nil {
			return e
		}
		if _, e := io.WriteString(w, "\r\n"); e != nil {
			return e
		}
	}
	return nil
}
func (s *redisCloudStore) command(args ...[]byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	d := net.Dialer{Timeout: 3 * time.Second}
	var conn net.Conn
	var e error
	if s.tls {
		host, _, _ := net.SplitHostPort(s.address)
		td := tls.Dialer{NetDialer: &d, Config: &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}}
		conn, e = td.DialContext(ctx, "tcp", s.address)
	} else {
		conn, e = d.DialContext(ctx, "tcp", s.address)
	}
	if e != nil {
		return nil, errors.New("session store unavailable")
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	reader := bufio.NewReader(conn)
	if s.password != "" {
		auth := [][]byte{[]byte("AUTH")}
		if s.username != "" {
			auth = append(auth, []byte(s.username))
		}
		auth = append(auth, []byte(s.password))
		if e = writeCommand(conn, auth...); e != nil {
			return nil, e
		}
		if _, e = redisReply(reader); e != nil {
			return nil, e
		}
	}
	if e = writeCommand(conn, args...); e != nil {
		return nil, e
	}
	return redisReply(reader)
}
func (s *redisCloudStore) Get(token string) ([]byte, error) {
	b, e := s.command([]byte("GET"), []byte(storeKey(token)))
	if e != nil || b == nil {
		return nil, e
	}
	return s.open(token, b)
}
func (s *redisCloudStore) Set(token string, b []byte) error {
	data, e := s.seal(token, b)
	if e != nil {
		return e
	}
	_, e = s.command([]byte("SET"), []byte(storeKey(token)), data, []byte("EX"), []byte(strconv.Itoa(int(cloudSessionIdle.Seconds()))))
	return e
}
func (s *redisCloudStore) Delete(token string) error {
	_, e := s.command([]byte("DEL"), []byte(storeKey(token)))
	return e
}

func (c *cloudApp) persist(key string, s *cloudSession) error {
	if c.store == nil {
		return nil
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	if s.revoked {
		return nil
	}
	rec := savedCloudSession{Version: 1, Origin: c.origin, Seen: c.now()}
	has := false
	s.app.mu.RLock()
	profiles := s.app.profiles
	s.app.mu.RUnlock()
	for i, p := range profiles {
		if p == nil {
			continue
		}
		p.mu.Lock()
		jar, ok := p.jar.(*persistentJar)
		if ok && !p.loggedAt.IsZero() {
			rec.Profiles[i] = &savedProfile{Username: p.username, LoggedAt: p.loggedAt, Cookies: jar.snapshot()}
			has = true
		}
		p.mu.Unlock()
	}
	if !has {
		return nil
	}
	b, e := json.Marshal(rec)
	if e != nil {
		return e
	}
	return c.store.Set(key, b)
}
func (c *cloudApp) restore(key string) (*cloudSession, error) {
	if c.store == nil || len(key) != 64 {
		return nil, nil
	}
	if _, e := hex.DecodeString(key); e != nil {
		return nil, nil
	}
	b, e := c.store.Get(key)
	if e != nil || b == nil {
		return nil, e
	}
	var rec savedCloudSession
	if json.Unmarshal(b, &rec) != nil || rec.Version != 1 || rec.Origin != c.origin || c.now().Sub(rec.Seen) > cloudSessionIdle {
		return nil, nil
	}
	a := &LocalApp{token: key, origin: c.origin}
	has := false
	for i, p := range rec.Profiles {
		if p == nil {
			continue
		}
		if p.LoggedAt.IsZero() || len(p.Cookies) > 300 || len(p.Username) > 128 {
			return nil, errors.New("invalid saved session")
		}
		client, e := newAPIClient(p.Username)
		if e != nil {
			return nil, e
		}
		client.loggedAt = p.LoggedAt
		for _, entry := range p.Cookies {
			u, e := url.Parse(entry.URL)
			if e != nil || !validURL(entry.URL, false) {
				return nil, errors.New("invalid cookie origin")
			}
			cookie := entry.Cookie
			client.jar.SetCookies(u, []*http.Cookie{&cookie})
		}
		if client.tokenPresent() {
			a.profiles[i] = client
			has = true
		}
	}
	if !has {
		return nil, nil
	}
	return &cloudSession{app: a, handler: a.routes(), seen: c.now()}, nil
}

// Renew provider cookies while the service is running. Passwords are never used.
func (c *cloudApp) renewSessions() {
	c.mu.Lock()
	items := map[string]*cloudSession{}
	for k, s := range c.sessions {
		items[k] = s
		s.inFlight++
	}
	c.mu.Unlock()
	for key, s := range items {
		s.app.mu.RLock()
		profiles := s.app.profiles
		s.app.mu.RUnlock()
		for _, p := range profiles {
			if p == nil {
				continue
			}
			p.mu.Lock()
			if time.Since(p.loggedAt) > 2*time.Hour {
				ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
				p.confirmProviderCookie(ctx)
				_ = p.ensure(ctx)
				cancel()
			}
			p.mu.Unlock()
		}
		_ = c.persist(key, s)
		c.mu.Lock()
		s.inFlight--
		c.mu.Unlock()
	}
}

// Validate an old cookie against the provider before the client's conservative
// local clock check. Only a successful real Me response advances validation time.
// Caller holds the APIClient mutex. No password or authentication bypass.
func (a *APIClient) confirmProviderCookie(ctx context.Context) {
	if time.Since(a.loggedAt) < 19*time.Hour || !a.tokenPresent() {
		return
	}
	response, body, err := a.request(ctx, "GET", dataBase+"/Me", nil, nil)
	if err != nil || response.StatusCode != 200 {
		return
	}
	raw, err := parseJSON(body, "Me")
	if err != nil {
		return
	}
	if _, ok := raw["Me"].(map[string]any); !ok {
		return
	}
	a.loggedAt = time.Now()
}
