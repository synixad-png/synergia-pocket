package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestV11NoticesStringIDsAndReadOnly(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/SchoolNotices", record{200, `{"SchoolNotices":[{"Id":"LID-NBOARD-NOTICE-111","Subject":"Wywiadówka","Content":"<img src=x onerror=alert(1)> Pełna treść","CreationDate":"2026-11-03","StartDate":"2026-11-04","EndDate":"2026-11-30","WasRead":false},{"Id":"LID-NBOARD-NOTICE-112","Subject":"Zakończone","WasRead":true},{"Id":"LID-NBOARD-NOTICE-113"}]}`, "", ""})
	rows, err := c.getNoticesV11(context.Background())
	if err != nil || len(rows.items) != 3 || len(rows.warnings) != 0 {
		t.Fatalf("rows:%+v err:%v", rows, err)
	}
	if rows.items[0]["id"] != "LID-NBOARD-NOTICE-111" || rows.items[0]["read"] != "Nieprzeczytane" || rows.items[1]["read"] != "Przeczytane" || rows.items[2]["read"] != "Nieznany" {
		t.Fatal(rows.items)
	}
	if rows.items[0]["content"] != "<img src=x onerror=alert(1)> Pełna treść" {
		t.Fatal(rows.items[0])
	}
	if len(m.calls) != 1 || strings.Contains(strings.Join(m.calls, ","), "POST") {
		t.Fatal(m.calls)
	}
}
func TestV11NoticesEmptyNoExtraCalls(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/SchoolNotices", record{200, `{"SchoolNotices":[]}`, "", ""})
	rows, err := c.getNoticesV11(context.Background())
	if err != nil || rows.items == nil || len(rows.items) != 0 || len(m.calls) != 1 {
		t.Fatalf("%+v %v %v", rows, err, m.calls)
	}
}
func TestV11NoticesMalformedAndUnauthorizedNotEmpty(t *testing.T) {
	for _, tc := range []struct {
		code int
		body string
	}{{200, `{"SchoolNotices":{}}`}, {200, `{"SchoolNotices":[{"Id":null}]}`}, {403, `denied`}} {
		c, m := fakeAccount()
		m.add("GET", dataBase+"/SchoolNotices", record{tc.code, tc.body, "", ""})
		if _, err := c.getNoticesV11(context.Background()); err == nil {
			t.Fatalf("invalid response accepted %+v", tc)
		}
	}
}
func TestV11FreeDaysSelectsOnlyStudentClassAndPartialLessons(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/SchoolFreeDays", record{200, `{"SchoolFreeDays":[{"Id":11,"Name":"Boże Narodzenie","DateFrom":"2026-12-24","DateTo":"2026-12-26"}]}`, "", ""})
	m.add("GET", dataBase+"/Classes", record{200, `{"Class":{"Id":501,"Number":5,"Symbol":"a"}}`, "", ""})
	m.add("GET", dataBase+"/ClassFreeDays", record{200, `{"ClassFreeDays":[{"Id":20,"Class":{"Id":999},"Type":{"Id":2},"DateFrom":"2026-11-08","DateTo":"2026-11-08"},{"Id":21,"Class":{"Id":501},"Type":{"Id":1},"DateFrom":"2026-11-12","DateTo":"2026-11-12","LessonNoFrom":2,"LessonNoTo":4},{"Id":22,"Class":{"Id":501},"Type":{"Id":2},"DateFrom":"2026-11-14","DateTo":"2026-11-14"},{"Id":23,"Type":{"Id":3},"DateFrom":"2026-11-15","DateTo":"2026-11-15"}]}`, "", ""})
	m.add("GET", dataBase+"/Calendars/ClassFreeDays/Types", record{200, `{"Types":[{"Id":1,"Name":"Rekolekcje"},{"Id":2,"Name":"Wycieczka"}]}`, "", ""})
	rows, err := c.getFreeDaysV11(context.Background())
	if err != nil || len(rows.warnings) != 0 || len(rows.items) != 3 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	if rows.items[0]["id"] != "21" || rows.items[0]["name"] != "Rekolekcje" || rows.items[0]["lessons"] != "Lekcje 2–4" || rows.items[0]["wholeDay"] != false {
		t.Fatal(rows.items[0])
	}
	if rows.items[1]["id"] != "22" || rows.items[1]["name"] != "Wycieczka" || rows.items[1]["wholeDay"] != true {
		t.Fatal(rows.items[1])
	}
	if rows.items[2]["scope"] != "Cała szkoła" {
		t.Fatal(rows.items[2])
	}
	for _, r := range rows.items {
		if r["id"] == "20" || r["id"] == "23" {
			t.Fatal("other class leaked", r)
		}
	}
	if len(m.calls) != 4 {
		t.Fatal(m.calls)
	}
}
func TestV11FreeDaysNoClassIDNeverShowsOtherClass(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/SchoolFreeDays", record{200, `{"SchoolFreeDays":[{"Id":5,"Name":"Wakacje","DateFrom":"2027-06-25","DateTo":"2027-08-31"}]}`, "", ""})
	m.add("GET", dataBase+"/Classes", record{403, `no access`, "", ""})
	rows, err := c.getFreeDaysV11(context.Background())
	if err != nil || len(rows.items) != 1 || len(rows.warnings) != 1 || len(m.calls) != 2 {
		t.Fatalf("%+v %v %v", rows, err, m.calls)
	}
	for _, call := range m.calls {
		if strings.Contains(call, "ClassFreeDays") {
			t.Fatal("class days fetched without class id", m.calls)
		}
	}
}
func TestV11FreeDaysClassEndpointFailsSchoolRecordsSurvive(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/SchoolFreeDays", record{200, `{"SchoolFreeDays":[{"Id":5,"Name":"Maj","DateFrom":"2027-05-01","DateTo":"2027-05-03"}]}`, "", ""})
	m.add("GET", dataBase+"/Classes", record{200, `{"Class":{"Id":1}}`, "", ""})
	m.add("GET", dataBase+"/ClassFreeDays", record{503, `unavailable`, "", ""})
	rows, err := c.getFreeDaysV11(context.Background())
	if err != nil || len(rows.items) != 1 || len(rows.warnings) != 1 {
		t.Fatalf("%+v %v", rows, err)
	}
}
func TestV11FreeDaysSchoolEndpointFailsClassStillWorks(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/SchoolFreeDays", record{503, `down`, "", ""})
	m.add("GET", dataBase+"/Classes", record{200, `{"Class":{"Id":"301"}}`, "", ""})
	m.add("GET", dataBase+"/ClassFreeDays", record{200, `{"ClassFreeDays":[{"Id":88,"Class":{"Id":301},"DateFrom":"2026-11-20","DateTo":"2026-11-20"}]}`, "", ""})
	rows, err := c.getFreeDaysV11(context.Background())
	if err != nil || len(rows.items) != 1 || len(rows.warnings) != 1 || rows.items[0]["name"] != "Dzień wolny (brak nazwy)" {
		t.Fatalf("%+v %v", rows, err)
	}
}
func TestV11FreeDaysBothEndpointsUnavailableReportsError(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/SchoolFreeDays", record{503, `down`, "", ""})
	m.add("GET", dataBase+"/Classes", record{403, `no class`, "", ""})
	if _, err := c.getFreeDaysV11(context.Background()); err == nil {
		t.Fatal("two failures must not become empty list")
	}
}
func TestV11FreeDaysNoClassEntriesDoesNotAskForTypes(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/SchoolFreeDays", record{200, `{"SchoolFreeDays":[]}`, "", ""})
	m.add("GET", dataBase+"/Classes", record{200, `{"Class":{"Id":1}}`, "", ""})
	m.add("GET", dataBase+"/ClassFreeDays", record{200, `{"ClassFreeDays":[{"Id":20,"Class":{"Id":2},"Type":{"Id":7},"DateFrom":"2026-11-04","DateTo":"2026-11-04"}]}`, "", ""})
	rows, err := c.getFreeDaysV11(context.Background())
	if err != nil || len(rows.items) != 0 || len(m.calls) != 3 {
		t.Fatalf("%+v %v %v", rows, err, m.calls)
	}
}
func TestV11FreeDaysOptionalTypesFailureKeepsDays(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/SchoolFreeDays", record{200, `{"SchoolFreeDays":[]}`, "", ""})
	m.add("GET", dataBase+"/Classes", record{200, `{"Class":{"Id":1}}`, "", ""})
	m.add("GET", dataBase+"/ClassFreeDays", record{200, `{"ClassFreeDays":[{"Id":1,"Class":{"Id":1},"Type":{"Id":7},"DateFrom":"2026-11-04","DateTo":"2026-11-04"}]}`, "", ""})
	m.add("GET", dataBase+"/Calendars/ClassFreeDays/Types", record{403, `denied`, "", ""})
	rows, err := c.getFreeDaysV11(context.Background())
	if err != nil || len(rows.items) != 1 || len(rows.warnings) != 1 {
		t.Fatalf("%+v %v", rows, err)
	}
}
func TestV11NewRoutesRequireLocalCookie(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/SchoolNotices", record{200, `{"SchoolNotices":[]}`, "", ""})
	app := &LocalApp{token: "t", origin: "http://127.0.0.1:3300"}
	app.profiles[0] = c
	for _, section := range []string{"notices", "freedays"} {
		path := "http://127.0.0.1:3300/api/data?profile=0&section=" + section
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, req)
		if w.Code != 403 {
			t.Fatalf("%s unauthorized code %d", section, w.Code)
		}
		req = httptest.NewRequest("GET", path, nil)
		req.AddCookie(&http.Cookie{Name: "synergia_local", Value: "t"})
		w = httptest.NewRecorder()
		app.routes().ServeHTTP(w, req)
		// Freedays will fail with fixtures lacking its endpoints. Auth must have succeeded.
		if section == "notices" && w.Code != 200 || section == "freedays" && w.Code != 502 {
			t.Fatalf("section %s code %d body %s", section, w.Code, w.Body)
		}
	}
}
func TestV11NewModulesDoNotAffectOldGradesAfterFailure(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/SchoolNotices", record{503, `down`, "", ""})
	if _, err := c.getNoticesV11(context.Background()); err == nil {
		t.Fatal("notices should fail")
	}
	m.add("GET", dataBase+"/Grades", record{200, `{"Grades":[{"Id":1,"Grade":"5","Subject":{"Id":1}}]}`, "", ""})
	m.add("GET", dataBase+"/Subjects", record{200, `{"Subjects":[{"Id":1,"Name":"Matematyka"}]}`, "", ""})
	m.add("GET", dataBase+"/Grades/Categories", record{200, `{"Categories":[]}`, "", ""})
	grades, err := c.getGrades(context.Background())
	if err != nil || len(grades) != 1 {
		t.Fatalf("grades after notices failed %+v %v", grades, err)
	}
}
func TestV11FeatureFileIsolationAndNoWrites(t *testing.T) {
	for _, filename := range []string{"module_notices.go", "module_freedays.go"} {
		content, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		source := string(content)
		for _, bad := range []string{"getMessages(", "getGrades(", "getTimetable(", "getContacts(", "getHomeworkV07(", "getAgendaV06(", "a.getNoticesV11(", "a.getFreeDaysV11(", ".request(ctx, \"POST\"", "MarkAsRead/"} {
			if strings.Contains(source, bad) {
				t.Fatalf("module %s calls another module/writes: %s", filename, bad)
			}
		}
	}
}
func TestV11OriginalSourcesExceptEntrypointsUntouched(t *testing.T) {
	folder := "../BASELINE_V11_PRIOR_SOURCES.json"
	raw, err := os.ReadFile(folder)
	if err != nil {
		t.Fatal(err)
	}
	var checks map[string]string
	if err = json.Unmarshal(raw, &checks); err != nil {
		t.Fatal(err)
	}
	for name, want := range checks {
		raw, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		got := sha256.Sum256(raw)
		if hex.EncodeToString(got[:]) != want {
			t.Fatalf("REGRESSION in frozen %s", name)
		}
	}
}
func TestV11FrontendRendersTextNotHTML(t *testing.T) {
	for _, id := range []string{`data-section="notices"`, `data-section="freedays"`, `'notices'`, `'freedays'`} {
		if !strings.Contains(htmlPage, id) {
			t.Fatalf("missing frontend %s", id)
		}
	}
	if !strings.Contains(htmlPage, "td.textContent=String(rec[key]??'')") || !strings.Contains(htmlPage, "notice-text") {
		t.Fatal("notice content must be inserted only as plain text")
	}
}
