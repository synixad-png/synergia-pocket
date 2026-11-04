package main

import (
	"context"
	"fmt"
	"time"
)

// getNotesV06 reads behaviour notices, independently of other modules.
// Notes.Positive: 0 negative, 1 positive, 2 neutral; any unknown value
// is explicitly shown as 'Nieokreślona' rather than being guessed.
func (a *APIClient) getNotesV06(ctx context.Context) (moduleRows, error) {
	// Hard cap: a failed optional new module cannot monopolize the account
	// session lock and block the previously working features indefinitely.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	primary, err := a.data(ctx, "Notes", nil)
	if err != nil {
		return moduleRows{}, err
	}
	raw, ok := primary["Notes"].([]any)
	if !ok {
		return moduleRows{}, ProtocolError{"Uwagi: brak oczekiwanej listy Notes."}
	}
	result := moduleRows{items: []map[string]any{}}
	if len(raw) == 0 {
		return result, nil
	}

	categoryNames := map[string]string{}
	if response, e := a.data(ctx, "Notes/Categories", nil); e == nil {
		if list, valid := response["Categories"].([]any); valid {
			for _, entry := range list {
				m := obj(entry)
				if id := str(m["Id"]); id != "" {
					categoryNames[id] = str(m["CategoryName"])
				}
			}
		} else {
			result.warnings = append(result.warnings, "Nieznany format kategorii uwag.")
		}
	} else {
		result.warnings = append(result.warnings, "Nie udało się pobrać kategorii uwag.")
	}
	teacherNames := map[string]string{}
	if response, e := a.data(ctx, "Users", nil); e == nil {
		if _, valid := response["Users"].([]any); valid {
			teacherNames = names(response, "Users", true)
		} else {
			result.warnings = append(result.warnings, "Nieznany format listy nauczycieli dla uwag.")
		}
	} else {
		result.warnings = append(result.warnings, "Nie udało się pobrać nazw nauczycieli dla uwag.")
	}
	for _, entry := range raw {
		m, valid := entry.(map[string]any)
		if !valid || m["Id"] == nil {
			continue
		}
		categoryID, teacherID := ref(m["Category"]), ref(m["Teacher"])
		category, teacher := categoryNames[categoryID], teacherNames[teacherID]
		if category == "" && categoryID != "" {
			category = "Kategoria ID: " + categoryID
		}
		if teacher == "" && teacherID != "" {
			teacher = "Nauczyciel ID: " + teacherID
		}
		sentiment := "Nieokreślona"
		switch v := m["Positive"].(type) {
		case float64:
			switch v {
			case 0:
				sentiment = "Negatywna"
			case 1:
				sentiment = "Pozytywna"
			case 2:
				sentiment = "Neutralna"
			}
		}
		result.items = append(result.items, map[string]any{
			"id": fmt.Sprint(m["Id"]), "date": m["Date"],
			"content": m["Text"], "category": category,
			"teacher": teacher, "sentiment": sentiment,
		})
	}
	return result, nil
}
