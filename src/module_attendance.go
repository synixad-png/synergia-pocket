package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// getAttendanceV06 is independent of grades, timetable, notes and messages.
// Core: Attendances. Optional lookups: Attendances/Types, Lessons, Subjects.
// Raw unusual IDs (including a leading 't') are deliberately not changed.
func (a *APIClient) getAttendanceV06(ctx context.Context) (moduleRows, error) {
	// Hard cap: a failed optional new module cannot monopolize the account
	// session lock and block the previously working features indefinitely.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	primary, err := a.data(ctx, "Attendances", nil)
	if err != nil {
		return moduleRows{}, err
	}
	raw, ok := primary["Attendances"].([]any)
	if !ok {
		return moduleRows{}, ProtocolError{"Frekwencja: brak oczekiwanej listy Attendances."}
	}
	result := moduleRows{items: []map[string]any{}}
	if len(raw) == 0 {
		return result, nil
	}

	// Lookup failures must not hide real attendance entries.
	types := map[string]map[string]any{}
	if response, e := a.data(ctx, "Attendances/Types", nil); e == nil {
		if list, valid := response["Types"].([]any); valid {
			for _, item := range list {
				r := obj(item)
				if id := str(r["Id"]); id != "" {
					types[id] = r
				}
			}
		} else {
			result.warnings = append(result.warnings, "Nieznany format nazw typów frekwencji; pozostawiono identyfikatory.")
		}
	} else {
		result.warnings = append(result.warnings, "Nie udało się pobrać nazw typów frekwencji; wpisy pozostają dostępne.")
	}

	// Both lookups are deliberately optional: one failed lookup never prevents
	// the other one, nor prevents showing the primary attendance records.
	lessonSubjects := map[string]string{}
	if response, e := a.data(ctx, "Lessons", nil); e == nil {
		if list, valid := response["Lessons"].([]any); valid {
			for _, item := range list {
				lesson := obj(item)
				if id := str(lesson["Id"]); id != "" {
					lessonSubjects[id] = ref(lesson["Subject"])
				}
			}
		} else {
			result.warnings = append(result.warnings, "Brak słownika lekcji; nazwy przedmiotów mogą być niedostępne.")
		}
	} else {
		result.warnings = append(result.warnings, "Nie udało się pobrać słownika lekcji.")
	}
	resolvedSubjects := map[string]string{}
	if response, e := a.data(ctx, "Subjects", nil); e == nil {
		if _, valid := response["Subjects"].([]any); valid {
			resolvedSubjects = names(response, "Subjects", false)
		} else {
			result.warnings = append(result.warnings, "Nieznany format przedmiotów frekwencji.")
		}
	} else {
		result.warnings = append(result.warnings, "Nie udało się pobrać nazw przedmiotów dla frekwencji.")
	}

	for _, item := range raw {
		m, valid := item.(map[string]any)
		if !valid || m["Id"] == nil {
			continue
		}
		typeID := ref(m["Type"])
		kind := types[typeID]
		typeName := str(kind["Name"])
		if typeName == "" {
			typeName = str(kind["Short"])
		}
		if typeName == "" {
			typeName = str(kind["Shortcut"])
		}
		if typeName == "" {
			typeName = "Typ ID: " + typeID
			if typeID == "" {
				typeName = "Nieznany typ"
			}
		}
		presence := "Nieznana"
		if rawKind, known := kind["IsPresenceKind"].(bool); known {
			if rawKind {
				presence = "Tak"
			} else {
				presence = "Nie"
			}
		}
		lessonID := ref(m["Lesson"])
		subjectID := lessonSubjects[lessonID]
		subject := strings.TrimSpace(resolvedSubjects[subjectID])
		if subject == "" && subjectID != "" {
			subject = "Przedmiot ID: " + subjectID
		}
		result.items = append(result.items, map[string]any{
			"id": fmt.Sprint(m["Id"]), "date": m["Date"],
			"lesson": m["LessonNo"], "lessonID": lessonID,
			"type": typeName, "present": presence,
			"subject": subject, "semester": m["Semester"],
		})
	}
	return result, nil
}
