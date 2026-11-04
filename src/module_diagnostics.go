package main

// v0.9 — diagnostics is deliberately a separate, read-only orchestration layer.
// It calls each existing feature independently and does not alter any feature
// implementation. Errors are returned as per-module results; they never become
// a failure of the diagnostics endpoint or cause a request to a different module.
// No credentials, cookies, message content or student names are reported.

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type diagnosticDefinition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var diagnosticModules = []diagnosticDefinition{
	{"me", "Logowanie / dane konta"},
	{"messages", "Wiadomości"},
	{"contacts", "Kontakty"},
	{"grades", "Oceny"},
	{"timetable", "Plan lekcji"},
	{"attendance", "Frekwencja"},
	{"agenda", "Terminarz"},
	{"notes", "Uwagi"},
	{"homework", "Zadania domowe"},
	{"attachments", "Załączniki do zadań"},
}

type diagnosticResult struct {
	Module    string `json:"module"`
	State     string `json:"state"` // ok, empty, partial, blocked, failed, unverified
	Count     int    `json:"count"`
	Detail    string `json:"detail"`
	ErrorKind string `json:"errorKind,omitempty"`
	ElapsedMS int64  `json:"elapsedMs"`
}

func diagnosticKnownModule(name string) bool {
	for _, module := range diagnosticModules {
		if module.ID == name {
			return true
		}
	}
	return false
}

// Do not forward opaque upstream errors to this UI or its export. The result
// may be shared with technical support and must not include school data.
func diagnosticError(err error) (string, string) {
	if err == nil {
		return "", ""
	}
	var diag ProtocolError
	if errors.As(err, &diag) {
		s := strings.ToLower(diag.Error())
		switch {
		case strings.Contains(s, "zaloguj") || strings.Contains(s, "sesja wygas") || strings.Contains(s, "oauth_token"):
			return "session", "Sesja wymaga ponownego logowania."
		case strings.Contains(s, "http 401"):
			return "session_or_access", "Librus zwrócił HTTP 401: sesja lub brak uprawnień."
		case strings.Contains(s, "http 403") || strings.Contains(s, "brak dostępu") || strings.Contains(s, "nie udostępnia"):
			return "permissions", "Librus nie udostępnił tego modułu (HTTP 403 / brak dostępu)."
		case strings.Contains(s, "http 404"):
			return "endpoint", "Librus nie udostępnił wskazanego endpointu (HTTP 404)."
		case strings.Contains(s, "http 429"):
			return "rate_limit", "Librus ograniczył liczbę żądań (HTTP 429)."
		case strings.Contains(s, "http 502") || strings.Contains(s, "http 503") || strings.Contains(s, "http 504"):
			return "upstream", "Serwer Librusa jest chwilowo niedostępny."
		case strings.Contains(s, "nieoczekiwany") || strings.Contains(s, "format") || strings.Contains(s, "json") || strings.Contains(s, "nieznany"):
			return "format", "Librus zwrócił nieobsługiwany format danych."
		case strings.Contains(s, "połączen") || strings.Contains(s, "czas") || strings.Contains(s, "przekrocz"):
			return "network", "Błąd połączenia lub przekroczony czas żądania."
		case strings.Contains(s, "przekierowan"):
			return "redirect", "Nie udało się potwierdzić bezpiecznego przekierowania."
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout", "Moduł nie odpowiedział w przewidzianym czasie."
	}
	if errors.Is(err, context.Canceled) {
		return "canceled", "Przerwano sprawdzanie modułu."
	}
	return "unknown", "Nie udało się sprawdzić tego modułu. Zapisz raport TXT i spróbuj sprawdzić moduł osobno."
}

// Caller serializes use of the account's cookie jar with c.mu, as normal
// /api/data does. The same shared HTTPS/authentication code is reused; no
// child feature module calls another child feature module.
func (c *APIClient) diagnosticProbe(parent context.Context, module string) diagnosticResult {
	result := diagnosticResult{Module: module, State: "failed"}
	started := time.Now()
	defer func() { result.ElapsedMS = time.Since(started).Milliseconds() }() // see explicit update below
	// Bound upstream time, independently of the other diagnostics calls.
	ctx, cancel := context.WithTimeout(parent, 32*time.Second)
	defer cancel()
	c.mu.Lock()
	defer c.mu.Unlock()

	var n int
	var warnings []string
	var err error
	switch module {
	case "me":
		var data map[string]any
		data, err = c.data(ctx, "Me", nil)
		if err == nil {
			if _, ok := data["Me"].(map[string]any); !ok {
				err = ProtocolError{"Dane konta: nieoczekiwany format Me."}
			} else {
				n = 1
			}
		}
	case "messages":
		var data []map[string]any
		data, err = c.getMessages(ctx)
		n = len(data)
	case "contacts":
		var data []map[string]any
		data, err = c.getContacts(ctx)
		n = len(data)
	case "grades":
		var data []map[string]any
		data, err = c.getGrades(ctx)
		n = len(data)
	case "timetable":
		var data []map[string]any
		data, err = c.getTimetable(ctx)
		n = len(data)
	case "attendance":
		var data moduleRows
		data, err = c.getAttendanceV06(ctx)
		n, warnings = len(data.items), data.warnings
	case "agenda":
		var data moduleRows
		data, err = c.getAgendaV06(ctx)
		n, warnings = len(data.items), data.warnings
	case "notes":
		var data moduleRows
		data, err = c.getNotesV06(ctx)
		n, warnings = len(data.items), data.warnings
	case "homework":
		var data moduleRows
		data, err = c.getHomeworkV07(ctx)
		n, warnings = len(data.items), data.warnings
	case "attachments":
		// Metadata probe is NON-DOWNLOADING and independently queries
		// HomeWorkAssignments; never calls the v0.7 homework parser or
		// the v0.8 attachment-transfer function. Even if metadata exists,
		// the real sandbox transfer must stay marked UNVERIFIED.
		var data map[string]any
		data, err = c.data(ctx, "HomeWorkAssignments", nil)
		if err == nil {
			raw, ok := data["HomeWorkAssignments"].([]any)
			if !ok {
				err = ProtocolError{"Załączniki: nieznany format HomeWorkAssignments."}
			}
			if err == nil {
				for _, value := range raw {
					item, ok := value.(map[string]any)
					if !ok {
						continue
					}
					if attachments, ok := item["HomeworkAssigmentFiles"].([]any); ok {
						for _, attachment := range attachments {
							if metadata, ok := attachment.(map[string]any); ok && idPattern.MatchString(str(metadata["Id"])) && len(str(metadata["Id"])) <= 40 {
								n++
							}
						}
					}
				}
				result.Count = n
				result.State = "unverified"
				if n == 0 {
					result.Detail = "Brak plików na koncie. Transfer z Librusa pozostaje niezweryfikowany."
				} else {
					result.Detail = "Wykryto metadane plików. Samo pobieranie z Librusa wymaga osobnego testu."
				}
				result.ElapsedMS = time.Since(started).Milliseconds()
				return result
			}
		}
	default:
		err = ProtocolError{"Nieznany moduł diagnostyczny."}
	}
	result.ElapsedMS = time.Since(started).Milliseconds()
	if err != nil {
		result.ErrorKind, result.Detail = diagnosticError(err)
		if result.ErrorKind == "session" {
			result.State = "blocked"
		}
		return result
	}
	result.Count = n
	switch {
	case len(warnings) > 0:
		result.State = "partial"
		result.Detail = "Pobrano dane, ale część informacji uzupełniających jest niedostępna."
	case n == 0:
		result.State = "empty"
		result.Detail = "Połączenie i odpowiedź poprawne; brak wpisów."
	default:
		result.State = "ok"
		result.Detail = "Dane pobrane prawidłowo."
	}
	return result
}

func installDiagnosticsRoutes(mux *http.ServeMux, a *LocalApp) {
	// Fixed public definitions contain no credentials or personal data.
	mux.HandleFunc("/api/diagnostics/modules", func(w http.ResponseWriter, r *http.Request) {
		if !a.validRequest(w, r) {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "Method", http.StatusMethodNotAllowed)
			return
		}
		serveJSON(w, 200, map[string]any{"modules": diagnosticModules, "mode": "manual_only"})
	})
	mux.HandleFunc("/api/diagnostics/check", func(w http.ResponseWriter, r *http.Request) {
		if !a.validRequest(w, r) {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "Method", http.StatusMethodNotAllowed)
			return
		}
		module := r.URL.Query().Get("module")
		if !diagnosticKnownModule(module) {
			serveJSON(w, 400, map[string]string{"error": "Nieznany moduł diagnostyczny."})
			return
		}
		index, err := strconv.Atoi(r.URL.Query().Get("profile"))
		if err != nil || index < 0 || index > 1 {
			serveJSON(w, 400, map[string]string{"error": "Nieprawidłowy profil."})
			return
		}
		a.mu.RLock()
		c := a.profiles[index]
		a.mu.RUnlock()
		if c == nil {
			serveJSON(w, 401, map[string]string{"error": "Najpierw zaloguj wybrany profil."})
			return
		}
		// A failure stays isolated in one JSON result (HTTP 200).
		// In particular, a 401 from one school API is not treated as a
		// universal authentication failure affecting other modules.
		result := c.diagnosticProbe(r.Context(), module)
		serveJSON(w, 200, result)
	})
	mux.HandleFunc("/api/diagnostics/sample-file", func(w http.ResponseWriter, r *http.Request) {
		if !a.validRequest(w, r) {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "Method", http.StatusMethodNotAllowed)
			return
		}
		// This never connects to Librus; it validates only local delivery.
		// No fake assignment is ever created on a real school account.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="SYNERGIA_API_PLIK_TESTOWY.txt"`)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("SYNERGIA API v0.9 — TEST LOKALNY\n\nPlik wygenerowany lokalnie.\nNie pobrano go z Librusa.\nTen test nie potwierdza działania pobierania prawdziwych załączników.\n"))
	})
}
