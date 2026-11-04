package main

// The grade screen has several independent upstream sources. The frozen
// classic getGrades remains available; this collector does not depend on any
// other feature module or silently collapse failures into an empty collection.
import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type gradeSourceV16 struct {
	ID    string `json:"source"`
	Label string `json:"label"`
	State string `json:"state"`
	Count int    `json:"count"`
}
type gradesResultV16 struct {
	items    []map[string]any
	warnings []string
	sources  []gradeSourceV16
}

var gradeDefinitionsV16 = []struct{ id, label, path string }{
	{"classic", "Klasyczne", "Grades"}, {"descriptive", "Opisowe", "DescriptiveGrades"}, {"text", "Tekstowe", "BaseTextGrades"}, {"point", "Punktowe", "PointGrades"},
}

func gradeUnavailableV16(err error) bool {
	var p ProtocolError
	if !errors.As(err, &p) {
		return false
	}
	s := p.Error()
	return strings.Contains(s, "HTTP 403") || strings.Contains(s, "HTTP 404") || strings.Contains(s, "HTTP 405")
}
func (c *APIClient) gradeReadV16(parent context.Context, path string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(parent, 7*time.Second)
	defer cancel()
	return c.data(ctx, path, nil)
}
func gradeListV16(data map[string]any, key string) ([]map[string]any, error) {
	raw, ok := data[key].([]any)
	if !ok {
		return nil, ProtocolError{"Oceny: nieznany format listy " + key + "."}
	}
	out := []map[string]any{}
	for _, v := range raw {
		row, ok := v.(map[string]any)
		if !ok || row["Id"] == nil {
			return nil, ProtocolError{"Oceny: nieznany format wpisu."}
		}
		out = append(out, row)
	}
	return out, nil
}
func gradeDictionaryV16(data map[string]any, key string) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, v := range arr(data[key]) {
		m := obj(v)
		if id := str(m["Id"]); id != "" {
			out[id] = m
		}
	}
	return out
}
func gradeValueV16(m map[string]any, source string) string {
	if source == "descriptive" {
		if value := str(m["Map"]); value != "" {
			return value
		}
		return str(m["RealGradeValue"])
	}
	return str(m["Grade"])
}
func gradeCommentsV16(value any, dictionary map[string]map[string]any) []string {
	out := []string{}
	for _, v := range arr(value) {
		id := ref(v)
		if id == "" {
			id = str(v)
		}
		text := str(dictionary[id]["Text"])
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}
func (c *APIClient) getAllGradesV16(parent context.Context) (gradesResultV16, error) {
	ctx, cancel := context.WithTimeout(parent, 48*time.Second)
	defer cancel()
	result := gradesResultV16{items: []map[string]any{}, warnings: []string{}, sources: []gradeSourceV16{}}
	// Fetch primary lists before optional lookups, so an unavailable dictionary
	// cannot consume the time budget or suppress grades from another system.
	lists := map[string][]map[string]any{}
	successes := 0
	for _, def := range gradeDefinitionsV16 {
		data, err := c.gradeReadV16(ctx, def.path)
		var rows []map[string]any
		if err == nil {
			rows, err = gradeListV16(data, "Grades")
		}
		state := "ok"
		if err != nil {
			state = "failed"
			if gradeUnavailableV16(err) {
				state = "unavailable"
			} else {
				result.warnings = append(result.warnings, def.label+": nie udało się pobrać tej części ocen.")
			}
		} else {
			successes++
			lists[def.id] = rows
			if len(rows) == 0 {
				state = "empty"
			}
		}
		result.sources = append(result.sources, gradeSourceV16{ID: def.id, Label: def.label, State: state})
	}
	partial, partialSubjects, partialErr := c.gradePartialV16(ctx)
	partialState := "ok"
	if partialErr != nil {
		partialState = "failed"
		if gradeUnavailableV16(partialErr) {
			partialState = "unavailable"
		} else {
			result.warnings = append(result.warnings, "Nowe opisowe: nie udało się potwierdzić odczytu tego źródła.")
		}
	} else {
		successes++
		if len(partial) == 0 {
			partialState = "empty"
		}
	}
	result.sources = append(result.sources, gradeSourceV16{ID: "partial", Label: "Nowe opisowe", State: partialState, Count: len(partial)})
	if successes == 0 {
		return result, ProtocolError{"Oceny: nie udało się odczytać żadnego źródła ocen."}
	}
	subjects := map[string]string{}
	if data, err := c.gradeReadV16(ctx, "Subjects"); err == nil {
		subjects = names(data, "Subjects", false)
	} else {
		result.warnings = append(result.warnings, "Nie udało się odczytać nazw przedmiotów. Oceny zachowano pod identyfikatorami.")
	}
	for id, name := range partialSubjects {
		subjects[id] = name
	}
	categories := map[string]map[string]map[string]any{}
	for _, source := range []string{"classic", "descriptive", "text", "point"} {
		if len(lists[source]) == 0 {
			continue
		}
		path, key := "Grades/Categories", "Categories"
		switch source {
		case "descriptive":
			path, key = "DescriptiveGrades/Skills", "Skills"
		case "text":
			path = "TextGrades/Categories"
		case "point":
			path = "PointGrades/Categories"
		}
		if data, e := c.gradeReadV16(ctx, path); e == nil {
			categories[source] = gradeDictionaryV16(data, key)
		}
	}
	seen := map[string]bool{}
	for i, def := range gradeDefinitionsV16 {
		for _, m := range lists[def.id] {
			if def.id == "text" && m["ShowInGradesView"] == false {
				continue
			}
			id := str(m["Id"])
			identity := def.id + ":" + id
			if seen[identity] {
				continue
			}
			seen[identity] = true
			subjectID := ref(m["Subject"])
			subject := subjects[subjectID]
			if subject == "" {
				subject = "Przedmiot #" + subjectID
				if subjectID == "" {
					subject = "Przedmiot nieokreślony"
				}
			}
			categoryID := ref(m["Category"])
			if def.id == "descriptive" {
				categoryID = ref(m["Skill"])
			}
			category := categories[def.id][categoryID]
			categoryName := str(category["Name"])
			if categoryName == "" {
				categoryName = str(category["CategoryName"])
			}
			if categoryName == "" {
				categoryName = def.label
			}
			kind := "cząstkowa"
			if m["IsFinal"] == true {
				kind = "roczna"
			} else if m["IsSemester"] == true {
				kind = "semestralna"
			} else if m["IsSemesterProposition"] == true || m["IsFinalProposition"] == true {
				kind = "proponowana"
			}
			date := str(m["AddDate"])
			if date == "" {
				date = str(m["Date"])
			}
			row := map[string]any{"id": id, "source": def.id, "sourceLabel": def.label, "value": gradeValueV16(m, def.id), "date": date, "subject": subject, "subjectId": subjectID, "category": categoryName, "semester": m["Semester"], "kind": kind, "weight": category["Weight"]}
			if def.id == "point" {
				row["points"], row["maxPoints"] = m["GradeValue"], category["ValueTo"]
			}
			result.items = append(result.items, row)
			result.sources[i].Count++
		}
	}
	for _, row := range partial {
		sid := str(row["subjectId"])
		if name := subjects[sid]; name != "" {
			row["subject"] = name
		}
		result.items = append(result.items, row)
	}
	if len(partial) > 0 {
		result.warnings = append(result.warnings, "Nowe oceny opisowe: odebrano pierwszą listę; kompletność tego źródła wymaga sprawdzenia.")
	}
	for i := range result.sources {
		if result.sources[i].State == "ok" && result.sources[i].Count == 0 {
			result.sources[i].State = "empty"
		}
	}
	return result, nil
}

var gradeLIDV16 = regexp.MustCompile(`^[A-Za-z0-9_-]{1,160}$`)

func (c *APIClient) gradePartialV16(ctx context.Context) ([]map[string]any, map[string]string, error) {
	empty := []map[string]any{}
	subjectNames := map[string]string{}
	token, err := c.gradeReadV16(ctx, "Auth/TokenInfo")
	if err != nil {
		return empty, subjectNames, err
	}
	user := ""
	for _, key := range []string{"UserIdentifier", "userIdentifier", "Identifier", "identifier"} {
		if value := str(token[key]); strings.HasPrefix(value, "LID-AUTH-USER-") {
			user = value
			break
		}
	}
	if !gradeLIDV16.MatchString(user) {
		return empty, subjectNames, ProtocolError{"Oceny: nieznany format identyfikatora konta."}
	}
	info, err := c.gradeReadV16(ctx, "Auth/UserInfo/"+user)
	if err != nil {
		return empty, subjectNames, err
	}
	if nested, ok := info["UserInfo"].(map[string]any); ok {
		info = nested
	}
	student := str(info["IdentifierOfStudentAssignedWithUser"])
	if student == "" {
		return empty, subjectNames, ProtocolError{"Oceny: nie udostępniono identyfikatora ucznia."}
	}
	if !gradeLIDV16.MatchString(student) {
		return empty, subjectNames, ProtocolError{"Oceny: nieznany format identyfikatora ucznia."}
	}
	child, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	if err = c.ensure(child); err != nil {
		return empty, subjectNames, err
	}
	target := dataBase + "/Auth/DescriptiveGradingSystem/PartialGrades/Student/" + student
	response, body, err := c.request(child, http.MethodPost, target, strings.NewReader("{}"), map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return empty, subjectNames, err
	}
	if response.StatusCode == 401 && c.refreshSession(child) {
		response, body, err = c.request(child, http.MethodPost, target, strings.NewReader("{}"), map[string]string{"Content-Type": "application/json"})
		if err != nil {
			return empty, subjectNames, err
		}
	}
	if response.StatusCode >= 400 {
		return empty, subjectNames, ProtocolError{"Oceny: HTTP " + strconv.Itoa(response.StatusCode) + "."}
	}
	payload, err := parseJSON(body, "Nowe oceny opisowe")
	if err != nil {
		return empty, subjectNames, err
	}
	raw, ok := payload["data"].([]any)
	if !ok {
		return empty, subjectNames, ProtocolError{"Oceny: nieznany format nowego systemu."}
	}
	if len(raw) == 0 {
		return empty, subjectNames, nil
	}
	authSubjects, _ := c.gradeReadV16(ctx, "Auth/Subjects")
	numeric := map[string]string{}
	for _, v := range arr(authSubjects["data"]) {
		m := obj(v)
		lid, id := str(m["identifier"]), str(m["numericIdentifier"])
		numeric[lid] = id
		if name := str(m["name"]); name != "" {
			subjectNames[id] = name
			subjectNames[lid] = name
		}
	}
	seen := map[string]bool{}
	for _, v := range raw {
		m, ok := v.(map[string]any)
		if !ok || m["gradeId"] == nil {
			return empty, subjectNames, ProtocolError{"Oceny: nieznany wpis nowego systemu."}
		}
		id := str(m["gradeId"])
		if seen[id] {
			continue
		}
		seen[id] = true
		sid := str(m["subjectId"])
		if id := numeric[sid]; id != "" {
			sid = id
		}
		value := str(obj(m["scaleValue"])["value"])
		if _, boolValue := obj(m["scaleValue"])["value"].(bool); boolValue {
			value = ""
		}
		subject := subjectNames[sid]
		if subject == "" {
			subject = "Przedmiot #" + sid
		}
		date := str(m["addDate"])
		if date == "" {
			date = str(m["date"])
		}
		comments := []string{}
		if content := str(m["content"]); content != "" {
			comments = append(comments, content)
		}
		empty = append(empty, map[string]any{"id": id, "source": "partial", "sourceLabel": "Nowe opisowe", "value": value, "subjectId": sid, "subject": subject, "category": obj(m["area"])["name"], "date": date, "semester": m["semester"], "comments": comments})
	}
	return empty, subjectNames, nil
}
