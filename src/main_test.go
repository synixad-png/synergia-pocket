package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type record struct {
	status   int
	body     string
	location string
	cookie   string
}
type mockedHTTP struct {
	mu       sync.Mutex
	scripted map[string][]record
	calls    []string
}

func newMock() *mockedHTTP { return &mockedHTTP{scripted: map[string][]record{}} }
func (m *mockedHTTP) add(method, path string, r record) {
	m.scripted[method+" "+path] = append(m.scripted[method+" "+path], r)
}
func (m *mockedHTTP) RoundTrip(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := req.Method + " " + req.URL.String()
	m.calls = append(m.calls, key)
	replies := m.scripted[key]
	if len(replies) == 0 {
		return nil, ProtocolError{"Unexpected mock request: " + key}
	}
	r := replies[0]
	m.scripted[key] = replies[1:]
	h := make(http.Header)
	if r.location != "" {
		h.Set("Location", r.location)
	}
	if r.cookie != "" {
		h.Set("Set-Cookie", r.cookie)
	}
	return &http.Response{StatusCode: r.status, Header: h, Body: io.NopCloser(strings.NewReader(r.body)), Request: req}, nil
}
func fakeAccount() (*APIClient, *mockedHTTP) {
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse("https://synergia.librus.pl/")
	jar.SetCookies(u, []*http.Cookie{{Name: "oauth_token", Value: "fake_token", Domain: "synergia.librus.pl", Path: "/"}})
	mock := newMock()
	c := &http.Client{Jar: jar, Transport: mock, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &APIClient{client: c, jar: jar, username: "user", loggedAt: time.Now()}, mock
}
func TestRedirectHostAllowlist(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://api.librus.pl/OAuth/Authorization", true},
		{"https://synergia.librus.pl/gateway/api/2.0/Me", true},
		{"http://api.librus.pl/OAuth/Token", false},
		{"https://synergia.librus.pl.evil.com/Me", false},
		{"https://127.0.0.1:443/private", false},
		{"https://synergia.librus.pl:444/private", false},
	}
	for _, c := range cases {
		if got := validURL(c.url, true); got != c.want {
			t.Fatalf("%s allowed=%v want %v", c.url, got, c.want)
		}
	}
}
func TestLoginOAuthWithSyntheticCookies(t *testing.T) {
	a, m := fakeAccount()
	a.loggedAt = time.Time{}
	authURL := "https://api.librus.pl/OAuth/Authorization?client_id=46&response_type=code"
	m.add("GET", portal, record{302, "", authURL, ""})
	m.add("GET", authURL, record{200, "", "", ""})
	m.add("POST", oauth, record{200, `{"status":"ok","goTo":"/OAuth/Authorization/2FA?client_id=46"}`, "", ""})
	m.add("GET", "https://api.librus.pl/OAuth/Authorization/2FA?client_id=46", record{302, "", "https://synergia.librus.pl/landing", ""})
	m.add("GET", "https://synergia.librus.pl/landing", record{200, "", "", "oauth_token=new_cookie; Path=/; Domain=synergia.librus.pl"})
	if err := a.login(context.Background(), "synthetic-only"); err != nil {
		t.Fatal(err)
	}
	if !a.tokenPresent() || a.loggedAt.IsZero() {
		t.Fatal("login did not finish")
	}
	if len(m.calls) != 5 {
		t.Fatalf("expected 5 calls, got: %v", m.calls)
	}
	if strings.Contains(strings.Join(m.calls, "\n"), "synthetic-only") {
		t.Fatal("password leaked into URL")
	}
}
func TestBlockedForeignLoginRedirect(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", portal, record{302, "", "https://attacker.invalid/credentials", ""})
	if e := a.login(context.Background(), "fake"); e == nil {
		t.Fatal("foreign redirect should be blocked")
	}
	if len(m.calls) != 1 {
		t.Fatal("should not follow foreign redirect")
	}
}
func TestRefreshRequiresNewTokenCookie(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", refresh, record{200, "", "", ""})
	if a.refreshSession(context.Background()) {
		t.Fatal("200 without new Set-Cookie must fail")
	}
	m.add("GET", refresh, record{200, "", "", "oauth_token=renewed; Path=/; Domain=synergia.librus.pl"})
	if !a.refreshSession(context.Background()) {
		t.Fatal("expected refresh successful")
	}
}
func TestDecodedMessagePreview(t *testing.T) {
	raw := "<Message><Content><![CDATA[Ważne <b>zadanie</b>]]></Content></Message>"
	b := base64.StdEncoding.EncodeToString([]byte(raw))
	if got := decodePreview(b); got != "Ważne zadanie" {
		t.Fatalf("got %q", got)
	}
}
func TestMessagesAreReadOnly(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", msgBootstrap, record{200, "Wiadomosci", "", ""})
	preview := base64.StdEncoding.EncodeToString([]byte("<Message><Content><![CDATA[Dzień dobry]]></Content></Message>"))
	body, _ := json.Marshal(map[string]any{"data": []any{map[string]any{
		"messageId": 42, "senderFirstName": "Nauczyciel", "senderLastName": "Przykladowy",
		"topic": "Sprawdzian", "content": preview, "readDate": nil}}})
	m.add("GET", msgBase+"/inbox/messages?limit=30", record{200, string(body), "", ""})
	rows, err := a.getMessages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["preview"] != "Dzień dobry" {
		t.Fatalf("unexpected messages %v", rows)
	}
	for _, call := range m.calls {
		if strings.Contains(call, "/messages/42") {
			t.Fatal("full message should never be fetched automatically")
		}
	}
}
func TestGradesAndReferences(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", dataBase+"/Grades", record{200, `{"Grades":[{"Grade":"5+","AddDate":"2026-11-04","Subject":{"Id":1},"Category":{"Id":2}}]}`, "", ""})
	m.add("GET", dataBase+"/Subjects", record{200, `{"Subjects":[{"Id":1,"Name":"Matematyka"}]}`, "", ""})
	m.add("GET", dataBase+"/Grades/Categories", record{200, `{"Categories":[{"Id":2,"Name":"Kartkówka"}]}`, "", ""})
	g, err := a.getGrades(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(g) != 1 || g[0]["value"] != "5+" || g[0]["subject"] != "Matematyka" || g[0]["category"] != "Kartkówka" {
		t.Fatal(g)
	}
}
func TestTimetableParallelAndSubstitution(t *testing.T) {
	a, m := fakeAccount()
	// The weekStart value is created by the runtime; route matching uses it.
	poland, _ := time.LoadLocation("Europe/Warsaw")
	now := time.Now().In(poland)
	monday := now.AddDate(0, 0, -(int(now.Weekday())+6)%7).Format("2006-01-02")
	m.add("GET", dataBase+"/Timetables?weekStart="+monday, record{200, `{"Timetable":{"2026-11-04":[[{"Subject":{"Id":3},"OrgSubject":{"Id":4},"IsSubstitutionClass":true},{"Subject":{"Id":4}}]]}}`, "", ""})
	m.add("GET", dataBase+"/Subjects", record{200, `{"Subjects":[{"Id":3,"Name":"Historia"},{"Id":4,"Name":"Biologia"}]}`, "", ""})
	m.add("GET", dataBase+"/Users", record{200, `{"Users":[]}`, "", ""})
	m.add("GET", dataBase+"/Classrooms", record{200, `{"Classrooms":[]}`, "", ""})
	p, err := a.getTimetable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 2 || p[0]["oldSubject"] != "Biologia" || p[0]["substitution"] != true {
		t.Fatal(p)
	}
}
func TestLocalAuthenticationAndCSRF(t *testing.T) {
	app := &LocalApp{token: "known_local_secret", origin: "http://127.0.0.1:12345"}
	routes := app.routes()
	blank := httptest.NewRecorder()
	routes.ServeHTTP(blank, httptest.NewRequest("GET", "http://127.0.0.1:12345/", nil))
	if blank.Code != 403 {
		t.Fatalf("non authorized UI should 403, got %d", blank.Code)
	}
	init := httptest.NewRecorder()
	routes.ServeHTTP(init, httptest.NewRequest("GET", "http://127.0.0.1:12345/?k=known_local_secret", nil))
	if init.Code != http.StatusSeeOther {
		t.Fatal("bootstrap", init.Code)
	}
	cookie := init.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("cookie not protected")
	}
	req := httptest.NewRequest("GET", "http://127.0.0.1:12345/", nil)
	req.AddCookie(cookie)
	ui := httptest.NewRecorder()
	routes.ServeHTTP(ui, req)
	if ui.Code != 200 || !strings.Contains(ui.Body.String(), "SYNERGIA API") {
		t.Fatal("UI not available")
	}
	if !strings.Contains(ui.Header().Get("Content-Security-Policy"), "connect-src 'self'") {
		t.Fatal("CSP missing")
	}
	badPost := httptest.NewRequest("POST", "http://127.0.0.1:12345/api/login", strings.NewReader("{}"))
	badPost.AddCookie(cookie)
	badPost.Header.Set("Origin", "https://attacker.invalid")
	denied := httptest.NewRecorder()
	routes.ServeHTTP(denied, badPost)
	if denied.Code != 403 {
		t.Fatal("CSRF not blocked")
	}
	noLogin := httptest.NewRequest("GET", "http://127.0.0.1:12345/api/data?profile=0&section=grades", nil)
	noLogin.AddCookie(cookie)
	noLoginResponse := httptest.NewRecorder()
	routes.ServeHTTP(noLoginResponse, noLogin)
	if noLoginResponse.Code != 401 {
		t.Fatal("unlogged profile should return 401")
	}
}
func TestTwoProfilesIndependentCookies(t *testing.T) {
	a, e := newAPIClient("A")
	if e != nil {
		t.Fatal(e)
	}
	b, e := newAPIClient("B")
	if e != nil {
		t.Fatal(e)
	}
	if a.jar == b.jar {
		t.Fatal("two profiles share cookiejar")
	}
}

// In production the public /wiadomosci3 bootstrap is a separate domain flow.
// A no-redirect HTTP client skips its cookie-setting hops unless handled explicitly.
func TestMessagesBootstrapFollowsAllowedRedirectsAndStoresCookies(t *testing.T) {
	a, mock := fakeAccount()
	jump := "https://wiadomosci.librus.pl/auth/callback?request=mock"
	mock.add("GET", msgBootstrap, record{status: 302, location: jump})
	mock.add("GET", jump, record{status: 200, body: "<html>Wiadomości</html>", cookie: "MOCK_MSG_SESSION=token; Path=/; Domain=wiadomosci.librus.pl; Secure"})
	payload := `{"data":[{"messageId":100,"senderName":"Przykładowa szkoła","topic":"Termin","content":""}]}`
	mock.add("GET", msgBase+"/inbox/messages?limit=30", record{status: 200, body: payload})
	messages, err := a.getMessages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages missing: %v", messages)
	}
	domain, _ := url.Parse("https://wiadomosci.librus.pl/")
	msgCookiePresent := false
	for _, c := range a.jar.Cookies(domain) {
		if c.Name == "MOCK_MSG_SESSION" && c.Value == "token" {
			msgCookiePresent = true
		}
	}
	if !msgCookiePresent {
		t.Fatal("Separate message domain cookies were lost")
	}
	if len(mock.calls) != 3 {
		t.Fatalf("unexpected messages sequence: %v", mock.calls)
	}
}

func TestMessagesUnauthorizedRebootstrapAndRetrySameCall(t *testing.T) {
	a, m := fakeAccount()
	a.messagesReady = true
	inbox := msgBase + "/inbox/messages?limit=30"
	m.add("GET", inbox, record{status: 401, body: "session rejected"})
	jump := "https://wiadomosci.librus.pl/auth/callback"
	m.add("GET", msgBootstrap, record{status: 302, location: jump})
	m.add("GET", jump, record{status: 200, body: "Welcome"})
	m.add("GET", inbox, record{status: 200, body: `{"data":[]}`})
	records, err := a.getMessages(context.Background())
	if err != nil || records == nil {
		t.Fatalf("failed rebootstrap: %v %v", records, err)
	}
	if !a.messagesReady || len(m.calls) != 4 {
		t.Fatalf("retry not performed: %v", m.calls)
	}
}

func TestMessagesRedirectToLoginRebootstrapsOnce(t *testing.T) {
	a, m := fakeAccount()
	a.messagesReady = true
	endpoint := msgBase + "/inbox/messages?limit=30"
	m.add("GET", endpoint, record{status: 302, location: "https://synergia.librus.pl/loguj/portalRodzina"})
	m.add("GET", msgBootstrap, record{status: 200, body: "Success"})
	m.add("GET", endpoint, record{status: 200, body: `{"data":[]}`})
	_, err := a.getMessages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

func TestMessagesBootstrapBlocksForeignRedirect(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", msgBootstrap, record{status: 302, location: "https://attacker.example/steal"})
	_, err := a.getMessages(context.Background())
	if err == nil {
		t.Fatal("foreign redirect accepted")
	}
	if len(m.calls) != 1 {
		t.Fatalf("redirect was followed: %v", m.calls)
	}
	if a.messagesReady {
		t.Fatal("failed bootstrap incorrectly marked ready")
	}
}

func TestMessagesBootstrapMissingLocationIsError(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", msgBootstrap, record{status: 302})
	if e := a.bootstrap(context.Background()); e == nil {
		t.Fatal("missing redirect Location should not be a success")
	}
	if a.messagesReady {
		t.Fatal("incorrectly marked ready")
	}
}

func TestContactsAfterMessageBootstrap(t *testing.T) {
	a, m := fakeAccount()
	jump := "https://wiadomosci.librus.pl/auth/callback"
	m.add("GET", msgBootstrap, record{status: 302, location: jump})
	m.add("GET", jump, record{status: 200, body: "<html>Wiadomości</html>"})
	m.add("GET", msgBase+"/receivers/types?includeClass=true", record{status: 200, body: `{"data":{"defaultGroup":"tutors","list":[{"id":"tutors","name":"Wychowawcy"}]}}`})
	m.add("GET", msgBase+"/receivers/groups/school-employees?receiverType=tutors", record{status: 200, body: `{"receivers":[{"accountId":"333444","label":"Przykładowy nauczyciel"}]}`})
	rows, err := a.getContacts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["role"] != "Wychowawcy" {
		t.Fatalf("contacts not shown: %v", rows)
	}
}

func TestContactsOptionalGroup403DoesNotRelogin(t *testing.T) {
	a, m := fakeAccount()
	a.messagesReady = true
	m.add("GET", msgBase+"/receivers/types?includeClass=true", record{status: 200, body: `{"data":{"list":[{"id":"tutors","name":"Wychowawcy"},{"id":"teachers","name":"Nauczyciele"}]}}`})
	m.add("GET", msgBase+"/receivers/groups/school-employees?receiverType=tutors", record{status: 403, body: "refused group"})
	m.add("GET", msgBase+"/receivers/groups/school-employees?receiverType=teachers", record{status: 200, body: `{"receivers":[{"accountId":"777","label":"Nauczyciel"}]}`})
	rows, err := a.getContacts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("wrong contacts after ignored optional group: %v", rows)
	}
	if len(m.calls) != 3 {
		t.Fatalf("extra calls / rebootstrap on refused group: %v", m.calls)
	}
}

// Reproduction of the user's v0.3 failure: Wiadomosci may first redirect
// via other first-party Librus subdomains, not just the three original hosts.
func TestBootstrapThroughFirstPartyPortalAndMessagesCookie(t *testing.T) {
	a, m := fakeAccount()
	jump1 := "https://portal.librus.pl/rodzina/synergia/wiadomosci?state=synthetic-do-not-leak"
	jump2 := "https://wiadomosci.librus.pl/auth/callback"
	m.add("GET", msgBootstrap, record{status: 302, location: jump1})
	m.add("GET", jump1, record{status: 302, location: jump2})
	m.add("GET", jump2, record{status: 200, body: "OK", cookie: "MOCK_MSG_SESSION=ready; Path=/; Domain=wiadomosci.librus.pl; Secure"})
	m.add("GET", msgBase+"/inbox/messages?limit=30", record{status: 200, body: `{"data":[{"messageId":33,"topic":"Test"}]}`})
	rows, err := a.getMessages(context.Background())
	if err != nil || len(rows) != 1 {
		t.Fatalf("First-party SSO chain failed: %v; rows=%v", err, rows)
	}
	if len(m.calls) != 4 {
		t.Fatalf("Unexpected SSO chain length: %v", m.calls)
	}
}
func TestBootstrapFromVerifiedOfficialLibrusComPL(t *testing.T) {
	a, m := fakeAccount()
	jump := "https://www.librus.com.pl/"
	m.add("GET", msgBootstrap, record{status: 302, location: jump})
	m.add("GET", jump, record{status: 302, location: "https://wiadomosci.librus.pl/sso/finish"})
	m.add("GET", "https://wiadomosci.librus.pl/sso/finish", record{status: 200, body: "OK"})
	if err := a.bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func TestRejectedSSOHostIsShownWithoutSensitiveQuery(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", msgBootstrap, record{status: 302, location: "https://attacker.example/steal?token=SUPER_SECRET"})
	err := a.bootstrap(context.Background())
	if err == nil || !strings.Contains(err.Error(), "attacker.example") {
		t.Fatalf("No useful diagnostic: %v", err)
	}
	if strings.Contains(err.Error(), "SUPER_SECRET") {
		t.Fatal("OAuth query leaked in diagnostic")
	}
	if len(m.calls) != 1 {
		t.Fatal("Followed third-party redirect")
	}
}
func TestSSORedirectURLSecurity(t *testing.T) {
	tests := []struct {
		u       string
		allowed bool
	}{
		{"https://login.librus.pl/oauth/callback", true},
		{"https://www.librus.pl/", true},
		{"https://librus.pl/", true},
		{"https://librus.com.pl/", true},
		{"https://www.librus.com.pl/", true},
		{"https://wiadomosci.librus.pl/", true},
		{"https://portal.librus.pl/", true},
		{"https://portal.librus.pl.evil.example/", false},
		{"https://evil-librus.pl/", false},
		{"https://librus.com.pl.evil.example/", false},
		{"http://portal.librus.pl/", false},
		{"https://portal.librus.pl:4444/", false},
		{"https://user:password@portal.librus.pl/", false},
		{"https://127.0.0.1/", false},
		{"https://portal.librus.pl/#fragment", false},
	}
	for _, tc := range tests {
		if got := validURL(tc.u, false); got != tc.allowed {
			t.Errorf("%q -> %v, want %v", tc.u, got, tc.allowed)
		}
	}
	if validURL("https://portal.librus.pl/", true) {
		t.Fatal("Login OAuth host must remain restricted")
	}
}
func TestFrontendClearsStaleViewWhileLoading(t *testing.T) {
	if !strings.Contains(htmlPage, "$('view-body').textContent='Łączenie z Librusem…'") {
		t.Fatal("Frontend leaves unrelated previous tab visible while loading")
	}
}

// v0.4 regression: a redirect whose hostname was *already* allowlisted
// still failed. It can happen when the redirected URL uses HTTP instead of
// HTTPS; only a same-host upgrade may be attempted, never an HTTP request.
func TestV05MessagesHTTPRedirectUpgradedWithoutPlaintextOrCredentialLeak(t *testing.T) {
	a, m := fakeAccount()
	legacy := "http://wiadomosci.librus.pl/auth/entry?ticket=SYNTHETIC_PRIVATE_CODE"
	secure := "https://wiadomosci.librus.pl/auth/entry?ticket=SYNTHETIC_PRIVATE_CODE"
	m.add("GET", msgBootstrap, record{status: 302, location: legacy})
	m.add("GET", secure, record{status: 200, body: "OK", cookie: "MOCK_MSG_SESSION=secure; Path=/; Domain=wiadomosci.librus.pl; Secure"})
	m.add("GET", msgBase+"/inbox/messages?limit=30", record{status: 200, body: `{"data":[{"messageId":9,"topic":"Przykład"}]}`})
	rows, err := a.getMessages(context.Background())
	if err != nil || len(rows) != 1 {
		t.Fatalf("expected independent inbox to be available: %v, %+v", err, rows)
	}
	for _, call := range m.calls {
		if strings.Contains(call, "http://") {
			t.Fatalf("a plaintext HTTP request was attempted (method and host only): %s", strings.Split(call, "?")[0])
		}
	}
	uri, _ := url.Parse("https://wiadomosci.librus.pl/")
	if !strings.Contains(fmt.Sprint(a.jar.Cookies(uri)), "MOCK_MSG_SESSION") {
		t.Fatal("messages cookie missing after secure callback")
	}
}

func TestV05ContactsHTTPPort80UpgradedToHTTPSPort443(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", msgBootstrap, record{status: 302, location: "http://wiadomosci.librus.pl:80/login"})
	m.add("GET", "https://wiadomosci.librus.pl/login", record{status: 200, body: "OK"})
	m.add("GET", msgBase+"/receivers/types?includeClass=true", record{status: 200, body: `{"data":{"list":[{"id":"teachers","name":"Nauczyciele"}]}}`})
	m.add("GET", msgBase+"/receivers/groups/school-employees?receiverType=teachers", record{status: 200, body: `{"receivers":[{"accountId":"123","label":"Przykładowa nauczycielka"}]}`})
	rows, err := a.getContacts(context.Background())
	if err != nil || len(rows) != 1 || rows[0]["name"] != "Przykładowa nauczycielka" {
		t.Fatalf("contacts should reuse secure messages session: %+v, %v", rows, err)
	}
	for _, call := range m.calls {
		if strings.Contains(call, "http://") {
			t.Fatal("HTTP used for messages SSO")
		}
	}
}

func TestV05HTTPUpgradeRestrictedToExactMessagesHostname(t *testing.T) {
	cases := []struct{ loc string }{
		{"http://portal.librus.pl/"},
		{"http://evil.example/steal"},
		{"http://wiadomosci.librus.pl.evil.example/path"},
		{"http://wiadomosci.librus.pl:8080/login"},
		{"http://user:secret@wiadomosci.librus.pl/callback"},
	}
	for _, tc := range cases {
		if link, err := followURL(msgBootstrap, tc.loc, false); err == nil {
			t.Errorf("must reject %s, but returned %s", tc.loc, link)
		}
	}
}

func TestV05HTTPSOriginalsAndMainOAuthUnchanged(t *testing.T) {
	urls := []string{
		"https://wiadomosci.librus.pl/auth?code=opaque",
		"https://portal.librus.pl/my-account",
		"https://www.librus.com.pl/",
	}
	for _, u := range urls {
		result, err := followURL(msgBootstrap, u, false)
		if err != nil || result != u {
			t.Errorf("expected ordinary HTTPS flow unchanged for %s: %s %v", u, result, err)
		}
	}
	_, err := followURL(oauth, "http://wiadomosci.librus.pl/oauth", true)
	if err == nil {
		t.Fatal("OAuth credential flow must not allow HTTP/domain expansion")
	}
}

func TestV05BlockedAllowedHostShowsReasonButNoTokens(t *testing.T) {
	cases := []struct {
		loc, reason string
	}{
		{"https://wiadomosci.librus.pl:8181/callback?secret=PASSWORD", "niedozwolony port"},
		{"https://wiadomosci.librus.pl/callback?secret=PASSWORD#FRAGMENT_SECRET", "fragment adresu"},
		{"http://wiadomosci.librus.pl:8080/callback?secret=PASSWORD", "protokół inny niż HTTPS"},
	}
	for _, tc := range cases {
		_, err := followURL(msgBootstrap, tc.loc, false)
		if err == nil || !strings.Contains(err.Error(), tc.reason) {
			t.Errorf("missing specific reason for protected message host: %v", err)
		}
		if err != nil && (strings.Contains(err.Error(), "PASSWORD") || strings.Contains(err.Error(), "FRAGMENT_SECRET") || strings.Contains(err.Error(), "/callback")) {
			t.Fatal("query/path/fragment leaked to user-visible diagnostic")
		}
	}
}

func TestV05NoHTTPUpgradeOnMainOauthEvenOnAuthHost(t *testing.T) {
	if _, err := followURL(oauth, "http://api.librus.pl/OAuth/Authorization", true); err == nil {
		t.Fatal("login must remain HTTPS-only")
	}
}

func TestV05RelativeSecureRedirectUnchanged(t *testing.T) {
	result, err := followURL("https://synergia.librus.pl/wiadomosci3", "//wiadomosci.librus.pl/callback?test=yes", false)
	if err != nil || result != "https://wiadomosci.librus.pl/callback?test=yes" {
		t.Fatalf("relative URL should inherit HTTPS without broad rewriting: %q %v", result, err)
	}
}
