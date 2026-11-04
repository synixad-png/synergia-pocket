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

func TestV07HomeworkSeparateGatewayAndCorrectLookupFields(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", dataBase+"/HomeWorkAssignments", record{200, `{"HomeWorkAssignments":[{"Id":102,"Date":"2026-11-04","DueDate":"2026-11-08","Topic":"Matematyka – zadanie","Text":"Przykład <script>alert(1)</script>","Teacher":{"Id":17},"Category":{"Id":45},"Lesson":{"Id":300},"MustSendAttachFile":true,"HomeworkAssigmentFiles":[{"Id":200,"Name":"materialy.pdf","Url":"HomeWorkAssignments/Attachment/102-200"}]}]}`, "", ""})
	m.add("GET", dataBase+"/Users", record{200, `{"Users":[{"Id":17,"FirstName":"Anna","LastName":"Nowak"}]}`, "", ""})
	m.add("GET", dataBase+"/HomeWorkAssignments/Categories", record{200, `{"Categories":[{"Id":45,"Name":"Zła nazwa","CategoryName":"Praca pisemna"}]}`, "", ""})
	res, err := a.getHomeworkV07(context.Background())
	if err != nil || len(res.items) != 1 || len(res.warnings) != 0 {
		t.Fatalf("result: %+v err: %v", res, err)
	}
	row := res.items[0]
	if row["topic"] != "Matematyka – zadanie" || row["content"] != "Przykład <script>alert(1)</script>" || row["teacher"] != "Anna Nowak" || row["category"] != "Praca pisemna" || row["attachmentNames"] != "materialy.pdf" || row["requirement"] != "Wymagany plik" || row["dueDate"] != "2026-11-08" {
		t.Fatalf("incorrect fields: %+v", row)
	}
	if len(m.calls) != 3 || strings.Contains(strings.Join(m.calls, "\n"), "/HomeWorks?") {
		t.Fatalf("accessed other feature: %v", m.calls)
	}
	if strings.Contains(strings.Join(m.calls, "\n"), "/Attachment/") {
		t.Fatal("must not auto-download attachments")
	}
}

func TestV07HomeworkWithoutOptionalReferencesDoesNotCallOthers(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", dataBase+"/HomeWorkAssignments", record{200, `{"HomeWorkAssignments":[{"Id":3,"Topic":"Przeczytaj","Text":"Rozdział 7","DueDate":"2026-11-10"}]}`, "", ""})
	rows, err := a.getHomeworkV07(context.Background())
	if err != nil || len(rows.items) != 1 || len(m.calls) != 1 || len(rows.warnings) != 0 {
		t.Fatalf("got: %+v err: %v calls: %v", rows, err, m.calls)
	}
	if rows.items[0]["attachmentNames"] != "—" {
		t.Fatal(rows.items[0])
	}
}

func TestV07HomeworkEmptyListSkipsLookups(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", dataBase+"/HomeWorkAssignments", record{200, `{"HomeWorkAssignments":[]}`, "", ""})
	got, err := a.getHomeworkV07(context.Background())
	if err != nil || got.items == nil || len(got.items) != 0 || len(m.calls) != 1 {
		t.Fatalf("got %+v err %v calls %v", got, err, m.calls)
	}
}

func TestV07HomeworkOptionalLookupsCanFailAndRecordsRemain(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", dataBase+"/HomeWorkAssignments", record{200, `{"HomeWorkAssignments":[{"Id":"H-4","Teacher":{"Id":19},"Category":{"Id":11},"Topic":"Karta pracy","HomeworkAssigmentFiles":[]}]}`, "", ""})
	m.add("GET", dataBase+"/Users", record{503, `unavailable`, "", ""})
	m.add("GET", dataBase+"/HomeWorkAssignments/Categories", record{403, `forbidden`, "", ""})
	got, err := a.getHomeworkV07(context.Background())
	if err != nil || len(got.items) != 1 || len(got.warnings) != 2 {
		t.Fatalf("got %+v err %v", got, err)
	}
	if got.items[0]["teacher"] != "Nauczyciel ID: 19" || got.items[0]["category"] != "Kategoria ID: 11" {
		t.Fatal(got.items[0])
	}
}

func TestV07HomeworkMalformedOptionalLookupStillReturnsData(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", dataBase+"/HomeWorkAssignments", record{200, `{"HomeWorkAssignments":[{"Id":2,"Teacher":{"Id":4},"Category":{"Id":8}}]}`, "", ""})
	m.add("GET", dataBase+"/Users", record{200, `{"UnexpectedRoot":[]}`, "", ""})
	m.add("GET", dataBase+"/HomeWorkAssignments/Categories", record{200, `{"UnexpectedRoot":[]}`, "", ""})
	got, err := a.getHomeworkV07(context.Background())
	if err != nil || len(got.items) != 1 || len(got.warnings) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestV07HomeworkMalformedPrimaryDoesNotBecomeEmptySuccess(t *testing.T) {
	for _, body := range []string{`{"HomeWorks":[]}`, `{"HomeWorkAssignments":null}`, `{"HomeWorkAssignments":{}}`} {
		a, m := fakeAccount()
		m.add("GET", dataBase+"/HomeWorkAssignments", record{200, body, "", ""})
		_, err := a.getHomeworkV07(context.Background())
		if err == nil {
			t.Fatalf("expected error on %s", body)
		}
	}
}

func TestV07HomeworkAttachmentNamesNoNetworkOrLinks(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", dataBase+"/HomeWorkAssignments", record{200, `{"HomeWorkAssignments":[{"Id":1,"Topic":"Test","HomeworkAssigmentFiles":[{"Name":"A.txt"},{"Id":33},{"Name":"X<script>"}]},{"Id":2,"HomeworkAssigmentFiles":"bad shape"}]}`, "", ""})
	got, err := a.getHomeworkV07(context.Background())
	if err != nil || len(got.items) != 2 || len(got.warnings) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	if got.items[0]["attachmentNames"] != "A.txt, Plik ID: 33, X<script>" {
		t.Fatal(got.items[0])
	}
	if len(m.calls) != 1 {
		t.Fatal("download attempted or other module touched", m.calls)
	}
}

func TestV07HomeworkPrimaryFailureDoesNotBreakExistingAgendaGradesAndMessages(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", dataBase+"/HomeWorkAssignments", record{503, `failure`, "", ""})
	m.add("GET", dataBase+"/HomeWorks", record{200, `{"HomeWorks":[]}`, "", ""})
	m.add("GET", dataBase+"/Grades", record{200, `{"Grades":[{"Id":2,"Grade":"6","Subject":{"Id":3}}]}`, "", ""})
	m.add("GET", dataBase+"/Subjects", record{200, `{"Subjects":[{"Id":3,"Name":"Fizyka"}]}`, "", ""})
	m.add("GET", dataBase+"/Grades/Categories", record{200, `{"Categories":[]}`, "", ""})
	m.add("GET", msgBootstrap, record{200, `Wiadomosci`, "", ""})
	m.add("GET", msgBase+"/inbox/messages?limit=30", record{200, `{"data":[]}`, "", ""})
	if _, err := a.getHomeworkV07(context.Background()); err == nil {
		t.Fatal("homework error expected")
	}
	if agenda, err := a.getAgendaV06(context.Background()); err != nil || len(agenda.items) != 0 {
		t.Fatalf("agenda broken: %+v %v", agenda, err)
	}
	if grades, err := a.getGrades(context.Background()); err != nil || len(grades) != 1 {
		t.Fatalf("grades broken: %+v %v", grades, err)
	}
	if messages, err := a.getMessages(context.Background()); err != nil || len(messages) != 0 {
		t.Fatalf("messages broken: %+v %v", messages, err)
	}
}

func TestV07HomeworkLocalRouteAndSessionIsolation(t *testing.T) {
	a, m := fakeAccount()
	m.add("GET", dataBase+"/HomeWorkAssignments", record{200, `{"HomeWorkAssignments":[{"Id":22,"Topic":"Czytaj"}]}`, "", ""})
	app := &LocalApp{token: "secure-test", origin: "http://127.0.0.1:32100"}
	app.profiles[0] = a
	req := httptest.NewRequest("GET", "http://127.0.0.1:32100/api/data?profile=0&section=homework", nil)
	req.AddCookie(&http.Cookie{Name: "synergia_local", Value: "secure-test"})
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, req)
	var value struct {
		Items []map[string]any `json:"items"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &value) != nil || len(value.Items) != 1 {
		t.Fatalf("route response %d %s", w.Code, w.Body.String())
	}
	anonymous := httptest.NewRecorder()
	app.routes().ServeHTTP(anonymous, httptest.NewRequest("GET", "http://127.0.0.1:32100/api/data?profile=0&section=homework", nil))
	if anonymous.Code != 403 {
		t.Fatal("anonymous local access allowed", anonymous.Code)
	}
	second := httptest.NewRequest("GET", "http://127.0.0.1:32100/api/data?profile=1&section=homework", nil)
	second.AddCookie(&http.Cookie{Name: "synergia_local", Value: "secure-test"})
	w2 := httptest.NewRecorder()
	app.routes().ServeHTTP(w2, second)
	if w2.Code != 401 {
		t.Fatal("second account must not reuse first account credentials", w2.Code)
	}
}

func TestV07ModuleIndependenceAgainstAllFeatureCalls(t *testing.T) {
	forbidden := []string{
		"a.getMessages(", "a.getGrades(", "a.getTimetable(", "a.getContacts(",
		"a.getAttendanceV06(", "a.getAgendaV06(", "a.getNotesV06(",
		"a.getHomeworkV07(",
		"a.request(ctx, \"POST\"", "a.messageGet(",
	}
	for _, name := range []string{"module_attendance.go", "module_agenda.go", "module_notes.go", "module_homework.go"} {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, needle := range forbidden {
			if strings.Contains(string(src), needle) {
				// The homework method's declaration doesn't match call syntax.
				t.Fatalf("module %s invokes feature %s", name, needle)
			}
		}
	}
	main, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(main), `case "homework":`) {
		t.Fatal("homework is not routed")
	}
	page, err := os.ReadFile("frontend.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{`data-section="homework"`, `HomeWorkAssignments`, `attachmentNames`} {
		if !strings.Contains(string(page), token) && token != `HomeWorkAssignments` {
			t.Fatalf("missing UI: %s", token)
		}
	}
}

func TestV07FrozenV06ModulesFromShippedLock(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "BASELINE_V06_MODULES_LOCK.json"))
	if err != nil {
		t.Fatal(err)
	}
	var checksums map[string]string
	if err := json.Unmarshal(raw, &checksums); err != nil {
		t.Fatal(err)
	}
	if len(checksums) != 3 {
		t.Fatal("expected three frozen v0.6 feature modules")
	}
	for file, want := range checksums {
		contents, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(contents)
		if hex.EncodeToString(hash[:]) != want {
			t.Fatalf("REGRESSION: old v0.6 module modified: %s", file)
		}
	}
}
