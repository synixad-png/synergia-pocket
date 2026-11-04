package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"
)

const (
	portal       = "https://synergia.librus.pl/loguj/portalRodzina"
	oauth        = "https://api.librus.pl/OAuth/Authorization?client_id=46"
	refresh      = "https://synergia.librus.pl/refreshToken"
	dataBase     = "https://synergia.librus.pl/gateway/api/2.0"
	msgBootstrap = "https://synergia.librus.pl/wiadomosci3"
	msgBase      = "https://wiadomosci.librus.pl/api"
	ua           = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
	maxBody      = 4 << 20
)

var (
	xmlCdata     = regexp.MustCompile(`(?is)<Content>\s*<!\[CDATA\[(.*?)\]\]>`)
	htmlTag      = regexp.MustCompile(`(?s)<[^>]*>`)
	brTag        = regexp.MustCompile(`(?i)<br\s*/?>`)
	idPattern    = regexp.MustCompile(`^[0-9]+$`)
	groupPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)
)

type ProtocolError struct{ message string }

func (e ProtocolError) Error() string { return e.message }

type APIClient struct {
	mu                sync.Mutex
	client            *http.Client
	jar               http.CookieJar
	username          string
	loggedAt          time.Time
	lastFailedRefresh time.Time
	messagesReady     bool
}

func newAPIClient(username string) (*APIClient, error) {
	if strings.TrimSpace(username) == "" {
		return nil, ProtocolError{"Podaj login."}
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	cli := &http.Client{Jar: jar, Transport: transport, Timeout: 30 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	return &APIClient{client: cli, jar: jar, username: strings.TrimSpace(username)}, nil
}

func validURL(raw string, onlyAuth bool) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "https" || u.User != nil || u.Fragment != "" || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	h := strings.ToLower(u.Hostname())
	if h == "api.librus.pl" || h == "synergia.librus.pl" {
		return true
	}
	if onlyAuth {
		// The credential login must remain restricted to exact OAuth hosts.
		return false
	}
	// The Wiadomosci bootstrap is an SSO redirect chain. Legitimate
	// first-party redirects may go through other Librus subdomains.
	// Accept only HTTPS sites actually under Librus-controlled DNS zones;
	// never allow substring lookalikes such as librus.pl.evil.example.
	return h == "librus.pl" || strings.HasSuffix(h, ".librus.pl") ||
		h == "librus.com.pl" || h == "www.librus.com.pl"
}

func followURL(base, location string, onlyAuth bool) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", ProtocolError{"Nieprawidłowy adres odpowiedzi."}
	}
	x, err := url.Parse(location)
	if err != nil {
		return "", ProtocolError{"Nieprawidłowy adres przekierowania."}
	}
	resolved := u.ResolveReference(x)
	// If the messages service redirects to an HTTP URL for its own host,
	// a browser may silently upgrade it (HSTS), whereas Go does not.
	// This is a defensive compatibility case, not a claim that the actual
	// redirect on the user's account has been observed.
	// We NEVER send a request over plaintext HTTP and NEVER send session
	// cookies or signed query strings to a different host. This narrowly
	// upgrades only the exact messages host, with a default/80 HTTP port.
	if !onlyAuth && strings.EqualFold(resolved.Hostname(), "wiadomosci.librus.pl") &&
		resolved.Scheme == "http" && resolved.User == nil && resolved.Fragment == "" &&
		(resolved.Port() == "" || resolved.Port() == "80") {
		resolved.Scheme = "https"
		resolved.Host = "wiadomosci.librus.pl"
	}
	target := resolved.String()
	if !validURL(target, onlyAuth) {
		// Show no path, query, fragment, user-info or credential values.
		// The hostname alone was insufficient for diagnosing a redirect to
		// an otherwise allowed host, so also report the rejection CATEGORY.
		host := strings.ToLower(resolved.Hostname())
		if host == "" || len(host) > 255 {
			host = "niepoprawny lub pusty"
		}
		reason := "niezatwierdzona domena"
		switch {
		case resolved.Scheme != "https":
			reason = "protokół inny niż HTTPS"
		case resolved.User != nil:
			reason = "niedozwolony login w adresie przekierowania"
		case resolved.Fragment != "":
			reason = "fragment adresu (parametr #)"
		case resolved.Port() != "" && resolved.Port() != "443":
			reason = "niedozwolony port"
		}
		if onlyAuth {
			return "", ProtocolError{"OAuth: zablokowano przekierowanie (host: " + host + "; powód: " + reason + ")."}
		}
		return "", ProtocolError{"Wiadomości: zablokowano przekierowanie (host: " + host + "; powód: " + reason + ")."}
	}
	return target, nil
}

func (a *APIClient) request(ctx context.Context, method, path string, body io.Reader, headers map[string]string) (*http.Response, []byte, error) {
	if !validURL(path, false) {
		return nil, nil, ProtocolError{"Niedozwolony host zewnętrzny."}
	}
	req, err := http.NewRequestWithContext(ctx, method, path, body)
	if err != nil {
		return nil, nil, ProtocolError{"Nieprawidłowe żądanie."}
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json,text/html,*/*")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, nil, ProtocolError{"Błąd połączenia z Librusem lub przekroczony limit czasu."}
	}
	defer resp.Body.Close()
	bytes, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, nil, ProtocolError{"Nie można odczytać odpowiedzi Librusa."}
	}
	if len(bytes) > maxBody {
		return nil, nil, ProtocolError{"Odpowiedź zbyt duża."}
	}
	return resp, bytes, nil
}

func (a *APIClient) tokenPresent() bool {
	u, _ := url.Parse("https://synergia.librus.pl/")
	for _, c := range a.jar.Cookies(u) {
		if c.Name == "oauth_token" && c.Value != "" {
			return true
		}
	}
	return false
}

func hasCaptcha(body []byte) bool {
	s := strings.ToLower(string(body))
	return strings.Contains(s, "recaptcha") || strings.Contains(s, "hcaptcha") || strings.Contains(s, "g-recaptcha") || strings.Contains(s, "captcha")
}

func (a *APIClient) login(ctx context.Context, password string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if password == "" {
		return ProtocolError{"Podaj hasło."}
	}
	a.loggedAt = time.Time{}
	a.messagesReady = false
	step, _, err := a.request(ctx, "GET", portal, nil, nil)
	if err != nil {
		return err
	}
	dest := step.Header.Get("Location")
	if dest == "" {
		return ProtocolError{"Brak przekierowania logowania Librus."}
	}
	authURL, err := followURL(portal, dest, true)
	if err != nil {
		return err
	}
	parsed, _ := url.Parse(authURL)
	if parsed.Hostname() != "api.librus.pl" {
		return ProtocolError{"Niepoprawny host autoryzacji."}
	}
	if _, _, err = a.request(ctx, "GET", authURL, nil, nil); err != nil {
		return err
	}
	form := url.Values{"action": {"login"}, "login": {a.username}, "pass": {password}}
	hdr := map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Accept": "*/*",
		"Origin": "https://api.librus.pl", "Referer": oauth, "X-Requested-With": "XMLHttpRequest",
		"Sec-Fetch-Dest": "empty", "Sec-Fetch-Mode": "cors", "Sec-Fetch-Site": "same-origin"}
	response, raw, err := a.request(ctx, "POST", oauth, strings.NewReader(form.Encode()), hdr)
	if err != nil {
		return err
	}
	if hasCaptcha(raw) {
		return ProtocolError{"Librus wymaga CAPTCHA. Skorzystaj z oficjalnego logowania."}
	}
	if response.StatusCode >= 400 {
		return ProtocolError{fmt.Sprintf("Librus odrzucił logowanie (HTTP %d).", response.StatusCode)}
	}
	var payload struct {
		GoTo string `json:"goTo"`
	}
	if json.Unmarshal(raw, &payload) != nil || payload.GoTo == "" {
		return ProtocolError{"Brak goTo w odpowiedzi. Hasło może być niepoprawne lub logowanie zmienione."}
	}
	next, err := followURL(oauth, payload.GoTo, true)
	if err != nil {
		return err
	}
	completed := false
	for i := 0; i < 10; i++ {
		r, b, e := a.request(ctx, "GET", next, nil, nil)
		if e != nil {
			return e
		}
		if hasCaptcha(b) {
			return ProtocolError{"Librus wymaga CAPTCHA."}
		}
		if r.Header.Get("Location") == "" {
			completed = true
			break
		}
		next, e = followURL(next, r.Header.Get("Location"), true)
		if e != nil {
			return e
		}
	}
	if !completed {
		return ProtocolError{"Przekroczono 10 przekierowań OAuth."}
	}
	if !a.tokenPresent() {
		return ProtocolError{"Nie otrzymano sesji oauth_token."}
	}
	a.loggedAt = time.Now()
	return nil
}

func (a *APIClient) refreshSession(ctx context.Context) bool {
	if a.loggedAt.IsZero() {
		return false
	}
	r, _, err := a.request(ctx, "GET", refresh, nil, nil)
	if err != nil {
		a.lastFailedRefresh = time.Now()
		return false
	}
	issued := false
	for _, c := range r.Cookies() {
		if c.Name == "oauth_token" && c.Value != "" {
			issued = true
		}
	}
	if r.StatusCode == 200 && issued && a.tokenPresent() {
		a.loggedAt = time.Now()
		a.lastFailedRefresh = time.Time{}
		return true
	}
	a.lastFailedRefresh = time.Now()
	return false
}

func (a *APIClient) ensure(ctx context.Context) error {
	if a.loggedAt.IsZero() || !a.tokenPresent() {
		return ProtocolError{"Zaloguj konto ponownie."}
	}
	if time.Since(a.loggedAt) > 2*time.Hour && (a.lastFailedRefresh.IsZero() || time.Since(a.lastFailedRefresh) > 30*time.Minute) {
		a.refreshSession(ctx)
	}
	if time.Since(a.loggedAt) > 20*time.Hour {
		return ProtocolError{"Sesja wygasła. Wymagane ponowne logowanie."}
	}
	return nil
}

func parseJSON(raw []byte, label string) (map[string]any, error) {
	var x map[string]any
	if err := json.Unmarshal(raw, &x); err != nil || x == nil {
		return nil, ProtocolError{label + ": nieoczekiwany format JSON."}
	}
	return x, nil
}

func (a *APIClient) get(ctx context.Context, endpoint, label string) (map[string]any, error) {
	if err := a.ensure(ctx); err != nil {
		return nil, err
	}
	r, b, err := a.request(ctx, "GET", endpoint, nil, nil)
	if err != nil {
		return nil, err
	}
	if r.StatusCode == 401 && a.refreshSession(ctx) {
		r, b, err = a.request(ctx, "GET", endpoint, nil, nil)
		if err != nil {
			return nil, err
		}
	}
	if r.StatusCode >= 400 {
		return nil, ProtocolError{fmt.Sprintf("%s: HTTP %d. Konto może nie mieć dostępu do tego modułu.", label, r.StatusCode)}
	}
	return parseJSON(b, label)
}

func (a *APIClient) data(ctx context.Context, path string, query url.Values) (map[string]any, error) {
	if path == "" || strings.Contains(path, "..") || strings.ContainsAny(path, ":\\") || strings.HasPrefix(path, "/") {
		return nil, ProtocolError{"Niepoprawny endpoint."}
	}
	target := dataBase + "/" + path
	if query != nil {
		target += "?" + query.Encode()
	}
	return a.get(ctx, target, path)
}

// bootstrap establishes the independently-authenticated Wiadomosci session.
// Unlike the OAuth handshake, its /wiadomosci3 entrypoint can return 302
// redirects to wiadomosci.librus.pl. The global HTTP client disables
// redirects, so they MUST be walked explicitly, preserving the same cookie jar.
func (a *APIClient) bootstrap(ctx context.Context) error {
	if err := a.ensure(ctx); err != nil {
		return err
	}
	a.messagesReady = false
	current := msgBootstrap
	visited := map[string]bool{}
	for hop := 0; hop < 10; hop++ {
		if visited[current] {
			return ProtocolError{"Moduł Wiadomości: wykryto pętlę przekierowań."}
		}
		visited[current] = true
		r, b, err := a.request(ctx, "GET", current, nil, nil)
		if err != nil {
			return err
		}
		if r.StatusCode >= 300 && r.StatusCode < 400 {
			location := r.Header.Get("Location")
			if location == "" {
				return ProtocolError{fmt.Sprintf("Moduł Wiadomości: HTTP %d bez adresu przekierowania.", r.StatusCode)}
			}
			next, err := followURL(current, location, false)
			if err != nil {
				return err
			}
			current = next
			continue
		}
		if r.StatusCode >= 400 {
			return ProtocolError{fmt.Sprintf("Moduł Wiadomości: HTTP %d przy ustanawianiu sesji.", r.StatusCode)}
		}
		if r.StatusCode < 200 {
			return ProtocolError{fmt.Sprintf("Moduł Wiadomości: nieoczekiwany HTTP %d przy ustanawianiu sesji.", r.StatusCode)}
		}
		if strings.Contains(strings.ToLower(string(b)), "brak dostępu") {
			return ProtocolError{"Szkoła nie udostępnia modułu wiadomości."}
		}
		a.messagesReady = true
		return nil
	}
	return ProtocolError{"Moduł Wiadomości: za dużo przekierowań sesji (limit 10)."}
}

func (a *APIClient) messageGet(ctx context.Context, endpoint string, query url.Values) (any, error) {
	if strings.Contains(endpoint, "..") || strings.Contains(endpoint, ":") || strings.HasPrefix(endpoint, "/") {
		return nil, ProtocolError{"Niepoprawna ścieżka wiadomości."}
	}
	if err := a.ensure(ctx); err != nil {
		return nil, err
	}
	if !a.messagesReady {
		if err := a.bootstrap(ctx); err != nil {
			return nil, err
		}
	}
	target := msgBase + "/" + endpoint
	if query != nil {
		target += "?" + query.Encode()
	}
	r, b, err := a.request(ctx, "GET", target, nil, nil)
	if err != nil {
		return nil, err
	}
	missingGroupPermission := strings.HasPrefix(endpoint, "receivers/groups/school-employees") || endpoint == "receivers/student-subjects"
	authRejected := r.StatusCode == 401 || (r.StatusCode == 403 && !missingGroupPermission) ||
		(r.StatusCode >= 300 && r.StatusCode < 400)
	if authRejected {
		a.messagesReady = false
		if err := a.bootstrap(ctx); err != nil {
			return nil, err
		}
		r, b, err = a.request(ctx, "GET", target, nil, nil)
		if err != nil {
			return nil, err
		}
	}
	if r.StatusCode >= 300 && r.StatusCode < 400 {
		return nil, ProtocolError{"Wiadomości: po odnowieniu sesji serwer nadal przekierowuje do logowania."}
	}
	if r.StatusCode >= 400 {
		return nil, ProtocolError{fmt.Sprintf("Wiadomości: HTTP %d po ustanowieniu sesji. Otwórz Wiadomości w Librusie i sprawdź uprawnienia konta.", r.StatusCode)}
	}
	var x any
	if json.Unmarshal(b, &x) != nil {
		return nil, ProtocolError{"Wiadomości: odpowiedź nie jest JSON."}
	}
	switch x.(type) {
	case []any, map[string]any:
		return x, nil
	}
	return nil, ProtocolError{"Wiadomości: niespodziewany format danych."}
}

func obj(v any) map[string]any { x, _ := v.(map[string]any); return x }
func arr(v any) []any          { x, _ := v.([]any); return x }
func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func ref(v any) string { return str(obj(v)["Id"]) }
func names(p map[string]any, key string, person bool) map[string]string {
	m := map[string]string{}
	for _, r := range arr(p[key]) {
		entry := obj(r)
		id := ref(entry)
		if id == "" {
			continue
		}
		if person {
			m[id] = strings.TrimSpace(str(entry["FirstName"]) + " " + str(entry["LastName"]))
		} else {
			m[id] = str(entry["Name"])
			if m[id] == "" {
				m[id] = str(entry["Symbol"])
			}
		}
	}
	return m
}

func decodePreview(raw string) string {
	decoded, err := base64.StdEncoding.DecodeString(raw)
	var text string
	if err == nil {
		text = string(decoded)
	} else {
		text = raw
	}
	if match := xmlCdata.FindStringSubmatch(text); len(match) > 1 {
		text = match[1]
	}
	text = brTag.ReplaceAllString(text, "\n")
	text = htmlTag.ReplaceAllString(text, "")
	return strings.TrimSpace(html.UnescapeString(text))
}

func (a *APIClient) getMessages(ctx context.Context) ([]map[string]any, error) {
	v, err := a.messageGet(ctx, "inbox/messages", url.Values{"limit": {"30"}})
	if err != nil {
		return nil, err
	}
	x := arr(v)
	if m, ok := v.(map[string]any); ok {
		x = arr(m["data"])
		if _, has := m["data"]; !has {
			return nil, ProtocolError{"Wiadomości: brak listy data."}
		}
	}
	out := []map[string]any{}
	for _, row := range x {
		m := obj(row)
		id := str(m["messageId"])
		if id == "" {
			continue
		}
		sender := str(m["senderName"])
		if sender == "" {
			sender = strings.TrimSpace(str(m["senderFirstName"]) + " " + str(m["senderLastName"]))
		}
		out = append(out, map[string]any{"date": m["sendDate"], "sender": sender, "subject": m["topic"], "preview": decodePreview(str(m["content"])), "unread": m["readDate"] == nil})
	}
	return out, nil
}

func (a *APIClient) getGrades(ctx context.Context) ([]map[string]any, error) {
	grades, err := a.data(ctx, "Grades", nil)
	if err != nil {
		return nil, err
	}
	subjects, err := a.data(ctx, "Subjects", nil)
	if err != nil {
		return nil, err
	}
	cat, err := a.data(ctx, "Grades/Categories", nil)
	if err != nil {
		return nil, err
	}
	s, c := names(subjects, "Subjects", false), names(cat, "Categories", false)
	out := []map[string]any{}
	for _, row := range arr(grades["Grades"]) {
		m := obj(row)
		out = append(out, map[string]any{"date": m["AddDate"], "value": m["Grade"], "subject": s[ref(m["Subject"])], "category": c[ref(m["Category"])]})
	}
	return out, nil
}

func (a *APIClient) getTimetable(ctx context.Context) ([]map[string]any, error) {
	poland, _ := time.LoadLocation("Europe/Warsaw")
	now := time.Now().In(poland)
	weekStart := now.AddDate(0, 0, -(int(now.Weekday())+6)%7).Format("2006-01-02")
	plan, err := a.data(ctx, "Timetables", url.Values{"weekStart": {weekStart}})
	if err != nil {
		return nil, err
	}
	subjects, err := a.data(ctx, "Subjects", nil)
	if err != nil {
		return nil, err
	}
	users, err := a.data(ctx, "Users", nil)
	if err != nil {
		return nil, err
	}
	rooms, err := a.data(ctx, "Classrooms", nil)
	if err != nil {
		return nil, err
	}
	sm, tm, rm := names(subjects, "Subjects", false), names(users, "Users", true), names(rooms, "Classrooms", false)
	days, ok := plan["Timetable"].(map[string]any)
	if !ok {
		return nil, ProtocolError{"Plan: nieoczekiwany format Timetable."}
	}
	result := []map[string]any{}
	for day, slots := range days {
		for _, slot := range arr(slots) {
			for _, row := range arr(slot) {
				r := obj(row)
				result = append(result, map[string]any{"date": day, "start": r["HourFrom"], "end": r["HourTo"], "number": r["LessonNo"],
					"subject": sm[ref(r["Subject"])], "teacher": tm[ref(r["Teacher"])], "room": rm[ref(r["Classroom"])],
					"canceled": r["IsCanceled"] == true, "substitution": r["IsSubstitutionClass"] == true,
					"oldSubject": sm[ref(r["OrgSubject"])], "note": r["SubstitutionNote"]})
			}
		}
	}
	return result, nil
}

func (a *APIClient) getContacts(ctx context.Context) ([]map[string]any, error) {
	v, err := a.messageGet(ctx, "receivers/types", url.Values{"includeClass": {"true"}})
	if err != nil {
		return nil, err
	}
	groups := arr(obj(obj(v)["data"])["list"])
	out := []map[string]any{}
	seen := map[string]bool{}
	for _, g := range groups {
		group := obj(g)
		code := str(group["id"])
		if !groupPattern.MatchString(code) {
			continue
		}
		payload, e := a.messageGet(ctx, "receivers/groups/school-employees", url.Values{"receiverType": {code}})
		if e != nil {
			// Missing/refused optional recipient groups are normal in some schools,
			// but transient auth/transport failures must NOT look like empty contacts.
			var perr ProtocolError
			if errors.As(e, &perr) && (strings.Contains(perr.message, "HTTP 403") || strings.Contains(perr.message, "HTTP 404")) {
				continue
			}
			return nil, e
		}
		for _, item := range arr(obj(payload)["receivers"]) {
			contact := obj(item)
			id := str(contact["accountId"])
			if !idPattern.MatchString(id) || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, map[string]any{"name": contact["label"], "role": group["name"], "id": id})
		}
	}
	return out, nil
}

// Local web server uses a one-time random token to issue an HttpOnly Strict cookie.
type LocalApp struct {
	token       string
	origin      string
	mu          sync.RWMutex
	profiles    [2]*APIClient
	stopUpdates func()
}

func token() string {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(raw[:])
}
func (a *LocalApp) validRequest(w http.ResponseWriter, r *http.Request) bool {
	c, err := r.Cookie("synergia_local")
	if err != nil || !hmac.Equal([]byte(c.Value), []byte(a.token)) {
		http.Error(w, "Dostęp tylko po uruchomieniu programu.", 403)
		return false
	}
	if r.Method == "POST" && r.Header.Get("Origin") != a.origin {
		http.Error(w, "Nieprawidłowe pochodzenie żądania.", 403)
		return false
	}
	return true
}
func serveJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
func serverError(w http.ResponseWriter, err error) {
	serveJSON(w, 502, map[string]any{"error": err.Error()})
}

func (a *LocalApp) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("k") != "" {
			if !hmac.Equal([]byte(r.URL.Query().Get("k")), []byte(a.token)) {
				http.Error(w, "Brak dostępu", 403)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "synergia_local", Value: a.token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if !a.validRequest(w, r) {
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; img-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
		io.WriteString(w, runtimeHTMLV13())
	})
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		if !a.validRequest(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "Method", 405)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		var payload struct {
			Profile  int    `json:"profile"`
			Login    string `json:"login"`
			Password string `json:"password"`
		}
		if json.NewDecoder(r.Body).Decode(&payload) != nil || payload.Profile < 0 || payload.Profile > 1 || len(payload.Login) > 128 || len(payload.Password) > 300 {
			serveJSON(w, 400, map[string]any{"error": "Nieprawidłowe dane logowania."})
			return
		}
		c, err := newAPIClient(payload.Login)
		if err != nil {
			serverError(w, err)
			return
		}
		// No password is persisted. The official Librus account is accessed only after user action.
		if err := c.login(r.Context(), payload.Password); err != nil {
			serverError(w, err)
			return
		}
		a.mu.Lock()
		a.profiles[payload.Profile] = c
		a.mu.Unlock()
		serveJSON(w, 200, map[string]any{"ok": true, "profile": payload.Profile})
	})
	mux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		if !a.validRequest(w, r) {
			return
		}
		if r.Method != "GET" {
			http.Error(w, "Method", 405)
			return
		}
		idx, err := strconv.Atoi(r.URL.Query().Get("profile"))
		if err != nil || idx < 0 || idx > 1 {
			serveJSON(w, 400, map[string]string{"error": "Nieprawidłowy profil."})
			return
		}
		a.mu.RLock()
		c := a.profiles[idx]
		a.mu.RUnlock()
		if c == nil {
			serveJSON(w, 401, map[string]string{"error": "Najpierw zaloguj profil."})
			return
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		var result any
		var warnings []string
		var gradeSources []gradeSourceV16
		switch r.URL.Query().Get("section") {
		case "messages":
			result, err = c.getMessages(r.Context())
		case "grades":
			var grades gradesResultV16
			grades, err = c.getAllGradesV16(r.Context())
			result, warnings, gradeSources = grades.items, grades.warnings, grades.sources
		case "timetable":
			result, err = c.getTimetable(r.Context())
		case "contacts":
			result, err = c.getContacts(r.Context())
		case "me":
			result, err = c.data(r.Context(), "Me", nil)
		// v0.6: independent read-only modules; they never call one another.
		case "attendance":
			var module moduleRows
			module, err = c.getAttendanceV06(r.Context())
			result, warnings = module.items, module.warnings
		case "agenda":
			var module moduleRows
			module, err = c.getAgendaV06(r.Context())
			result, warnings = module.items, module.warnings
		case "notes":
			var module moduleRows
			module, err = c.getNotesV06(r.Context())
			result, warnings = module.items, module.warnings
		// v0.7: isolated HomeWorkAssignments; never invokes the agenda module.
		case "homework":
			var module moduleRows
			module, err = c.getHomeworkV07(r.Context())
			result, warnings = module.items, module.warnings
		// v0.11: two new read-only modules; existing cases unchanged.
		case "notices":
			var module moduleRows
			module, err = c.getNoticesV11(r.Context())
			result, warnings = module.items, module.warnings
		case "freedays":
			var module moduleRows
			module, err = c.getFreeDaysV11(r.Context())
			result, warnings = module.items, module.warnings
		default:
			serveJSON(w, 400, map[string]string{"error": "Nieznana sekcja."})
			return
		}
		if err != nil {
			serverError(w, err)
			return
		}
		serveJSON(w, 200, map[string]any{"items": result, "warnings": warnings, "gradeSources": gradeSources})
	})
	// v0.8: isolated homework attachment endpoints; no other route is changed.
	mux.HandleFunc("/api/homework/files", func(w http.ResponseWriter, r *http.Request) {
		a.homeworkFilesHandler(w, r, false)
	})
	mux.HandleFunc("/api/homework/download", func(w http.ResponseWriter, r *http.Request) {
		a.homeworkFilesHandler(w, r, true)
	})
	// v0.9: diagnostyka jest samodzielnym modułem; dotychczasowe trasy niezmienione.
	installDiagnosticsRoutes(mux, a)
	// v0.10: separate SchoolFiles module; previous route logic untouched.
	installSchoolRoutes(mux, a)
	// v0.12: independent message routes; old list and feature modules unchanged.
	installMessagesRoutesV12(mux, a)
	observer := installUpdatesRoutesV13(mux, a)
	a.stopUpdates = observer.stopAll
	mux.HandleFunc("/api/exit", func(w http.ResponseWriter, r *http.Request) {
		if !a.validRequest(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "Method", 405)
			return
		}
		serveJSON(w, 200, map[string]bool{"ok": true})
		go func() { time.Sleep(200 * time.Millisecond); os.Exit(0) }()
	})
	return mux
}

func desktopMain() {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal("Nie można otworzyć portu lokalnego: ", err)
	}
	a := &LocalApp{token: token(), origin: "http://" + listener.Addr().String()}
	srv := &http.Server{Handler: a.routes(), ReadHeaderTimeout: 8 * time.Second}
	go func() {
		if e := srv.Serve(listener); e != nil && !errors.Is(e, http.ErrServerClosed) {
			log.Println("Local server:", e)
		}
	}()
	address := a.origin + "/?k=" + url.QueryEscape(a.token)
	if err := exec.Command("rundll32", "url.dll,FileProtocolHandler", address).Start(); err != nil {
		log.Printf("Otwórz lokalnie w przeglądarce: %s", address)
	}
	// This executable runs until the user closes it via the local dashboard.
	<-make(chan struct{})
}
