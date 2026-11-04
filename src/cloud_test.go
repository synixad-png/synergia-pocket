package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func cloudRequest(t *testing.T, app *cloudApp, method, path string, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "https://pocket-test.example"+path, strings.NewReader(""))
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	return w
}
func cloudTest(t *testing.T) *cloudApp {
	t.Helper()
	app := newCloudApp("https://pocket-test.example")
	t.Cleanup(app.close)
	return app
}
func cloudNewBrowser(t *testing.T, app *cloudApp) *http.Cookie {
	t.Helper()
	w := cloudRequest(t, app, "GET", "/", nil, "")
	if w.Code != 200 || len(w.Result().Cookies()) != 1 {
		t.Fatal(w.Code, w.Body)
	}
	return w.Result().Cookies()[0]
}
func TestCloudSeparateVisitorsDoNotShareAccounts(t *testing.T) {
	app := cloudTest(t)
	one := cloudNewBrowser(t, app)
	two := cloudNewBrowser(t, app)
	if one.Value == two.Value {
		t.Fatal("shared session")
	}
	c, _ := fakeAccount()
	app.sessions[one.Value].app.profiles[0] = c
	for _, test := range []struct {
		cookie *http.Cookie
		want   string
	}{{one, `"logged":[true,false]`}, {two, `"logged":[false,false]`}} {
		w := cloudRequest(t, app, "GET", "/api/updates/profiles", test.cookie, "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), test.want) {
			t.Fatal(w.Code, w.Body)
		}
	}
}
func TestCloudCookieAndHeaders(t *testing.T) {
	app := cloudTest(t)
	cookie := cloudNewBrowser(t, app)
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" || cookie.Domain != "" {
		t.Fatal(cookie)
	}
	w := cloudRequest(t, app, "GET", "/", cookie, "")
	if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") || w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal(w.Header())
	}
}
func TestCloudNoSessionsCreatedByAPIRequests(t *testing.T) {
	app := cloudTest(t)
	w := cloudRequest(t, app, "GET", "/api/updates/profiles", nil, "")
	if w.Code != 401 || len(app.sessions) != 0 {
		t.Fatal(w.Code, len(app.sessions))
	}
}
func TestCloudCannotBootstrapWithLocalStartupKey(t *testing.T) {
	app := cloudTest(t)
	w := cloudRequest(t, app, "GET", "/?k=forged", nil, "")
	if w.Code != 400 || len(app.sessions) != 0 {
		t.Fatal(w.Code)
	}
}
func TestCloudCrossOriginAndHostRefused(t *testing.T) {
	app := cloudTest(t)
	cookie := cloudNewBrowser(t, app)
	for _, origin := range []string{"", "https://evil.example", "https://pocket-test.example.evil.example"} {
		w := cloudRequest(t, app, "POST", "/api/exit", cookie, origin)
		if w.Code != 403 {
			t.Fatal(origin, w.Code)
		}
	}
	r := httptest.NewRequest("GET", "https://evil.example/", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	r = httptest.NewRequest("GET", "https://pocket-test.example/api/updates/profiles", nil)
	r.AddCookie(cookie)
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w = httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
func TestCloudLogoutDoesNotStopServerOrOtherVisitor(t *testing.T) {
	app := cloudTest(t)
	one := cloudNewBrowser(t, app)
	two := cloudNewBrowser(t, app)
	w := cloudRequest(t, app, "POST", "/api/exit", one, app.origin)
	if w.Code != 200 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal(w.Code, w.Body)
	}
	if _, exists := app.sessions[one.Value]; exists {
		t.Fatal("session retained after logout")
	}
	if w = cloudRequest(t, app, "GET", "/api/updates/profiles", one, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w = cloudRequest(t, app, "GET", "/api/updates/profiles", two, ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = cloudRequest(t, app, "GET", "/api/health", nil, ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
}
func TestCloudIdleCleanupStopsObserverAndDropsAccounts(t *testing.T) {
	app := cloudTest(t)
	cookie := cloudNewBrowser(t, app)
	session := app.sessions[cookie.Value]
	client, _ := fakeAccount()
	session.app.profiles[0] = client
	ctx, cancel := context.WithCancel(context.Background())
	session.app.stopUpdates = cancel
	session.seen = time.Now().Add(-cloudSessionIdle - time.Minute)
	app.prune()
	if len(app.sessions) != 0 || session.app.profiles[0] != nil || ctx.Err() == nil {
		t.Fatal("cleanup did not invalidate session")
	}
}
func TestCloudInFlightRequestIsNotPruned(t *testing.T) {
	app := cloudTest(t)
	cookie := cloudNewBrowser(t, app)
	s := app.sessions[cookie.Value]
	s.seen = time.Now().Add(-2 * cloudSessionIdle)
	s.inFlight = 1
	app.prune()
	if len(app.sessions) != 1 {
		t.Fatal("pruned active request")
	}
}
func TestCloudExpiredCookieCannotRestoreAnotherSession(t *testing.T) {
	app := cloudTest(t)
	cookie := cloudNewBrowser(t, app)
	app.sessions[cookie.Value].seen = time.Now().Add(-2 * cloudSessionIdle)
	w := cloudRequest(t, app, "GET", "/api/updates/profiles", cookie, "")
	if w.Code != 401 || len(app.sessions) != 0 {
		t.Fatal(w.Code)
	}
}
func TestCloudSessionCapacityBounded(t *testing.T) {
	app := cloudTest(t)
	for i := 0; i < 200; i++ {
		cloudNewBrowser(t, app)
	}
	w := cloudRequest(t, app, "GET", "/", nil, "")
	if w.Code != 503 || len(app.sessions) != 200 {
		t.Fatal(w.Code, len(app.sessions))
	}
}
func TestCloudPWAPublicAssetsAndNoOfflineAccountData(t *testing.T) {
	app := cloudTest(t)
	for _, path := range []string{"/manifest.webmanifest", "/sw.js", "/icon-192.png", "/icon-512.png", "/apple-touch-icon.png", "/offline.html"} {
		w := cloudRequest(t, app, "GET", path, nil, "")
		if w.Code != 200 || len(w.Body.Bytes()) == 0 {
			t.Fatal(path, w.Code)
		}
		if len(app.sessions) != 0 {
			t.Fatal("asset allocated session")
		}
	}
	w := cloudRequest(t, app, "GET", "/manifest.webmanifest", nil, "")
	var manifest map[string]any
	if json.Unmarshal(w.Body.Bytes(), &manifest) != nil || manifest["display"] != "standalone" {
		t.Fatal(w.Body)
	}
	sw := cloudRequest(t, app, "GET", "/sw.js", nil, "").Body.String()
	if strings.Contains(sw, "cache.put") || strings.Contains(sw, "addAll(['/')") {
		t.Fatal("account page cache introduced")
	}
}
func TestCloudPageIsWebAndHasFullModules(t *testing.T) {
	app := cloudTest(t)
	w := cloudRequest(t, app, "GET", "/", nil, "")
	text := w.Body.String()
	for _, fragment := range []string{"SYNERGIA POCKET TEST", "manifest.webmanifest", "apple-mobile-web-app-capable", `data-section="messages"`, `data-section="grades"`, `data-section="schoolfiles"`, `data-section="updates"`, "serviceWorker.register"} {
		if !strings.Contains(text, fragment) {
			t.Fatal(fragment)
		}
	}
	for _, fragment := range []string{"Windows x64", "127.0.0.1 •", "Zamknąć lokalny program", "To powiadomienia wewnątrz programu Windows"} {
		if strings.Contains(text, fragment) {
			t.Fatal("desktop wording remains", fragment)
		}
	}
}
func TestCloudLoginSlotsDoNotBlockExistingSessions(t *testing.T) {
	app := cloudTest(t)
	cookie := cloudNewBrowser(t, app)
	for i := 0; i < cap(app.loginSlots); i++ {
		app.loginSlots <- struct{}{}
	}
	w := cloudRequest(t, app, "POST", "/api/login", cookie, app.origin)
	if w.Code != 429 {
		t.Fatal(w.Code)
	}
	w = cloudRequest(t, app, "GET", "/api/updates/profiles", cookie, "")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
}
