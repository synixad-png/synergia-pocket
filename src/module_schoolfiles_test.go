package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

const sampleSchoolFiles = `{"Data":[{"id":83,"displayName":"Plan pracy.pdf","addedOnDate":"2026-11-03","fileStatus":"active","downloadUrl":"/pliki_szkoly/pobierz/83"},{"id":"ZX-7","displayName":"../sekret\\nazwa.docx","addedOnDate":"2026-11-02","downloadUrl":"/pliki_szkoly/pobierz/ZX-7"}]}`

func schoolNoWait(ctx context.Context, d time.Duration) error { return nil }
func mockSchoolFile(c *APIClient, m *mockedHTTP, id, path, location string) {
	m.add("GET", dataBase+"/SchoolFiles", record{200, `{"Data":[{"id":"` + id + `","displayName":"Dokument.pdf","downloadUrl":"` + path + `"}]}`, "", ""})
	if location != "" {
		m.add("GET", "https://synergia.librus.pl"+path, record{302, "", location, ""})
	}
}

func TestV10SchoolListUsesOnlyOwnEndpoint(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/SchoolFiles", record{200, sampleSchoolFiles, "", ""})
	docs, err := c.schoolDocuments(context.Background())
	if err != nil || len(docs) != 2 {
		t.Fatalf("docs=%+v err=%v", docs, err)
	}
	if docs[0].ID != "83" || docs[0].Name != "Plan pracy.pdf" || docs[0].Date != "2026-11-03" || !docs[0].Downloadable || docs[0].Status != "active" {
		t.Fatal(docs)
	}
	if docs[1].Name != "nazwa.docx" {
		t.Fatal(docs[1])
	}
	if len(m.calls) != 1 || m.calls[0] != "GET "+dataBase+"/SchoolFiles" {
		t.Fatal(m.calls)
	}
}
func TestV10EmptySchoolDocumentsIsValid(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/SchoolFiles", record{200, `{"Data":[]}`, "", ""})
	docs, err := c.schoolDocuments(context.Background())
	if err != nil || docs == nil || len(docs) != 0 {
		t.Fatal(docs, err)
	}
}
func TestV10MalformedDataIsNotMisreportedEmpty(t *testing.T) {
	for _, body := range []string{`{"SchoolFiles":[]}`, `{"Data":null}`, `{"Data":{}}`, `[]`} {
		c, m := fakeAccount()
		m.add("GET", dataBase+"/SchoolFiles", record{200, body, "", ""})
		_, err := c.schoolDocuments(context.Background())
		if err == nil {
			t.Fatal("missing error", body)
		}
	}
}
func TestV10MalformedDocumentIDsDoNotLookLikeEmptyCollection(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/SchoolFiles", record{200, `{"Data":[{"id":"../etc","displayName":"invalid"}]}`, "", ""})
	if rows, err := c.schoolDocuments(context.Background()); err == nil {
		t.Fatal("should report unexpected ids, not empty success", rows)
	}
}

func TestV10ForbiddenModuleCannotBreakOtherModules(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/SchoolFiles", record{403, "forbidden", "", ""})
	m.add("GET", dataBase+"/HomeWorks", record{200, `{"HomeWorks":[]}`, "", ""})
	m.add("GET", dataBase+"/Grades", record{200, `{"Grades":[]}`, "", ""})
	m.add("GET", dataBase+"/Subjects", record{200, `{"Subjects":[]}`, "", ""})
	m.add("GET", dataBase+"/Grades/Categories", record{200, `{"Categories":[]}`, "", ""})
	if _, e := c.schoolDocuments(context.Background()); e == nil {
		t.Fatal("expected 403")
	}
	if a, e := c.getAgendaV06(context.Background()); e != nil || len(a.items) != 0 {
		t.Fatal(a, e)
	}
	if g, e := c.getGrades(context.Background()); e != nil || len(g) != 0 {
		t.Fatal(g, e)
	}
}
func TestV10MaliciousPathsNeverBecomeDownloadable(t *testing.T) {
	for _, p := range []string{"http://evil.test/x", "https://synergia.librus.pl/pliki_szkoly/pobierz/4", "//evil.test/file", "/pliki_szkoly/pobierz/../4", "/pliki_szkoly/pobierz/4?x=1", "/pliki_szkoly/pobierz/4#token", "/pliki_szkoly/pobierz/%2e%2e", "/pliki_szkoly/pobierz/4%2f5", "/homework/downloadFile/4"} {
		if parseSchoolDownloadPath(p) != "" {
			t.Fatal("unsafe path allowed", p)
		}
	}
	if parseSchoolDownloadPath("/pliki_szkoly/pobierz/abc-100") == "" {
		t.Fatal("good path blocked")
	}
}
func TestV10SanitizesFileNames(t *testing.T) {
	for _, test := range []struct{ in, want string }{{`C:\\secret\\raport.pdf`, "raport.pdf"}, {"../../x.png", "x.png"}, {"CON.txt", "plik_CON.txt"}, {" \u202eevil.exe ", "evil.exe"}, {"<script>.pdf", "script.pdf"}, {"..", "dokument.bin"}} {
		got := safeSchoolFileName(test.in)
		if got != test.want {
			t.Fatalf("%q -> %q, expected %q", test.in, got, test.want)
		}
	}
}
func TestV10UnknownDocumentsNotDownloadable(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/SchoolFiles", record{200, sampleSchoolFiles, "", ""})
	if _, e := c.verifiedSchoolDocument(context.Background(), "777"); e == nil {
		t.Fatal("other user document allowed")
	}
	if len(m.calls) != 1 {
		t.Fatal(m.calls)
	}
}
func TestV10UnreadableDownloadUrlStillVisibleInList(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/SchoolFiles", record{200, `{"Data":[{"id":55,"displayName":"Wniosek","downloadUrl":"https://evil.example/file"}]}`, "", ""})
	docs, err := c.schoolDocuments(context.Background())
	if err != nil || len(docs) != 1 || docs[0].Downloadable {
		t.Fatal(docs, err)
	}
}
func TestV10SandboxHostAndLinkValidation(t *testing.T) {
	good := []string{"https://sandbox.librus.pl/GetFile/ABCDEFGH", "https://sandbox.librus.pl/index.php?action=CSTryToDownload&singleUseKey=ABCDEFGH"}
	for _, v := range good {
		if _, e := schoolSandboxLink(v); e != nil {
			t.Fatal(v, e)
		}
	}
	bad := []string{
		"http://sandbox.librus.pl/GetFile/abc", "https://sandbox.librus.pl.evil.test/GetFile/abc", "https://user@sandbox.librus.pl/GetFile/abc", "https://sandbox.librus.pl:9443/GetFile/abc", "https://sandbox.librus.pl/other", "https://sandbox.librus.pl/GetFile/../admin", "https://sandbox.librus.pl/GetFile/a?token=secret", "https://sandbox.librus.pl/index.php?action=CSDownload&singleUseKey=ABCDEFGH", "https://sandbox.librus.pl/index.php?action=CSTryToDownload&singleUseKey=ABCDEFGH&evil=1", "https://sandbox.librus.pl/GetFile/a#fragment",
	}
	for _, v := range bad {
		if _, e := schoolSandboxLink(v); e == nil {
			t.Fatal("unsafe link accepted", v)
		}
	}
}
func TestV10ServerRejectsUntrustedDownloadTargets(t *testing.T) {
	c, m := fakeAccount()
	for _, target := range []string{"https://external.test/", "http://sandbox.librus.pl/GetFile/a", "https://sandbox.librus.pl/evil", "https://synergia.librus.pl/OAuth/Authorization", "https://synergia.librus.pl/pliki_szkoly/pobierz/1?x=1"} {
		if _, _, err := c.schoolHTTP(context.Background(), "GET", target, nil, nil, 100); err == nil {
			t.Fatal("bad target allowed", target)
		}
	}
	if len(m.calls) != 0 {
		t.Fatal("network accessed", m.calls)
	}
}
func TestV10StartSchoolDownloadRestrictsForeignRedirect(t *testing.T) {
	for _, link := range []string{"http://sandbox.librus.pl/GetFile/x", "https://untrusted.example/GetFile/x", "https://sandbox.librus.pl/GetFile/../fake"} {
		c, m := fakeAccount()
		mockSchoolFile(c, m, "10", "/pliki_szkoly/pobierz/10", link)
		_, _, err := c.startSchoolDownload(context.Background(), "10")
		if err == nil {
			t.Fatal("allowed", link)
		}
		if len(m.calls) != 2 {
			t.Fatal(m.calls)
		}
	}
}
func TestV10WebSessionExpiredShowsError(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/SchoolFiles", record{200, `{"Data":[{"id":10,"downloadUrl":"/pliki_szkoly/pobierz/10"}]}`, "", ""})
	m.add("GET", "https://synergia.librus.pl/pliki_szkoly/pobierz/10", record{200, "<html>Brak dostępu</html>", "", ""})
	_, _, err := c.startSchoolDownload(context.Background(), "10")
	if err == nil || !strings.Contains(err.Error(), "Zaloguj") {
		t.Fatal(err)
	}
}
func TestV10SingleUseKeyDownloadFullFlow(t *testing.T) {
	c, m := fakeAccount()
	link := "https://sandbox.librus.pl/index.php?action=CSTryToDownload&singleUseKey=ABCDEF123456"
	mockSchoolFile(c, m, "10", "/pliki_szkoly/pobierz/10", link)
	m.add("GET", link, record{200, "waiting", "", ""})
	m.add("POST", schoolSandbox+"?action=CSCheckKey", record{200, `{"status":"not_downloaded_yet"}`, "", ""})
	m.add("POST", schoolSandbox+"?action=CSCheckKey", record{200, `{"status":"ready"}`, "", ""})
	m.add("GET", schoolSandbox+"?action=CSDownload&singleUseKey=ABCDEF123456", record{200, "%PDF-1.7 fake PDF", "", ""})
	file, err := c.downloadSchoolDocumentWithPause(context.Background(), "10", schoolNoWait)
	if err != nil || file.Name != "Dokument.pdf" || !strings.HasPrefix(string(file.Data), "%PDF") {
		t.Fatalf("file=%+v err=%v calls=%v", file, err, m.calls)
	}
	if len(m.calls) != 6 {
		t.Fatal(m.calls)
	}
}
func TestV10GetFileDownloadFullFlow(t *testing.T) {
	c, m := fakeAccount()
	link := "https://sandbox.librus.pl/GetFile/0123-ABC"
	mockSchoolFile(c, m, "23", "/pliki_szkoly/pobierz/23", link)
	m.add("GET", link, record{200, "Waiting room", "", ""})
	m.add("GET", link+"/get", record{404, "not ready", "", ""})
	m.add("GET", link+"/get", record{200, "DATA-1234", "", ""})
	file, err := c.downloadSchoolDocumentWithPause(context.Background(), "23", schoolNoWait)
	if err != nil || file.Name != "Dokument.pdf" || string(file.Data) != "DATA-1234" {
		t.Fatal(file, err, m.calls)
	}
}
func TestV10SingleUseKeyRetriesOnlyAfterFailedKey(t *testing.T) {
	c, m := fakeAccount()
	linkA := "https://sandbox.librus.pl/index.php?action=CSTryToDownload&singleUseKey=AAAAAAAA"
	linkB := "https://sandbox.librus.pl/index.php?action=CSTryToDownload&singleUseKey=BBBBBBBB"
	for _, link := range []string{linkA, linkB} {
		mockSchoolFile(c, m, "7", "/pliki_szkoly/pobierz/7", link)
		m.add("GET", link, record{200, "wait", "", ""})
	}
	m.add("POST", schoolSandbox+"?action=CSCheckKey", record{200, `{"status":"download_failed"}`, "", ""})
	m.add("POST", schoolSandbox+"?action=CSCheckKey", record{200, `{"status":"ready"}`, "", ""})
	m.add("GET", schoolSandbox+"?action=CSDownload&singleUseKey=BBBBBBBB", record{200, "good", "", ""})
	got, err := c.downloadSchoolDocumentWithPause(context.Background(), "7", schoolNoWait)
	if err != nil || string(got.Data) != "good" {
		t.Fatal(got, err, m.calls)
	}
}
func TestV10HTMLIsNotAFile(t *testing.T) {
	c, m := fakeAccount()
	link := "https://sandbox.librus.pl/GetFile/XYZ123"
	mockSchoolFile(c, m, "2", "/pliki_szkoly/pobierz/2", link)
	m.add("GET", link, record{200, "waiting", "", ""})
	for i := 0; i < 15; i++ {
		m.add("GET", link+"/get", record{200, "<html>login</html>", "", ""})
	}
	_, err := c.downloadSchoolDocumentWithPause(context.Background(), "2", schoolNoWait)
	if err == nil {
		t.Fatal("HTML accepted")
	}
}
func TestV10LocalEndpointsAuthAndProfileIsolation(t *testing.T) {
	app := &LocalApp{token: "secret", origin: "http://127.0.0.1:49000"}
	c, m := fakeAccount()
	app.profiles[0] = c
	m.add("GET", dataBase+"/SchoolFiles", record{200, sampleSchoolFiles, "", ""})
	u := "http://127.0.0.1:49000/api/school/files?profile=0"
	request := func(raw string, cookie bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", raw, nil)
		if cookie {
			r.AddCookie(&http.Cookie{Name: "synergia_local", Value: "secret"})
		}
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, r)
		return w
	}
	if w := request(u, false); w.Code != 403 {
		t.Fatal("unauth", w.Code)
	}
	if w := request("http://127.0.0.1:49000/api/school/files?profile=1", true); w.Code != 401 {
		t.Fatal("cross profile", w.Code)
	}
	w := request(u, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var payload struct {
		Items []schoolDocument `json:"items"`
	}
	if json.Unmarshal(w.Body.Bytes(), &payload) != nil || len(payload.Items) != 2 {
		t.Fatal(w.Body.String())
	}
	if strings.Contains(w.Body.String(), "downloadUrl") || strings.Contains(w.Body.String(), "/pliki_szkoly/") {
		t.Fatal("upstream path leaked")
	}
	if len(m.calls) != 1 {
		t.Fatal("file fetched without click", m.calls)
	}
}
func TestV10RouteRejectsBadFileWithoutNetwork(t *testing.T) {
	app := &LocalApp{token: "secret", origin: "http://127.0.0.1:49000"}
	c, m := fakeAccount()
	app.profiles[0] = c
	for _, id := range []string{"../../x", "http://other", "x%2Fy", ""} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:49000/api/school/download?profile=0&file="+id, nil)
		r.AddCookie(&http.Cookie{Name: "synergia_local", Value: "secret"})
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal(id, w.Code)
		}
	}
	if len(m.calls) > 0 {
		t.Fatal(m.calls)
	}
}
func TestV10RouteDownloadBinaryAndSafetyHeaders(t *testing.T) {
	app := &LocalApp{token: "secret", origin: "http://127.0.0.1:49000"}
	c, m := fakeAccount()
	app.profiles[0] = c
	link := "https://sandbox.librus.pl/index.php?action=CSTryToDownload&singleUseKey=ABCDEFGHIJKL"
	mockSchoolFile(c, m, "81", "/pliki_szkoly/pobierz/81", link)
	m.add("GET", link, record{200, "waiting", "", ""})
	m.add("POST", schoolSandbox+"?action=CSCheckKey", record{200, `{"status":"ready"}`, "", ""})
	m.add("GET", schoolSandbox+"?action=CSDownload&singleUseKey=ABCDEFGHIJKL", record{200, "file-content", "", ""})
	r := httptest.NewRequest("GET", "http://127.0.0.1:49000/api/school/download?profile=0&file=81", nil)
	r.AddCookie(&http.Cookie{Name: "synergia_local", Value: "secret"})
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	if w.Code != 200 || w.Body.String() != "file-content" || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Header(), w.Body.String())
	}
}
func TestV10OldModulesAreBaitProtected(t *testing.T) {
	// SHA-256 values from the verified v0.9 ZIP, independent of v0.10 changes.
	locks := map[string]string{"module_diagnostics.go": "9ef9d47fd96babfe7a5cdb37eb7f5fe24c06e1ff305bba1b032969b2f854afb8", "module_attachment.go": "72b0308e13807c5f39cac14e02ec9a79feb640f29deb63aca19c251e7e6ae1db", "module_homework.go": "8616f1bf21b3195490d9e00a1b428cf92a873a62592377cac181bb055259e595", "module_attendance.go": "c120c3dafbfd44664a309ce7804cc592d9ac56064443678bb278433c7b59a68f", "module_agenda.go": "87efbe20a6b09cf2fccdd7371db6c4434e1732aba2bb52b1dde493417b968cc9", "module_notes.go": "b8c7e712534355b09bb7f05af519e5245fd3b62c337a85f83278be238984e84f", "module_diagnostics_test.go": "0057addbf6520f3526dcfc33f26271417d21862b3830c976ce5d532e764405a4", "module_attachment_test.go": "3538c26392ad713f6fb7d608900abb3c32b2db961360dfa9a21d4ac42f146cab", "module_v06_test.go": "59c4ab8723f1cc279d75e90eb369f1861b84e0ed0cbbe4c311549f5f7aa864a6", "module_v07_test.go": "de0d6dfb40b3ab7918084510b54404f85e64b589053e90c024285952df2830a0", "main_test.go": "308777fdd66a9217cbe9c825703b9b8eb7861979619edd78c37d70785737eef7", "module_contract.go": "ec897124835057b29f6aa8dc3286d118b8980a80da7fa766f5be130010d99c7a"}
	for filename, want := range locks {
		data, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(data)
		if hex.EncodeToString(h[:]) != want {
			t.Fatalf("v0.9 module changed: %s", filename)
		}
	}
}
func TestV10FrontendIsIsolatedAndEscapesSchoolText(t *testing.T) {
	s := htmlPage
	for _, needle := range []string{"v0.10 — Dokumenty szkoły", `data-section="schoolfiles"`, "/api/school/files?", "/api/school/download?", "schoolViewActive()", "textContent=", "new AbortController()"} {
		if !strings.Contains(s, needle) {
			t.Fatal("missing", needle)
		}
	}
	if strings.Contains(s, "innerHTML=String(doc") || strings.Contains(s, "window.open(doc") {
		t.Fatal("untrusted HTML sink")
	}
}
func TestV10SchoolHTTPContentLimit(t *testing.T) {
	c, m := fakeAccount()
	target := "https://sandbox.librus.pl/GetFile/DATA123/get"
	// The body is bounded even if the server does not announce Content-Length.
	m.add("GET", target, record{200, strings.Repeat("x", 1025), "", ""})
	if _, _, err := c.schoolHTTP(context.Background(), "GET", target, nil, nil, 1024); err == nil {
		t.Fatal("oversized file accepted")
	}
}
func TestV10InvalidSchoolStatusDoesNotTriggerForeignRequests(t *testing.T) {
	c, m := fakeAccount()
	mockSchoolFile(c, m, "5", "/pliki_szkoly/pobierz/5", "https://evil.invalid/a")
	_, err := c.downloadSchoolDocumentWithPause(context.Background(), "5", schoolNoWait)
	if err == nil {
		t.Fatal("foreign redirect followed")
	}
	for _, call := range m.calls {
		if strings.Contains(call, "evil.invalid") {
			t.Fatal(call)
		}
	}
}
func TestV10PreviousAppEndpointsStillPresent(t *testing.T) {
	c, _ := fakeAccount()
	app := &LocalApp{token: "secure", origin: "http://127.0.0.1:44000"}
	app.profiles[0] = c
	for _, endpoint := range []string{"/api/data?profile=1&section=grades", "/api/homework/files?profile=1&assignment=1", "/api/diagnostics/check?profile=1&module=grades"} {
		req := httptest.NewRequest("GET", "http://127.0.0.1:44000"+endpoint, nil)
		req.AddCookie(&http.Cookie{Name: "synergia_local", Value: "secure"})
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, req)
		if w.Code != 401 {
			t.Fatal(endpoint, w.Code, w.Body.String())
		}
	}
}

// Compile-time request body type validation, not a network operation.
var _ io.Reader = (*strings.Reader)(nil)
