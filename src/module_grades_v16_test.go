package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func gradeFixtureV16(c *APIClient, m *mockedHTTP, classic string) {
	m.add("GET", dataBase+"/Grades", record{200, classic, "", ""})
	m.add("GET", dataBase+"/DescriptiveGrades", record{200, `{"Grades":[{"Id":1,"Map":"6","Grade":3,"Subject":{"Id":2},"Skill":{"Id":8},"AddDate":"2026-11-04"}]}`, "", ""})
	m.add("GET", dataBase+"/BaseTextGrades", record{200, `{"Grades":[{"Id":1,"Grade":"Pełny opis umiejętności ucznia","ShowInGradesView":true,"Subject":{"Id":2}},{"Id":2,"Grade":"Ukryta","ShowInGradesView":false}]}`, "", ""})
	m.add("GET", dataBase+"/PointGrades", record{200, `{"Grades":[{"Id":1,"Grade":"17","GradeValue":17,"Subject":{"Id":1},"Category":{"Id":4}}]}`, "", ""})
	m.add("GET", dataBase+"/Auth/TokenInfo", record{403, `{}`, "", ""})
	m.add("GET", dataBase+"/Subjects", record{200, `{"Subjects":[{"Id":1,"Name":"Matematyka"},{"Id":2,"Name":"Muzyka"}]}`, "", ""})
	m.add("GET", dataBase+"/Grades/Categories", record{200, `{"Categories":[{"Id":3,"Name":"Sprawdzian","Weight":3}]}`, "", ""})
	m.add("GET", dataBase+"/DescriptiveGrades/Skills", record{200, `{"Skills":[{"Id":8,"Name":"Śpiew"}]}`, "", ""})
	m.add("GET", dataBase+"/TextGrades/Categories", record{200, `{"Categories":[]}`, "", ""})
	m.add("GET", dataBase+"/PointGrades/Categories", record{200, `{"Categories":[{"Id":4,"Name":"Test","ValueTo":20}]}`, "", ""})
}
func TestV16AllGradeSystemsAndNoValueBasedDeduplication(t *testing.T) {
	c, m := fakeAccount()
	rows := []string{}
	for i := 1; i <= 120; i++ {
		rows = append(rows, fmt.Sprintf(`{"Id":%d,"Grade":"5","Subject":{"Id":1},"Category":{"Id":3},"AddDate":"2026-11-04"}`, i))
	}
	gradeFixtureV16(c, m, `{"Grades":[`+strings.Join(rows, ",")+`]}`)
	result, e := c.getAllGradesV16(context.Background())
	if e != nil || len(result.items) != 123 {
		t.Fatal(e, len(result.items), result.warnings)
	}
	counts := map[string]int{}
	for _, row := range result.items {
		source := str(row["source"])
		counts[source]++
		if source == "descriptive" && row["value"] != "6" {
			t.Fatal("used range instead of visible grade", row)
		}
		if source == "point" && row["maxPoints"] != float64(20) {
			t.Fatal(row)
		}
	}
	if counts["classic"] != 120 || counts["descriptive"] != 1 || counts["text"] != 1 || counts["point"] != 1 {
		t.Fatal(counts)
	}
}
func TestV16AuxiliaryFailureDoesNotRemoveOtherGradeTypes(t *testing.T) {
	c, m := fakeAccount()
	gradeFixtureV16(c, m, `{"Grades":[{"Id":1,"Grade":"5","Subject":{"Id":1}}]}`)
	m.scripted["GET "+dataBase+"/DescriptiveGrades"] = []record{{503, `{"error":"PRIVATE CONTENT"}`, "", ""}}
	result, e := c.getAllGradesV16(context.Background())
	if e != nil || len(result.items) != 3 || len(result.warnings) == 0 {
		t.Fatal(e, result)
	}
	if strings.Contains(strings.Join(result.warnings, " "), "PRIVATE") {
		t.Fatal("opaque upstream data leaked")
	}
}
func TestV16BrokenDictionariesKeepEveryGrade(t *testing.T) {
	c, m := fakeAccount()
	gradeFixtureV16(c, m, `{"Grades":[{"Id":1,"Grade":"5","Subject":{"Id":1}}]}`)
	m.scripted["GET "+dataBase+"/Subjects"] = []record{{503, `{}`, "", ""}}
	result, e := c.getAllGradesV16(context.Background())
	if e != nil || len(result.items) != 4 || result.items[0]["subject"] != "Przedmiot #1" {
		t.Fatal(e, result)
	}
}
func TestV16SameIDAcrossTypesRemainsDistinct(t *testing.T) {
	c, m := fakeAccount()
	gradeFixtureV16(c, m, `{"Grades":[{"Id":1,"Grade":"5","Subject":{"Id":1}},{"Id":1,"Grade":"5","Subject":{"Id":1}}]}`)
	result, e := c.getAllGradesV16(context.Background())
	if e != nil || len(result.items) != 4 {
		t.Fatal(e, len(result.items))
	}
}
func TestV16PartialSourceUsesCurrentStudentIdentityAndPreservesZero(t *testing.T) {
	c, m := fakeAccount()
	m.add("GET", dataBase+"/Auth/TokenInfo", record{200, `{"UserIdentifier":"LID-AUTH-USER-parent"}`, "", ""})
	m.add("GET", dataBase+"/Auth/UserInfo/LID-AUTH-USER-parent", record{200, `{"IdentifierOfStudentAssignedWithUser":"LID-AUTH-USER-child"}`, "", ""})
	m.add("POST", dataBase+"/Auth/DescriptiveGradingSystem/PartialGrades/Student/LID-AUTH-USER-child", record{200, `{"data":[{"gradeId":1,"subjectId":"LID-SUBJECT-1","scaleValue":{"value":0},"area":{"name":"Czytanie"}}],"pagination":{"page":1,"limit":20,"total":1}}`, "", ""})
	m.add("GET", dataBase+"/Auth/Subjects", record{200, `{"data":[{"identifier":"LID-SUBJECT-1","numericIdentifier":2,"name":"Język polski"}]}`, "", ""})
	rows, _, _, e := c.gradePartialV16(context.Background())
	if e != nil || len(rows) != 1 || rows[0]["value"] != "0" || rows[0]["subject"] != "Język polski" {
		t.Fatal(e, rows)
	}
}

func TestV16PartialPaginationCollectsVerifiedPages(t *testing.T) {
	c, m := fakeAccount()
	target := dataBase + "/Auth/DescriptiveGradingSystem/PartialGrades/Student/LID-AUTH-USER-child"
	m.add("POST", target, record{200, `{"data":[{"gradeId":1}],"pagination":{"page":1,"limit":1,"total":3}}`, "", ""})
	m.add("POST", target+"?limit=1&page=2", record{200, `{"data":[{"gradeId":2}],"pagination":{"page":2,"limit":1,"total":3}}`, "", ""})
	m.add("POST", target+"?limit=1&page=3", record{200, `{"data":[{"gradeId":3}],"pagination":{"page":3,"limit":1,"total":3}}`, "", ""})
	rows, warning, e := c.gradePartialPagesV16(context.Background(), target)
	if e != nil || len(rows) != 3 || warning != "" {
		t.Fatal(e, len(rows), warning)
	}
}
func TestV16PartialPaginationRejectsRepeatedOrFailedPages(t *testing.T) {
	for _, response := range []record{{200, `{"data":[{"gradeId":1}],"pagination":{"page":1,"limit":1,"total":3}}`, "", ""}, {503, `{"PRIVATE":"content"}`, "", ""}} {
		c, m := fakeAccount()
		target := dataBase + "/Auth/DescriptiveGradingSystem/PartialGrades/Student/LID-AUTH-USER-child"
		m.add("POST", target, record{200, `{"data":[{"gradeId":1}],"pagination":{"page":1,"limit":1,"total":3}}`, "", ""})
		m.add("POST", target+"?limit=1&page=2", response)
		rows, warning, e := c.gradePartialPagesV16(context.Background(), target)
		if e != nil || len(rows) != 1 || !strings.Contains(warning, "1 z 3") || strings.Contains(warning, "PRIVATE") {
			t.Fatal(e, len(rows), warning)
		}
	}
}
