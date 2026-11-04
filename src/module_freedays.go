package main

// v0.11 — free-day calendar. SchoolFreeDays (school-wide) and
// ClassFreeDays (all classes!) are read independently from other features.
// Never display another class's entries: Classes.Class.Id is mandatory
// before we add ANY class-specific event.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

func dayText(value any) string {
	valueText := strings.TrimSpace(str(value))
	// Librus commonly returns ISO dates; preserve the server representation.
	return valueText
}

func classIDFromClasses(response map[string]any) string {
	cls, ok := response["Class"].(map[string]any)
	if !ok {
		return ""
	}
	return strings.TrimSpace(str(cls["Id"]))
}

// Append only well-formed day records; preserve unknown type as a generic
// label and never assert that a partially free day frees the whole day.
func appendFreeDaysV11(out *moduleRows, source []any, scope, classID string, types map[string]string) int {
	added := 0
	for _, entry := range source {
		item, ok := entry.(map[string]any)
		if !ok || item["Id"] == nil {
			continue
		}
		start, end := dayText(item["DateFrom"]), dayText(item["DateTo"])
		if start == "" || end == "" {
			continue
		}
		if scope == "Klasa dziecka" {
			// Both the user's class and the record's class must be explicit.
			if classID == "" || ref(item["Class"]) == "" || ref(item["Class"]) != classID {
				continue
			}
		}
		name := strings.TrimSpace(str(item["Name"]))
		typeID := ref(item["Type"])
		if name == "" {
			name = types[typeID]
		}
		if name == "" {
			name = "Dzień wolny (brak nazwy)"
		}
		lessonFrom := strings.TrimSpace(str(item["LessonNoFrom"]))
		lessonTo := strings.TrimSpace(str(item["LessonNoTo"]))
		lessons := "Cały dzień"
		whole := true
		if lessonFrom != "" || lessonTo != "" {
			whole = false
			switch {
			case lessonFrom != "" && lessonTo != "":
				lessons = "Lekcje " + lessonFrom + "–" + lessonTo
			case lessonFrom != "":
				lessons = "Od lekcji " + lessonFrom
			default:
				lessons = "Do lekcji " + lessonTo
			}
		}
		out.items = append(out.items, map[string]any{
			"id": str(item["Id"]), "name": name, "from": start, "to": end,
			"scope": scope, "lessons": lessons, "wholeDay": whole,
		})
		added++
	}
	return added
}

func (a *APIClient) getFreeDaysV11(parent context.Context) (moduleRows, error) {
	ctx, cancel := context.WithTimeout(parent, 35*time.Second)
	defer cancel()
	result := moduleRows{items: []map[string]any{}}
	var schoolError, classError error

	// The school-wide endpoint stands on its own; a refused class endpoint
	// must not prevent these entries from being displayed.
	school, err := a.data(ctx, "SchoolFreeDays", nil)
	if err != nil {
		schoolError = err
		result.warnings = append(result.warnings, "Nie udało się pobrać dni wolnych całej szkoły.")
	} else if rows, ok := school["SchoolFreeDays"].([]any); ok {
		appendFreeDaysV11(&result, rows, "Cała szkoła", "", nil)
	} else {
		schoolError = ProtocolError{"Dni wolne: nieznany format SchoolFreeDays."}
		result.warnings = append(result.warnings, "Librus zwrócił nieznany format dni wolnych całej szkoły.")
	}

	// IMPORTANT: ClassFreeDays contains all classes, including other students.
	// Without the exact current class ID, it is unsafe to show the list.
	classID := ""
	classes, err := a.data(ctx, "Classes", nil)
	if err == nil {
		classID = classIDFromClasses(classes)
	}
	if classID == "" {
		classError = ProtocolError{"Dni wolne: nie ustalono klasy bieżącego profilu."}
		result.warnings = append(result.warnings, "Nie ustalono klasy dziecka: pominięto dni wolne przypisane klasom, aby nie pokazać innych klas.")
	} else {
		classDays, e := a.data(ctx, "ClassFreeDays", nil)
		if e != nil {
			classError = e
			result.warnings = append(result.warnings, "Nie udało się pobrać dni wolnych klasy dziecka.")
		} else if rows, ok := classDays["ClassFreeDays"].([]any); ok {
			typeIDs := map[string]bool{}
			for _, v := range rows {
				m := obj(v)
				if ref(m["Class"]) == classID && ref(m["Type"]) != "" {
					typeIDs[ref(m["Type"])] = true
				}
			}
			types := map[string]string{}
			if len(typeIDs) > 0 {
				categories, err := a.data(ctx, "Calendars/ClassFreeDays/Types", nil)
				if err != nil {
					result.warnings = append(result.warnings, "Brak nazw rodzajów dni wolnych klasy; zachowano daty.")
				} else if typesList, ok := categories["Types"].([]any); ok {
					for _, v := range typesList {
						m := obj(v)
						if id := str(m["Id"]); id != "" {
							types[id] = str(m["Name"])
						}
					}
				} else {
					result.warnings = append(result.warnings, "Nieznany format rodzajów dni wolnych klasy.")
				}
			}
			appendFreeDaysV11(&result, rows, "Klasa dziecka", classID, types)
		} else {
			classError = ProtocolError{"Dni wolne: nieznany format ClassFreeDays."}
			result.warnings = append(result.warnings, "Librus zwrócił nieznany format dni wolnych klasy.")
		}
	}
	if schoolError != nil && classError != nil {
		return moduleRows{}, ProtocolError{fmt.Sprintf("Dni wolne: nie udało się odczytać ani szkoły, ani klasy (%v; %v).", schoolError, classError)}
	}
	// Stable display order: upcoming dates in ascending order.
	sort.SliceStable(result.items, func(i, j int) bool {
		return str(result.items[i]["from"]) < str(result.items[j]["from"])
	})
	return result, nil
}
