package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

const testHomeworkAssignment = `{"HomeWorkAssignments":[{"Id":101,"Topic":"Zdanie","HomeworkAssigmentFiles":[{"Id":200,"Name":"kartka.pdf","Url":"https://malicious.example/not-used"},{"Id":201,"Name":"../../CON.txt"}]},{"Id":102,"Topic":"Inne","HomeworkAssigmentFiles":[{"Id":300,"Name":"other.txt"}]}]}`
const testSandboxLink = "https://sandbox.librus.pl/index.php?action=CSTryToDownload&singleUseKey=TESTKEY9876"

func prepareHomeworkMock(a *APIClient, m *mockedHTTP) {
	m.add("GET", dataBase+"/HomeWorkAssignments", record{200, testHomeworkAssignment, "", ""})
}
func prepareSandboxFlow(m *mockedHTTP) {
	m.add("GET", "https://synergia.librus.pl/homework/downloadFile/200", record{302, "", testSandboxLink, ""})
	m.add("GET", testSandboxLink, record{200, "<!doctype html><html>Przygotowywanie...</html>", "", ""})
	m.add("POST", sandboxHome+"?action=CSCheckKey", record{200, `{"status":"ready"}`, "", ""})
	m.add("GET", sandboxHome+"?action=CSDownload&singleUseKey=TESTKEY9876", record{200, "%PDF-test-content", "", ""})
}
func TestV08AttachmentListVerifiedFromProfileOnly(t *testing.T) {
	a, m := fakeAccount()
	prepareHomeworkMock(a, m)
	rows, err := a.verifiedHomeworkFiles(context.Background(), "101")
	if err != nil || len(rows) != 2 {
		t.Fatalf("got %+v %v", rows, err)
	}
	if rows[0].ID != "200" || rows[0].Name != "kartka.pdf" || rows[0].AssignmentID != "101" {
		t.Fatal(rows[0])
	}
	if strings.Contains(rows[1].Name, "/") || strings.HasPrefix(strings.ToUpper(rows[1].Name), "CON.") {
		t.Fatal(rows[1].Name)
	}
	if len(m.calls) != 1 || strings.Contains(strings.Join(m.calls, "\n"), "malicious.example") {
		t.Fatal(m.calls)
	}
}
func TestV08RejectUnauthorizedOrMalformedIDs(t *testing.T) {
	for _, p := range []string{"abc", "102/../../x", "1?x=1", "", "-1", strings.Repeat("9", 45)} {
		a, m := fakeAccount()
		if _, err := a.verifiedHomeworkFiles(context.Background(), p); err == nil || len(m.calls) != 0 {
			t.Errorf("accepted invalid id %q", p)
		}
	}
	a, m := fakeAccount()
	prepareHomeworkMock(a, m)
	if _, err := a.verifiedHomeworkFiles(context.Background(), "9999"); err == nil {
		t.Fatal("unknown assignment accepted")
	}
}
func TestV08SandboxAllowlistRejectsSpoofedHostsAndUnsafeURLs(t *testing.T) {
	bad := []string{
		"http://sandbox.librus.pl/GetFile/13", "https://sandbox.librus.pl.evil.test/GetFile/13",
		"https://evil.test/GetFile/13", "https://sandbox.librus.pl:444/GetFile/13",
		"https://sandbox.librus.pl/index.php?action=CSDownload&singleUseKey=TESTKEY9876",
		"https://sandbox.librus.pl/index.php?action=CSTryToDownload&singleUseKey=a/b",
		"https://sandbox.librus.pl/index.php?action=CSTryToDownload&singleUseKey=<script>",
		"https://sandbox.librus.pl/private", "https://sandbox.librus.pl/GetFile/../secret",
		"https://sandbox.librus.pl/GetFile/134#frag",
	}
	for _, p := range bad {
		if _, err := sandboxDownloadLink(p); err == nil {
			t.Errorf("allowed %s", p)
		}
	}
	for _, p := range []string{testSandboxLink, "https://sandbox.librus.pl/GetFile/234"} {
		if _, err := sandboxDownloadLink(p); err != nil {
			t.Errorf("blocked %s: %v", p, err)
		}
	}
}
func TestV08DownloadOneAttachmentThroughSandboxAndHTTPHeaders(t *testing.T) {
	a, m := fakeAccount()
	prepareHomeworkMock(a, m)
	prepareSandboxFlow(m)
	app := &LocalApp{token: "secret-cookie", origin: "http://127.0.0.1:3030"}
	app.profiles[0] = a
	req := httptest.NewRequest("GET", "http://127.0.0.1:3030/api/homework/download?profile=0&assignment=101&file=200", nil)
	req.AddCookie(&http.Cookie{Name: "synergia_local", Value: "secret-cookie"})
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, req)
	if w.Code != 200 || w.Body.String() != "%PDF-test-content" {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "application/octet-stream" || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Header())
	}
	_, params, err := mime.ParseMediaType(w.Header().Get("Content-Disposition"))
	if err != nil || params["filename"] != "kartka.pdf" {
		t.Fatal(w.Header().Get("Content-Disposition"), err)
	}
	if len(m.calls) != 5 {
		t.Fatal(m.calls)
	}
	for _, call := range m.calls {
		if strings.Contains(call, "malicious.example") || strings.Contains(call, "https://evil") {
			t.Fatalf("leaked to unknown host %s", call)
		}
	}
}
func TestV08NoAutomaticDownloadOnListOrOtherModules(t *testing.T) {
	a, m := fakeAccount()
	prepareHomeworkMock(a, m)
	app := &LocalApp{token: "secure", origin: "http://127.0.0.1:3123"}
	app.profiles[0] = a
	req := httptest.NewRequest("GET", "http://127.0.0.1:3123/api/homework/files?profile=0&assignment=101", nil)
	req.AddCookie(&http.Cookie{Name: "synergia_local", Value: "secure"})
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, req)
	var result struct {
		Files []homeworkAttachment `json:"files"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Files) != 2 {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(m.calls) != 1 {
		t.Fatal(m.calls)
	}
}
func TestV08NoCrossProfileSessionOrForeignFile(t *testing.T) {
	a, m := fakeAccount()
	app := &LocalApp{token: "private", origin: "http://127.0.0.1:3123"}
	app.profiles[0] = a
	cases := []struct {
		uri  string
		code int
	}{
		{"/api/homework/files?profile=1&assignment=101", 401},
		{"/api/homework/download?profile=1&assignment=101&file=200", 401},
		{"/api/homework/download?profile=0&assignment=xyz&file=200", 400},
		{"/api/homework/download?profile=0&assignment=101&file=../../123", 400},
	}
	for _, test := range cases {
		req := httptest.NewRequest("GET", "http://127.0.0.1:3123"+test.uri, nil)
		req.AddCookie(&http.Cookie{Name: "synergia_local", Value: "private"})
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, req)
		if w.Code != test.code {
			t.Errorf("%s expected %d got %d", test.uri, test.code, w.Code)
		}
	}
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:3123/api/homework/files?profile=0&assignment=101", nil))
	if w.Code != 403 {
		t.Fatal("unauthorized local access allowed")
	}
	if len(m.calls) != 0 {
		t.Fatal("unauthorized requests reached the school API", m.calls)
	}
}
func TestV08AssignmentOwnershipIsRecheckedBeforeDownload(t *testing.T) {
	a, m := fakeAccount()
	prepareHomeworkMock(a, m)
	_, err := a.downloadHomeworkAttachment(context.Background(), "101", "300")
	if err == nil || len(m.calls) != 1 {
		t.Fatal("cross-assignment download allowed", err, m.calls)
	}
}
func TestV08DeniedSandboxRedirectDoesNotLeakSession(t *testing.T) {
	a, m := fakeAccount()
	prepareHomeworkMock(a, m)
	m.add("GET", "https://synergia.librus.pl/homework/downloadFile/200", record{302, "", "https://attacker.example/test", ""})
	_, err := a.downloadHomeworkAttachment(context.Background(), "101", "200")
	if err == nil || len(m.calls) != 2 {
		t.Fatal("foreign redirect followed", err, m.calls)
	}
}
func TestV08SandboxStatusPollingWithoutRealSleep(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", testSandboxLink, record{200, "<html>working</html>", "", ""})
	m.add("POST", sandboxHome+"?action=CSCheckKey", record{200, `{"status":"not_downloaded_yet"}`, "", ""})
	m.add("POST", sandboxHome+"?action=CSCheckKey", record{200, `{"status":"ready"}`, "", ""})
	m.add("GET", sandboxHome+"?action=CSDownload&singleUseKey=TESTKEY9876", record{200, "PNG!", "", ""})
	sleeps := 0
	pause := func(ctx context.Context, d time.Duration) error { sleeps++; return nil }
	got, err := a.sandboxFile(context.Background(), testSandboxLink, "foto.png", pause)
	if err != nil || string(got.Content) != "PNG!" || sleeps != 1 || len(m.calls) != 4 {
		t.Fatal(err, got, sleeps, m.calls)
	}
}
func TestV08SandboxFailureRequiresExplicitRetry(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", testSandboxLink, record{200, "waiting", "", ""})
	m.add("POST", sandboxHome+"?action=CSCheckKey", record{200, `{"status":"download_failed"}`, "", ""})
	_, err := a.sandboxFile(context.Background(), testSandboxLink, "file.txt", sandboxSleep)
	if !errors.Is(err, errSandboxKeyFailed) || len(m.calls) != 2 {
		t.Fatal("unexpected retry", err, m.calls)
	}
}
func TestV08GetFileFallbackNoHTMLAsFile(t *testing.T) {
	a, m := fakeAccount()
	link := "https://sandbox.librus.pl/GetFile/1234"
	m.add("GET", link, record{200, "<html>waiting</html>", "", ""})
	m.add("GET", link+"/get", record{200, "<html>waiting", "", ""})
	// Only one attempt, and a canceled context stops retrying promptly.
	ctx, cancel := context.WithCancel(context.Background())
	pause := func(context.Context, time.Duration) error { cancel(); return context.Canceled }
	_, err := a.sandboxFile(ctx, link, "test.pdf", pause)
	if err == nil {
		t.Fatal("html returned as file")
	}
}
func TestV08HTMLResponseNeverReturnedAsDocument(t *testing.T) {
	resp := &http.Response{StatusCode: 200, Header: make(http.Header)}
	for _, s := range []string{"<!DOCTYPE html><html>", "<html>Access denied</html>"} {
		if _, err := readHomeworkBinary(resp, []byte(s), "file.pdf"); err == nil {
			t.Fatal("HTML accepted as file")
		}
	}
}
func TestV08FilenamesAreSanitized(t *testing.T) {
	bad := []string{"../../secret.pdf", "C:\\Users\\abc\\evil.txt", "CON.txt", "LPT1.pdf", "x\x00y.pdf", "a\u202Eexe.txt"}
	for _, b := range bad {
		name := safeHomeworkName(b)
		if strings.ContainsAny(name, "/\\\x00") || strings.HasPrefix(name, ".") || strings.ContainsRune(name, '\u202e') || name == "" {
			t.Fatalf("unsanitized %q -> %q", b, name)
		}
	}
	resp := &http.Response{StatusCode: 200, Header: make(http.Header)}
	resp.Header.Set("Content-Disposition", `attachment; filename="../../oops.pdf"`)
	out, err := readHomeworkBinary(resp, []byte("PDF"), "fallback.pdf")
	if err != nil || out.Name != "oops.pdf" {
		t.Fatal(out, err)
	}
}
func TestV08LargeFileIsBlocked(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", testSandboxLink, record{200, "waiting", "", ""})
	m.add("POST", sandboxHome+"?action=CSCheckKey", record{200, `{"status":"ready"}`, "", ""})
	oversized := strings.Repeat("x", maxHomeworkFile+1)
	m.add("GET", sandboxHome+"?action=CSDownload&singleUseKey=TESTKEY9876", record{200, oversized, "", ""})
	_, err := a.sandboxFile(context.Background(), testSandboxLink, "big.bin", sandboxSleep)
	if err == nil || !strings.Contains(err.Error(), "20 MB") {
		t.Fatal("oversized attachment accepted", err)
	}
}
func TestV08RegressionsFrontendAndRouteAreNewOnly(t *testing.T) {
	// Golden v0.7 source files are compared in build/verification; here
	// ensure the client never guesses filenames or uses metadata URLs.
	a, m := fakeAccount()
	prepareHomeworkMock(a, m)
	_, _ = a.verifiedHomeworkFiles(context.Background(), "101")
	if len(m.calls) != 1 {
		t.Fatal(m.calls)
	}
}
func TestV08SandboxDoesNotInventRetriesForWriteAPI(t *testing.T) {
	a, m := fakeAccount()
	prepareHomeworkMock(a, m)
	_, _ = a.verifiedHomeworkFiles(context.Background(), "101")
	for _, call := range m.calls {
		if strings.HasPrefix(call, "POST ") {
			t.Fatal("unexpected POST while listing homework files")
		}
	}
}

var _ io.Reader // make compiler enforce imports stay clean

func TestV08GetFileFallbackActualBinary(t *testing.T) {
	a, m := fakeAccount()
	link := "https://sandbox.librus.pl/GetFile/7777"
	m.add("GET", link, record{200, "<html>Przygotowywanie</html>", "", ""})
	m.add("GET", link+"/get", record{200, "JPEG-contents", "", ""})
	out, err := a.sandboxFile(context.Background(), link, "zdjecie.jpg", sandboxSleep)
	if err != nil || out.Name != "zdjecie.jpg" || string(out.Content) != "JPEG-contents" || len(m.calls) != 2 {
		t.Fatal(out, err, m.calls)
	}
}

func TestV08FreshSandboxKeyAfterDownloadFailed(t *testing.T) {
	a, m := fakeAccount()
	prepareHomeworkMock(a, m)
	firstLink := "https://sandbox.librus.pl/index.php?action=CSTryToDownload&singleUseKey=FIRSTKEY9999"
	secondLink := "https://sandbox.librus.pl/index.php?action=CSTryToDownload&singleUseKey=SECONDKEY8888"
	m.add("GET", "https://synergia.librus.pl/homework/downloadFile/200", record{302, "", firstLink, ""})
	m.add("GET", firstLink, record{200, "waiting", "", ""})
	m.add("POST", sandboxHome+"?action=CSCheckKey", record{200, `{"status":"download_failed"}`, "", ""})
	prepareHomeworkMock(a, m)
	m.add("GET", "https://synergia.librus.pl/homework/downloadFile/200", record{302, "", secondLink, ""})
	m.add("GET", secondLink, record{200, "waiting", "", ""})
	m.add("POST", sandboxHome+"?action=CSCheckKey", record{200, `{"status":"ready"}`, "", ""})
	m.add("GET", sandboxHome+"?action=CSDownload&singleUseKey=SECONDKEY8888", record{200, "GOOD-DATA", "", ""})
	got, err := a.downloadHomeworkAttachment(context.Background(), "101", "200")
	if err != nil || string(got.Content) != "GOOD-DATA" {
		t.Fatal(err, got, m.calls)
	}
	if len(m.calls) != 9 {
		t.Fatal("wrong number of requests", m.calls)
	}
}

func TestV08DownloadErrorDoesNotBreakPriorFunctions(t *testing.T) {
	a, m := fakeAccount()
	prepareHomeworkMock(a, m)
	m.add("GET", "https://synergia.librus.pl/homework/downloadFile/200", record{403, "Brak dostępu", "", ""})
	_, err := a.downloadHomeworkAttachment(context.Background(), "101", "200")
	if err == nil {
		t.Fatal("download must reject expired web session")
	}
	m.add("GET", dataBase+"/Grades", record{200, `{"Grades":[{"Id":1,"Grade":"6","Subject":{"Id":3}}]}`, "", ""})
	m.add("GET", dataBase+"/Subjects", record{200, `{"Subjects":[{"Id":3,"Name":"Fizyka"}]}`, "", ""})
	m.add("GET", dataBase+"/Grades/Categories", record{200, `{"Categories":[]}`, "", ""})
	grade, err := a.getGrades(context.Background())
	if err != nil || len(grade) != 1 {
		t.Fatal("grade regression after download failure", err, grade)
	}
}

func TestV08FrozenV07HomeworkFileUnchanged(t *testing.T) {
	raw, err := os.ReadFile("../BASELINE_V07_HOMEWORK_LOCK.json")
	if err != nil {
		t.Fatal(err)
	}
	var checksums map[string]string
	if err := json.Unmarshal(raw, &checksums); err != nil {
		t.Fatal(err)
	}
	if len(checksums) != 1 {
		t.Fatal("expected exactly one frozen module")
	}
	for file, want := range checksums {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		got := sha256.Sum256(content)
		if hex.EncodeToString(got[:]) != want {
			t.Fatalf("REGRESSION: %s changed", file)
		}
	}
}

type blockingAttachmentTransport struct {
	mock    *mockedHTTP
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingAttachmentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == "sandbox.librus.pl" && req.URL.Query().Get("action") == "CSTryToDownload" {
		b.once.Do(func() { close(b.started) })
		<-b.release
	}
	return b.mock.RoundTrip(req)
}
func TestV08OtherModulesCanWorkWhileDownloadWaitsForSandbox(t *testing.T) {
	a, m := fakeAccount()
	a.client.Transport = &blockingAttachmentTransport{mock: m, started: make(chan struct{}), release: make(chan struct{})}
	gate := a.client.Transport.(*blockingAttachmentTransport)
	prepareHomeworkMock(a, m)
	prepareSandboxFlow(m)
	m.add("GET", dataBase+"/Grades", record{200, `{"Grades":[{"Id":1,"Grade":"5"}]}`, "", ""})
	m.add("GET", dataBase+"/Subjects", record{200, `{"Subjects":[]}`, "", ""})
	m.add("GET", dataBase+"/Grades/Categories", record{200, `{"Categories":[]}`, "", ""})
	downloadDone := make(chan error, 1)
	go func() {
		_, err := a.downloadHomeworkAttachment(context.Background(), "101", "200")
		downloadDone <- err
	}()
	select {
	case <-gate.started:
	case <-time.After(time.Second):
		close(gate.release)
		t.Fatal("download never reached sandbox")
	}
	gradesDone := make(chan error, 1)
	go func() { a.mu.Lock(); _, err := a.getGrades(context.Background()); a.mu.Unlock(); gradesDone <- err }()
	select {
	case err := <-gradesDone:
		if err != nil {
			t.Fatalf("grades blocked by download: %v", err)
		}
	case <-time.After(time.Second):
		close(gate.release)
		t.Fatal("download blocked unrelated grades module")
	}
	close(gate.release)
	select {
	case err := <-downloadDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("download didn't resume")
	}
}
