package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// getHomeworkV07 implements a fully independent, read-only homework module.
// It uses HomeWorkAssignments (NOT HomeWorks, which is the school agenda).
// The optional teacher/category catalogs are read without invoking any
// other feature modules. Attachments are metadata-only at this stage;
// downloading them requires a separate verified flow and user action.
func (a *APIClient) getHomeworkV07(ctx context.Context) (moduleRows, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	primary, err := a.data(ctx, "HomeWorkAssignments", nil)
	if err != nil {
		return moduleRows{}, err
	}
	raw, ok := primary["HomeWorkAssignments"].([]any)
	if !ok {
		return moduleRows{}, ProtocolError{"Zadania domowe: brak oczekiwanej listy HomeWorkAssignments."}
	}
	output := moduleRows{items: []map[string]any{}}
	if len(raw) == 0 {
		return output, nil
	}

	// The optional reference lookups are completely separate from the
	// primary request. Their failure never suppresses the homework content.
	hasTeacher, hasCategory := false, false
	for _, v := range raw {
		m, valid := v.(map[string]any)
		if !valid || m["Id"] == nil {
			continue
		}
		hasTeacher = hasTeacher || ref(m["Teacher"]) != ""
		hasCategory = hasCategory || ref(m["Category"]) != ""
	}
	teacherNames := map[string]string{}
	categoryNames := map[string]string{}
	if hasTeacher {
		if response, e := a.data(ctx, "Users", nil); e == nil {
			if _, valid := response["Users"].([]any); valid {
				teacherNames = names(response, "Users", true)
			} else {
				output.warnings = append(output.warnings, "Nieznany format nazw nauczycieli dla zadań domowych.")
			}
		} else {
			output.warnings = append(output.warnings, "Nie udało się pobrać nauczycieli dla zadań domowych.")
		}
	}
	if hasCategory {
		if response, e := a.data(ctx, "HomeWorkAssignments/Categories", nil); e == nil {
			if cats, valid := response["Categories"].([]any); valid {
				for _, v := range cats {
					m, valid := v.(map[string]any)
					if valid && m["Id"] != nil {
						// This endpoint uses CategoryName, not Name.
						categoryNames[str(m["Id"])] = str(m["CategoryName"])
					}
				}
			} else {
				output.warnings = append(output.warnings, "Nieznany format kategorii zadań domowych.")
			}
		} else {
			output.warnings = append(output.warnings, "Nie udało się pobrać kategorii zadań domowych.")
		}
	}

	for _, row := range raw {
		m, valid := row.(map[string]any)
		if !valid || m["Id"] == nil {
			continue
		}
		teacherID, categoryID := ref(m["Teacher"]), ref(m["Category"])
		teacher, category := strings.TrimSpace(teacherNames[teacherID]), strings.TrimSpace(categoryNames[categoryID])
		if teacher == "" && teacherID != "" {
			teacher = "Nauczyciel ID: " + teacherID
		}
		if category == "" && categoryID != "" {
			category = "Kategoria ID: " + categoryID
		}

		// The source names this field HomeworkAssigmentFiles (sic). We do
		// not guess attachment URLs, download files, or silently discard
		// malformed metadata when the upstream says attachments exist.
		files := []string{}
		if meta, present := m["HomeworkAssigmentFiles"]; present && meta != nil {
			list, valid := meta.([]any)
			if !valid {
				output.warnings = append(output.warnings,
					"Zadanie "+fmt.Sprint(m["Id"])+": nieznany format listy załączników.")
			} else {
				for _, f := range list {
					obj, valid := f.(map[string]any)
					if !valid {
						continue
					}
					title := strings.TrimSpace(str(obj["Name"]))
					if title == "" && obj["Id"] != nil {
						title = "Plik ID: " + str(obj["Id"])
					}
					if title != "" {
						files = append(files, title)
					}
				}
			}
		}
		attachmentList := strings.Join(files, ", ")
		if attachmentList == "" {
			attachmentList = "—"
		}
		mustSend := ""
		if value, exists := m["MustSendAttachFile"].(bool); exists {
			if value {
				mustSend = "Wymagany plik"
			} else {
				mustSend = "Plik niewymagany"
			}
		}
		output.items = append(output.items, map[string]any{
			"id":   fmt.Sprint(m["Id"]),
			"date": m["Date"], "dueDate": m["DueDate"],
			"topic": m["Topic"], "content": m["Text"],
			"teacher": teacher, "category": category,
			"attachmentNames": attachmentList,
			"requirement":     mustSend,
		})
	}
	return output, nil
}
