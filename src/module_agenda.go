package main

import (
	"context"
	"fmt"
	"time"
)

// getAgendaV06 reads the school's HomeWorks agenda independently.
// HomeWorks are events (including tests and excursions); they are NOT the
// separate HomeWorkAssignments endpoint for assignments to complete.
func (a *APIClient) getAgendaV06(ctx context.Context) (moduleRows, error) {
	// Hard cap: a failed optional new module cannot monopolize the account
	// session lock and block the previously working features indefinitely.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	primary, err := a.data(ctx, "HomeWorks", nil)
	if err != nil {
		return moduleRows{}, err
	}
	raw, ok := primary["HomeWorks"].([]any)
	if !ok {
		return moduleRows{}, ProtocolError{"Terminarz: brak oczekiwanej listy HomeWorks."}
	}
	result := moduleRows{items: []map[string]any{}}
	if len(raw) == 0 {
		return result, nil
	}

	categoryNames := map[string]string{}
	if response, e := a.data(ctx, "HomeWorks/Categories", nil); e == nil {
		if _, valid := response["Categories"].([]any); valid {
			categoryNames = names(response, "Categories", false)
		} else {
			result.warnings = append(result.warnings, "Nieznany format kategorii terminarza; zachowano identyfikatory.")
		}
	} else {
		result.warnings = append(result.warnings, "Nie udało się pobrać kategorii terminarza.")
	}
	subjectNames := map[string]string{}
	if response, e := a.data(ctx, "Subjects", nil); e == nil {
		if _, valid := response["Subjects"].([]any); valid {
			subjectNames = names(response, "Subjects", false)
		} else {
			result.warnings = append(result.warnings, "Nieznany format przedmiotów terminarza.")
		}
	} else {
		result.warnings = append(result.warnings, "Nie udało się pobrać przedmiotów terminarza.")
	}
	for _, entry := range raw {
		m, valid := entry.(map[string]any)
		if !valid || m["Id"] == nil {
			continue
		}
		categoryID, subjectID := ref(m["Category"]), ref(m["Subject"])
		category, subject := categoryNames[categoryID], subjectNames[subjectID]
		if category == "" && categoryID != "" {
			category = "Kategoria ID: " + categoryID
		}
		if subject == "" && subjectID != "" {
			subject = "Przedmiot ID: " + subjectID
		}
		result.items = append(result.items, map[string]any{
			"id": fmt.Sprint(m["Id"]), "date": m["Date"], "time": m["TimeFrom"],
			"category": category, "subject": subject, "content": m["Content"],
		})
	}
	return result, nil
}
