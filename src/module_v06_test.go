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
	"regexp"
	"strings"
	"testing"
)

func TestV06AttendanceNamesIDsAndPresence(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", dataBase+"/Attendances", record{200, `{"Attendances":[{"Id":"t41685","Date":"2026-11-04","LessonNo":2,"Semester":1,"Type":{"Id":3},"Lesson":{"Id":9}},{"Id":41686,"Date":"2026-11-04","LessonNo":3,"Type":{"Id":100},"Lesson":{"Id":9}}]}`, "", ""})
	m.add("GET", dataBase+"/Attendances/Types", record{200, `{"Types":[{"Id":3,"Name":"Nieobecność uspr.","IsPresenceKind":false},{"Id":100,"Name":"Obecność","IsPresenceKind":true}]}`, "", ""})
	m.add("GET", dataBase+"/Lessons", record{200, `{"Lessons":[{"Id":9,"Subject":{"Id":32}}]}`, "", ""})
	m.add("GET", dataBase+"/Subjects", record{200, `{"Subjects":[{"Id":32,"Name":"Matematyka"}]}`, "", ""})
	got, err := a.getAttendanceV06(context.Background())
	if err != nil || len(got.items) != 2 || len(got.warnings) != 0 {
		t.Fatalf("attendance not parsed: %+v err=%v", got, err)
	}
	if got.items[0]["id"] != "t41685" || got.items[0]["type"] != "Nieobecność uspr." || got.items[0]["present"] != "Nie" || got.items[0]["subject"] != "Matematyka" {
		t.Fatal(got.items[0])
	}
	if got.items[1]["present"] != "Tak" {
		t.Fatal(got.items[1])
	}
	if len(m.calls) != 4 {
		t.Fatal(m.calls)
	}
}

func TestV06AttendanceOptionalLookupFailuresKeepRecords(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", dataBase+"/Attendances", record{200, `{"Attendances":[{"Id":"t101","Date":"2026-11-04","Type":{"Id":1}}]}`, "", ""})
	m.add("GET", dataBase+"/Attendances/Types", record{403, `no permission`, "", ""})
	m.add("GET", dataBase+"/Lessons", record{503, `server failure`, "", ""})
	m.add("GET", dataBase+"/Subjects", record{404, `not found`, "", ""})
	got, err := a.getAttendanceV06(context.Background())
	if err != nil || len(got.items) != 1 || len(got.warnings) != 3 {
		t.Fatalf("%+v %v", got, err)
	}
	if got.items[0]["type"] != "Typ ID: 1" {
		t.Fatal(got.items)
	}
}

func TestV06EmptyAttendanceDoesNotCallOptionalEndpoints(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", dataBase+"/Attendances", record{200, `{"Attendances":[]}`, "", ""})
	got, err := a.getAttendanceV06(context.Background())
	if err != nil || got.items == nil || len(got.items) != 0 || len(m.calls) != 1 {
		t.Fatalf("empty: %+v %v calls:%v", got, err, m.calls)
	}
}

func TestV06AgendaCategoriesAndSubjectIndependent(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", dataBase+"/HomeWorks", record{200, `{"HomeWorks":[{"Id":100,"Date":"2026-11-15","TimeFrom":"08:15","Content":"Sprawdzian <script>" ,"Category":{"Id":20},"Subject":{"Id":3}}]}`, "", ""})
	m.add("GET", dataBase+"/HomeWorks/Categories", record{200, `{"Categories":[{"Id":20,"Name":"Sprawdzian"}]}`, "", ""})
	m.add("GET", dataBase+"/Subjects", record{200, `{"Subjects":[{"Id":3,"Name":"Język polski"}]}`, "", ""})
	got, err := a.getAgendaV06(context.Background())
	if err != nil || len(got.items) != 1 || len(got.warnings) > 0 {
		t.Fatalf("%+v %v", got, err)
	}
	if got.items[0]["category"] != "Sprawdzian" || got.items[0]["subject"] != "Język polski" || got.items[0]["content"] != "Sprawdzian <script>" {
		t.Fatal(got.items)
	}
}
func TestV06AgendaWithoutCategoriesStillWorks(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", dataBase+"/HomeWorks", record{200, `{"HomeWorks":[{"Id":1,"Date":"2026-11-04","Content":"Wycieczka","Category":{"Id":20}}]}`, "", ""})
	m.add("GET", dataBase+"/HomeWorks/Categories", record{503, `maintenance`, "", ""})
	m.add("GET", dataBase+"/Subjects", record{200, `{"Subjects":[]}`, "", ""})
	got, err := a.getAgendaV06(context.Background())
	if err != nil || len(got.items) != 1 || len(got.warnings) != 1 || got.items[0]["category"] != "Kategoria ID: 20" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestV06NotesSentimentAndCategoryName(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", dataBase+"/Notes", record{200, `{"Notes":[{"Id":1,"Date":"2026-11-01","Text":"Wzorowa pomoc","Category":{"Id":8},"Teacher":{"Id":6},"Positive":1},{"Id":2,"Text":"Negatywna uwaga","Positive":0},{"Id":3,"Text":"Informacja","Positive":2},{"Id":4,"Text":"Nieznana","Positive":null}]}`, "", ""})
	m.add("GET", dataBase+"/Notes/Categories", record{200, `{"Categories":[{"Id":8,"CategoryName":"Pochwała","Name":"WRONG NAME"}]}`, "", ""})
	m.add("GET", dataBase+"/Users", record{200, `{"Users":[{"Id":6,"FirstName":"Anna","LastName":"Nowak"}]}`, "", ""})
	got, err := a.getNotesV06(context.Background())
	if err != nil || len(got.items) != 4 || len(got.warnings) > 0 {
		t.Fatalf("%+v %v", got, err)
	}
	if got.items[0]["sentiment"] != "Pozytywna" || got.items[0]["teacher"] != "Anna Nowak" || got.items[0]["category"] != "Pochwała" {
		t.Fatal(got.items[0])
	}
	for i, kind := range []string{"Pozytywna", "Negatywna", "Neutralna", "Nieokreślona"} {
		if got.items[i]["sentiment"] != kind {
			t.Fatal(got.items[i])
		}
	}
}

func TestV06NotesOptionalLookupDoesNotBlock(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", dataBase+"/Notes", record{200, `{"Notes":[{"Id":1,"Text":"Merytoryczna uwaga","Category":{"Id":2},"Teacher":{"Id":20}}]}`, "", ""})
	m.add("GET", dataBase+"/Notes/Categories", record{404, `not found`, "", ""})
	m.add("GET", dataBase+"/Users", record{503, `service unavailable`, "", ""})
	got, err := a.getNotesV06(context.Background())
	if err != nil || len(got.items) != 1 || len(got.warnings) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	if got.items[0]["teacher"] != "Nauczyciel ID: 20" {
		t.Fatal(got.items[0])
	}
}

func TestV06PrimaryModuleFailureDoesNotPoisonOtherModules(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", dataBase+"/Attendances", record{503, `maintenance`, "", ""})
	m.add("GET", dataBase+"/Notes", record{200, `{"Notes":[]}`, "", ""})
	m.add("GET", dataBase+"/HomeWorks", record{200, `{"HomeWorks":[]}`, "", ""})
	m.add("GET", dataBase+"/Grades", record{200, `{"Grades":[{"Id":1,"Grade":"5","Subject":{"Id":1}}]}`, "", ""})
	m.add("GET", dataBase+"/Subjects", record{200, `{"Subjects":[{"Id":1,"Name":"Matematyka"}]}`, "", ""})
	m.add("GET", dataBase+"/Grades/Categories", record{200, `{"Categories":[]}`, "", ""})
	if _, err := a.getAttendanceV06(context.Background()); err == nil {
		t.Fatal("attendance must fail")
	}
	if m, err := a.getNotesV06(context.Background()); err != nil || len(m.items) != 0 {
		t.Fatalf("Notes broken after attendance: %+v %v", m, err)
	}
	if m, err := a.getAgendaV06(context.Background()); err != nil || len(m.items) != 0 {
		t.Fatalf("Agenda broken after attendance: %+v %v", m, err)
	}
	if g, err := a.getGrades(context.Background()); err != nil || len(g) != 1 || g[0]["value"] != "5" {
		t.Fatalf("old grades broken after attendance: %+v %v", g, err)
	}
}

func TestV06RejectsMalformedPrimaryPayloadsWithoutBreakingOtherModules(t *testing.T) {
	for _, section := range []string{"Attendances", "HomeWorks", "Notes"} {
		a, m := fakeAccount()
		m.add("GET", dataBase+"/"+section, record{200, `{"wrongRoot":{}}`, "", ""})
		var err error
		switch section {
		case "Attendances":
			_, err = a.getAttendanceV06(context.Background())
		case "HomeWorks":
			_, err = a.getAgendaV06(context.Background())
		case "Notes":
			_, err = a.getNotesV06(context.Background())
		}
		if err == nil {
			t.Fatalf("%s missing root should not be treated as empty list", section)
		}
	}
}

func TestV06NewSectionsLocalRouteWithoutChangingOldAuth(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", dataBase+"/Notes", record{200, `{"Notes":[]}`, "", ""})
	app := &LocalApp{token: "test-token", origin: "http://127.0.0.1:12345"}
	app.profiles[0] = a
	req := httptest.NewRequest("GET", "http://127.0.0.1:12345/api/data?profile=0&section=notes", nil)
	req.AddCookie(&http.Cookie{Name: "synergia_local", Value: "test-token"})
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("notes HTTP %d %s", w.Code, w.Body.String())
	}
	var res struct {
		Items    []map[string]any `json:"items"`
		Warnings []string         `json:"warnings"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil || res.Items == nil {
		t.Fatalf("not normalized response %v %s", err, w.Body.String())
	}
	noToken := httptest.NewRecorder()
	app.routes().ServeHTTP(noToken, httptest.NewRequest("GET", "http://127.0.0.1:12345/api/data?profile=0&section=notes", nil))
	if noToken.Code != 403 {
		t.Fatalf("no token response %d", noToken.Code)
	}
}

func TestV06FeatureFilesDoNotDependOnEachOther(t *testing.T) {
	files := []string{"module_attendance.go", "module_agenda.go", "module_notes.go"}
	for _, f := range files {
		s, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, wrong := range []string{"a.getMessages(", "a.getGrades(", "a.getTimetable(", "a.getContacts(", "a.getAttendanceV06(", "a.getAgendaV06(", "a.getNotesV06("} {
			if strings.Contains(string(s), wrong) {
				t.Fatalf("module %s depends on feature: %s", f, wrong)
			}
		}
		if strings.Contains(string(s), `a.request(ctx, "POST"`) {
			t.Fatalf("module %s must remain read-only", f)
		}
	}
}

func TestV06FrozenV05FunctionsUnchanged(t *testing.T) {
	// The frozen-function check uses exact v0.5 body hashes (not names or
	// hand-written copies), so inadvertent changes fail the build.
	var original map[string]string
	bytes, err := os.ReadFile(filepath.Join("..", "BASELINE_V05_LOCK.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(bytes, &original); err != nil {
		t.Fatal(err)
	}
	code, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range original {
		p := regexp.MustCompile(`(?m)^func (?:\(a \*APIClient\) )?` + regexp.QuoteMeta(name) + `\([^\n]*`).FindIndex(code)
		if p == nil {
			t.Fatal("frozen function missing: " + name)
		}
		end := strings.Index(string(code[p[0]:]), "\n}\n")
		if end < 0 {
			t.Fatal("unterminated function: " + name)
		}
		got := fmtSHA256(code[p[0] : p[0]+end+2])
		if got != want {
			t.Fatalf("REGRESSION: v0.5 function %s changed (%s != %s)", name, got, want)
		}
	}
}

func TestV06FrontendSectionsAndSafeTextRendering(t *testing.T) {
	for _, id := range []string{"attendance", "agenda", "notes"} {
		if !strings.Contains(htmlPage, `data-section="`+id+`"`) {
			t.Fatal("missing UI section: " + id)
		}
	}
	if !strings.Contains(htmlPage, "td.textContent=String(rec[key]??'')") {
		t.Fatal("upstream HTML must be rendered as text")
	}
	if !strings.Contains(htmlPage, "body.warnings") {
		t.Fatal("partial lookup failures must be surfaced")
	}
}

func TestV06MessagesContactsAreIndependentWhenNotesFails(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", dataBase+"/Notes", record{503, "unavailable", "", ""})
	m.add("GET", msgBootstrap, record{200, "Login successfully", "", ""})
	m.add("GET", msgBase+"/inbox/messages?limit=30", record{200, `{"data":[]}`, "", ""})
	if _, err := a.getNotesV06(context.Background()); err == nil {
		t.Fatal("expected notes failure")
	}
	rows, err := a.getMessages(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatalf("messages affected: %v %+v", err, rows)
	}
}

func TestV06RequiredRootEmptyVsAbsent(t *testing.T) {
	// A normal empty list should not generate a false error; an absent root
	// must not be misreported as no school records.
	cases := []struct {
		section string
		root    string
	}{
		{"Attendances", "Attendances"}, {"Notes", "Notes"}, {"HomeWorks", "HomeWorks"},
	}
	for _, c := range cases {
		a, m := fakeAccount()
		m.add("GET", dataBase+"/"+c.section, record{200, `{"` + c.root + `":[]}`, "", ""})
		var err error
		switch c.section {
		case "Attendances":
			_, err = a.getAttendanceV06(context.Background())
		case "Notes":
			_, err = a.getNotesV06(context.Background())
		case "HomeWorks":
			_, err = a.getAgendaV06(context.Background())
		}
		if err != nil {
			t.Fatal(c.section, err)
		}
	}
}

// fmtSHA256 is in the dedicated frozen version lock, not production code.
func fmtSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
