package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func diagnosticRequest(app *LocalApp, path string, authorized bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3030"+path, nil)
	if authorized {
		req.AddCookie(&http.Cookie{Name: "synergia_local", Value: app.token})
	}
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, req)
	return w
}
func parseDiagnostic(t *testing.T, w *httptest.ResponseRecorder) diagnosticResult {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	var r diagnosticResult
	if e := json.Unmarshal(w.Body.Bytes(), &r); e != nil {
		t.Fatal(e)
	}
	return r
}

func TestV09DiagnosticsSecurityAndMethodValidation(t *testing.T) {
	app := &LocalApp{token: "fake-secret", origin: "http://127.0.0.1:3030"}
	for _, path := range []string{
		"/api/diagnostics/check?profile=0&module=me", "/api/diagnostics/modules", "/api/diagnostics/sample-file",
	} {
		if w := diagnosticRequest(app, path, false); w.Code != 403 {
			t.Fatalf("%s: unauthorized HTTP %d", path, w.Code)
		}
	}
	for _, path := range []string{
		"/api/diagnostics/check?profile=0&module=not-a-module",
		"/api/diagnostics/check?profile=-1&module=grades",
		"/api/diagnostics/check?profile=2&module=messages",
		"/api/diagnostics/check?profile=../x&module=me",
	} {
		if w := diagnosticRequest(app, path, true); w.Code != 400 {
			t.Fatalf("%s: invalid input HTTP %d", path, w.Code)
		}
	}
	if w := diagnosticRequest(app, "/api/diagnostics/check?profile=0&module=me", true); w.Code != 401 {
		t.Fatalf("missing school login should be HTTP 401, got %d", w.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:3030/api/diagnostics/check?profile=0&module=grades", strings.NewReader("test"))
	req.AddCookie(&http.Cookie{Name: "synergia_local", Value: app.token})
	req.Header.Set("Origin", app.origin)
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, req)
	if w.Code != 405 {
		t.Fatalf("diagnostic POST should be rejected, got %d", w.Code)
	}
}

func TestV09DiagnosticModuleCatalogNoSecrets(t *testing.T) {
	app := &LocalApp{token: "fake-secret", origin: "http://127.0.0.1:3030"}
	w := diagnosticRequest(app, "/api/diagnostics/modules", true)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var data struct {
		Modules []diagnosticDefinition `json:"modules"`
		Mode    string                 `json:"mode"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if data.Mode != "manual_only" || len(data.Modules) != 10 {
		t.Fatalf("unexpected modules: %+v", data)
	}
	if strings.Contains(strings.ToLower(w.Body.String()), "password") || strings.Contains(w.Body.String(), app.token) {
		t.Fatal("sensitive data in response")
	}
	seen := map[string]bool{}
	for _, module := range data.Modules {
		if seen[module.ID] || !diagnosticKnownModule(module.ID) {
			t.Fatal(module.ID)
		}
		seen[module.ID] = true
	}
}

func TestV09DiagnosticsFailedHomeworkDoesNotBlockGrades(t *testing.T) {
	client, m := fakeAccount()
	app := &LocalApp{token: "local-secret", origin: "http://127.0.0.1:3030"}
	app.profiles[0] = client
	m.add("GET", dataBase+"/HomeWorkAssignments", record{503, "unavailable", "", ""})
	fail := parseDiagnostic(t, diagnosticRequest(app, "/api/diagnostics/check?profile=0&module=homework", true))
	if fail.State != "failed" || fail.ErrorKind != "upstream" || fail.Count != 0 {
		t.Fatal(fail)
	}
	m.add("GET", dataBase+"/Grades", record{200, `{"Grades":[{"Grade":"4","Subject":{"Id":5},"Category":{"Id":2}}]}`, "", ""})
	m.add("GET", dataBase+"/Subjects", record{200, `{"Subjects":[{"Id":5,"Name":"Niepokazuj Nazwiska"}]}`, "", ""})
	m.add("GET", dataBase+"/Grades/Categories", record{200, `{"Categories":[{"Id":2,"Name":"Sekretne dane testowe"}]}`, "", ""})
	grade := parseDiagnostic(t, diagnosticRequest(app, "/api/diagnostics/check?profile=0&module=grades", true))
	if grade.State != "ok" || grade.Count != 1 {
		t.Fatal(grade)
	}
	content, _ := json.Marshal(grade)
	if strings.Contains(string(content), "Niepokazuj") || strings.Contains(string(content), "Sekretne") || strings.Contains(string(content), "4\"") {
		t.Fatal("PII leaked into diagnostic result")
	}
	for _, call := range m.calls {
		if strings.HasPrefix(call, "POST ") {
			t.Fatal("diagnostics performed write", m.calls)
		}
	}
}

func TestV09DiagnosticsEmptyIsNotFailure(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/HomeWorkAssignments", record{200, `{"HomeWorkAssignments":[]}`, "", ""})
	got := c.diagnosticProbe(context.Background(), "homework")
	if got.State != "empty" || got.Count != 0 || got.ErrorKind != "" {
		t.Fatal(got)
	}
	if len(m.calls) != 1 {
		t.Fatal(m.calls)
	}
}
func TestV09AttachmentsAlwaysUnverifiedWithoutRealTransfer(t *testing.T) {
	for _, tc := range []struct {
		body  string
		count int
	}{
		{`{"HomeWorkAssignments":[]}`, 0},
		{`{"HomeWorkAssignments":[{"Id":1,"HomeworkAssigmentFiles":[]}]}`, 0},
		{`{"HomeWorkAssignments":[{"Id":1,"HomeworkAssigmentFiles":[{"Id":3,"Name":"a.pdf"},{"Id":4,"Name":"b.pdf"}]}]}`, 2},
	} {
		c, m := fakeAccount()
		m.add("GET", dataBase+"/HomeWorkAssignments", record{200, tc.body, "", ""})
		got := c.diagnosticProbe(context.Background(), "attachments")
		if got.State != "unverified" || got.Count != tc.count || got.ErrorKind != "" {
			t.Fatalf("%+v", got)
		}
		if len(m.calls) != 1 || strings.Contains(m.calls[0], "sandbox") || strings.Contains(m.calls[0], "downloadFile") {
			t.Fatal("download attempted", m.calls)
		}
	}
}
func TestV09AttachmentsMalformedResponseDoesNotBecomeEmpty(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/HomeWorkAssignments", record{200, `{"HomeWorks":[]}`, "", ""})
	got := c.diagnosticProbe(context.Background(), "attachments")
	if got.State != "failed" || got.ErrorKind != "format" {
		t.Fatal(got)
	}
}
func TestV09SessionExpiredIsPerModuleStatus(t *testing.T) {
	c, m := fakeAccount()
	c.loggedAt = time.Time{}
	got := c.diagnosticProbe(context.Background(), "grades")
	if got.State != "blocked" || got.ErrorKind != "session" || len(m.calls) != 0 {
		t.Fatal(got, m.calls)
	}
}
func TestV09DiagnosticDistinguishes403And401And429(t *testing.T) {
	for _, tc := range []struct {
		code int
		kind string
	}{{403, "permissions"}, {401, "session_or_access"}, {429, "rate_limit"}, {404, "endpoint"}} {
		c, m := fakeAccount()
		m.add("GET", dataBase+"/HomeWorkAssignments", record{tc.code, "server-rejected", "", ""})
		if tc.code == 401 {
			m.add("GET", refresh, record{403, "expired", "", ""})
		}
		got := c.diagnosticProbe(context.Background(), "attachments")
		if got.State != "failed" || got.ErrorKind != tc.kind {
			t.Fatalf("status=%d result=%+v", tc.code, got)
		}
	}
}
func TestV09LocalFileDoesNotConnectToLibrus(t *testing.T) {
	app := &LocalApp{token: "sample-local-token", origin: "http://127.0.0.1:3030"}
	w := diagnosticRequest(app, "/api/diagnostics/sample-file", true)
	if w.Code != 200 || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Header())
	}
	if !strings.Contains(w.Body.String(), "Nie pobrano go z Librusa") || !strings.Contains(w.Header().Get("Content-Disposition"), "TESTOWY") {
		t.Fatal(w.Body.String())
	}
	if strings.Contains(w.Body.String(), app.token) {
		t.Fatal("local cookie included")
	}
}
func TestV09EachProfileHasSeparateDiagnostics(t *testing.T) {
	c1, m1 := fakeAccount()
	c2, m2 := fakeAccount()
	app := &LocalApp{token: "local-secret", origin: "http://127.0.0.1:3030"}
	app.profiles[0] = c1
	app.profiles[1] = c2
	m1.add("GET", dataBase+"/HomeWorkAssignments", record{200, `{"HomeWorkAssignments":[]}`, "", ""})
	m2.add("GET", dataBase+"/HomeWorkAssignments", record{200, `{"HomeWorkAssignments":[{"Id":999,"HomeworkAssigmentFiles":[{"Id":19,"Name":"test.pdf"}]}]}`, "", ""})
	one := parseDiagnostic(t, diagnosticRequest(app, "/api/diagnostics/check?profile=0&module=attachments", true))
	two := parseDiagnostic(t, diagnosticRequest(app, "/api/diagnostics/check?profile=1&module=attachments", true))
	if one.Count != 0 || two.Count != 1 || len(m1.calls) != 1 || len(m2.calls) != 1 {
		t.Fatal(one, two, m1.calls, m2.calls)
	}
	if c1 == c2 || c1.jar == c2.jar {
		t.Fatal("profiles share sessions")
	}
}
func TestV09DiagnosticRejectsUnexpectedModulesWithoutRequests(t *testing.T) {
	c, m := fakeAccount()
	app := &LocalApp{token: "ok", origin: "http://127.0.0.1:3030"}
	app.profiles[0] = c
	for _, path := range []string{"/api/diagnostics/check?profile=0&module=../../admin", "/api/diagnostics/check?profile=0&module=unknown", "/api/diagnostics/check?profile=0&module=homework%3Fdanger=1"} {
		if x := diagnosticRequest(app, path, true); x.Code != 400 {
			t.Fatalf("unexpected status: %d", x.Code)
		}
	}
	if len(m.calls) != 0 {
		t.Fatal("unexpected upstream calls", m.calls)
	}
}
func TestV09OldModulesSourceIdenticalToV08(t *testing.T) {
	frozen := map[string]string{
		"module_attachment.go": "72b0308e13807c5f39cac14e02ec9a79feb640f29deb63aca19c251e7e6ae1db",
		"module_homework.go":   "8616f1bf21b3195490d9e00a1b428cf92a873a62592377cac181bb055259e595",
		"module_agenda.go":     "87efbe20a6b09cf2fccdd7371db6c4434e1732aba2bb52b1dde493417b968cc9",
		"module_attendance.go": "c120c3dafbfd44664a309ce7804cc592d9ac56064443678bb278433c7b59a68f",
		"module_notes.go":      "b8c7e712534355b09bb7f05af519e5245fd3b62c337a85f83278be238984e84f",
	}
	for name, expected := range frozen {
		data, e := os.ReadFile(name)
		if e != nil {
			t.Fatal(e)
		}
		hash := sha256.Sum256(data)
		if hex.EncodeToString(hash[:]) != expected {
			t.Fatalf("regression: %s", name)
		}
	}
}
func TestV09FrontendContainsUserConsentAndClearSimulationLabels(t *testing.T) {
	if !strings.Contains(htmlPage, "data-section=\"diagnostics\"") || !strings.Contains(htmlPage, "Sprawdź wszystkie moduły") || !strings.Contains(htmlPage, "Przykład — DEMO") || !strings.Contains(htmlPage, "diagStateText") {
		t.Fatal("diagnostics controls missing")
	}
	if !strings.Contains(htmlPage, "Plik wygenerowany lokalnie") && !strings.Contains(htmlPage, "plik testowy") {
		t.Fatal("missing local test file label")
	}
	if strings.Contains(htmlPage, "api/diagnostics/check')") {
		t.Fatal("diagnostics starts automatically")
	}
}

func TestV09FrozenV08AttachmentUIActions(t *testing.T) {
	frozen := map[string]string{
		" async function showHomeworkFiles(":    "a12c31fd9686c0e4098d09a3e8bfd16d6346d2beed7b8968b16f5ed2a6bf5d5f",
		" async function downloadHomeworkFile(": "b326841cea408aa72be1c58feddf63990f952556b0b803cd24de73c1e7f68367",
	}
	for prefix, expected := range frozen {
		i := strings.Index(htmlPage, prefix)
		if i < 0 {
			t.Fatalf("missing frozen UI function %s", prefix)
		}
		end := strings.Index(htmlPage[i:], "\n }")
		if end < 0 {
			t.Fatal("cannot find end of frozen function", prefix)
		}
		text := htmlPage[i : i+end+3]
		digest := sha256.Sum256([]byte(text))
		if hex.EncodeToString(digest[:]) != expected {
			t.Fatalf("v0.8 attachment action changed: %s", prefix)
		}
	}
}
