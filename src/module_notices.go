package main

// v0.11 — SchoolNotices is a fully separate, READ-ONLY module.
// Browsing does not POST MarkAsRead and does not change the user's account.
// This module depends only on the shared authenticated HTTPS data gateway.

import (
	"context"
	"strings"
	"time"
)

func (a *APIClient) getNoticesV11(parent context.Context) (moduleRows, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	payload, err := a.data(ctx, "SchoolNotices", nil)
	if err != nil {
		return moduleRows{}, err
	}
	items, ok := payload["SchoolNotices"].([]any)
	if !ok {
		return moduleRows{}, ProtocolError{"Ogłoszenia: Librus zwrócił nieznany format SchoolNotices."}
	}
	result := moduleRows{items: []map[string]any{}}
	for _, entry := range items {
		item, ok := entry.(map[string]any)
		if !ok || strings.TrimSpace(str(item["Id"])) == "" {
			continue
		}
		read, ok := item["WasRead"].(bool)
		// Unknown read status must not be presented as definitely unread.
		readText := "Nieznany"
		if ok {
			if read {
				readText = "Przeczytane"
			} else {
				readText = "Nieprzeczytane"
			}
		}
		result.items = append(result.items, map[string]any{
			"id": str(item["Id"]), "subject": str(item["Subject"]),
			"content": str(item["Content"]), "created": str(item["CreationDate"]),
			"start": str(item["StartDate"]), "end": str(item["EndDate"]),
			"read": readText,
		})
	}
	if len(items) > 0 && len(result.items) == 0 {
		return moduleRows{}, ProtocolError{"Ogłoszenia: nie rozpoznano żadnego wpisu SchoolNotices."}
	}
	return result, nil
}
