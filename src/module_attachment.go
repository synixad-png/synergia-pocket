package main

// v0.8: attachments for HomeWorkAssignments only. No calls to feature
// modules, no changes to the v0.5 login, messages, grades, or timetable.
// No automatic attachment downloads. Each operation requires a user's click.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const (
	maxHomeworkFile = 20 << 20 // 20 MiB total in-memory limit
	maxHomeworkWait = 150 * time.Second
	sandboxHome     = "https://sandbox.librus.pl/index.php"
)

var sandboxKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,200}$`)
var errSandboxKeyFailed = errors.New("sandbox key could not prepare file")

type homeworkAttachment struct {
	ID           string `json:"id"`
	AssignmentID string `json:"assignmentId"`
	Name         string `json:"name"`
}
type homeworkFile struct {
	Name    string
	Content []byte
}

// This module verifies the attachment ID against the current profile's
// HomeWorkAssignments response, not against caller-provided URLs or names.
// It is intentionally separate from getHomeworkV07(), preserving all v0.7
// output and fallback behavior without changes.
func (a *APIClient) verifiedHomeworkFiles(ctx context.Context, assignmentID string) ([]homeworkAttachment, error) {
	if !idPattern.MatchString(assignmentID) || len(assignmentID) > 40 {
		return nil, ProtocolError{"Nieprawidłowy identyfikator zadania."}
	}
	p, err := a.data(ctx, "HomeWorkAssignments", nil)
	if err != nil {
		return nil, err
	}
	raw, ok := p["HomeWorkAssignments"].([]any)
	if !ok {
		return nil, ProtocolError{"Zadania: nieznany format danych od Librusa."}
	}
	for _, x := range raw {
		item, ok := x.(map[string]any)
		if !ok || str(item["Id"]) != assignmentID {
			continue
		}
		attach, ok := item["HomeworkAssigmentFiles"].([]any)
		if !ok { // null/missing list is empty, but a malformed present value is an error.
			if item["HomeworkAssigmentFiles"] == nil {
				return []homeworkAttachment{}, nil
			}
			return nil, ProtocolError{"Zadanie zawiera niepoprawną listę załączników."}
		}
		files := make([]homeworkAttachment, 0, len(attach))
		seen := map[string]bool{}
		for _, f := range attach {
			m, ok := f.(map[string]any)
			if !ok {
				continue
			}
			id := str(m["Id"])
			if !idPattern.MatchString(id) || len(id) > 40 || seen[id] {
				continue
			}
			seen[id] = true
			filename := strings.TrimSpace(str(m["Name"]))
			if filename == "" {
				filename = "zalacznik-" + id
			}
			files = append(files, homeworkAttachment{
				ID: id, AssignmentID: assignmentID, Name: safeHomeworkName(filename),
			})
		}
		return files, nil
	}
	return nil, ProtocolError{"Zadanie nie należy do wybranego konta lub już nie istnieje."}
}

func safeHomeworkName(name string) string {
	// Content-Disposition must not contain paths, controls, Windows device
	// names or separators; also reject dangerous right-to-left overrides.
	name = strings.ReplaceAll(name, "\\", "/")
	pieces := strings.Split(name, "/")
	name = strings.TrimSpace(pieces[len(pieces)-1])
	var out []rune
	for _, r := range name {
		if unicode.IsControl(r) || r == '\u202e' || r == '\u202d' || r == '\u2066' || r == '\u2067' || r == '\u2068' || r == '\u2069' || strings.ContainsRune(`<>:"|?*`, r) {
			continue
		}
		out = append(out, r)
		if len(out) == 125 {
			break
		}
	}
	value := strings.Trim(string(out), " .")
	if value == "" {
		value = "zalacznik.bin"
	}
	upper := strings.ToUpper(strings.Split(value, ".")[0])
	switch upper {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		value = "plik_" + value
	}
	return value
}

// Only the exact HTTPS Synergia file entry point and sandbox.librus.pl are
// permitted. Never allow a server-supplied redirect to another hostname.
func sandboxDownloadLink(location string) (string, error) {
	u, e := url.Parse(location)
	if e != nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "sandbox.librus.pl") || u.Port() != "" || u.User != nil || u.Fragment != "" {
		return "", ProtocolError{"Pobieranie: zablokowano nieznany adres serwera plików."}
	}
	if u.Path == "/index.php" && u.Query().Get("action") == "CSTryToDownload" && sandboxKeyPattern.MatchString(u.Query().Get("singleUseKey")) {
		return u.String(), nil
	}
	if strings.HasPrefix(u.Path, "/GetFile/") && !strings.Contains(u.EscapedPath(), "..") && !strings.HasSuffix(u.Path, "/") && u.RawQuery == "" {
		return u.String(), nil
	}
	return "", ProtocolError{"Pobieranie: nieznany format adresu serwera plików."}
}

func (a *APIClient) homeworkHTTP(ctx context.Context, method, rawURL string, body io.Reader, headers map[string]string, limit int64) (*http.Response, []byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Port() != "" {
		return nil, nil, ProtocolError{"Pobieranie: niepoprawny adres HTTPS."}
	}
	host := strings.ToLower(u.Hostname())
	if host != "sandbox.librus.pl" && host != "synergia.librus.pl" {
		return nil, nil, ProtocolError{"Pobieranie: adres poza domeną plików Librusa."}
	}
	if host == "synergia.librus.pl" && !strings.HasPrefix(u.Path, "/homework/downloadFile/") {
		return nil, nil, ProtocolError{"Pobieranie: niepoprawna ścieżka Synergii."}
	}
	req, e := http.NewRequestWithContext(ctx, method, rawURL, body)
	if e != nil {
		return nil, nil, ProtocolError{"Pobieranie: niepoprawne żądanie."}
	}
	req.Header.Set("User-Agent", ua)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	response, e := a.client.Do(req)
	if e != nil {
		return nil, nil, ProtocolError{"Nie udało się połączyć z serwerem plików Librusa."}
	}
	defer response.Body.Close()
	if response.ContentLength > limit {
		return nil, nil, ProtocolError{"Załącznik przekracza limit rozmiaru 20 MB."}
	}
	bytes, e := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if e != nil {
		return nil, nil, ProtocolError{"Błąd odczytu pobieranego pliku."}
	}
	if int64(len(bytes)) > limit {
		return nil, nil, ProtocolError{"Załącznik przekracza limit rozmiaru 20 MB."}
	}
	return response, bytes, nil
}

func hasHTMLPayload(response *http.Response, content []byte) bool {
	media := strings.ToLower(response.Header.Get("Content-Type"))
	if strings.Contains(media, "text/html") || strings.Contains(media, "application/xhtml") {
		return true
	}
	peek := strings.ToLower(strings.TrimSpace(string(content[:min(len(content), 256)])))
	return strings.HasPrefix(peek, "<!doctype html") || strings.HasPrefix(peek, "<html")
}

func readHomeworkBinary(resp *http.Response, bytes []byte, fallback string) (homeworkFile, error) {
	if resp.StatusCode != 200 || len(bytes) == 0 || hasHTMLPayload(resp, bytes) {
		return homeworkFile{}, ProtocolError{"Serwer plików nie zwrócił gotowego załącznika."}
	}
	name := fallback
	if disposition := resp.Header.Get("Content-Disposition"); disposition != "" {
		_, params, e := mime.ParseMediaType(disposition)
		if e == nil && strings.TrimSpace(params["filename"]) != "" {
			name = params["filename"]
		}
	}
	return homeworkFile{Name: safeHomeworkName(name), Content: bytes}, nil
}

// Called from one explicit UI download. Verification is locked while the
// selected profile's auth and cookie status are checked; polling the sandbox
// happens after releasing that lock so other modules remain responsive.
func (a *APIClient) startHomeworkDownload(ctx context.Context, assignmentID, fileID string) (homeworkAttachment, string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	files, err := a.verifiedHomeworkFiles(ctx, assignmentID)
	if err != nil {
		return homeworkAttachment{}, "", err
	}
	var attachment homeworkAttachment
	found := false
	for _, f := range files {
		if f.ID == fileID {
			attachment = f
			found = true
			break
		}
	}
	if !found {
		return homeworkAttachment{}, "", ProtocolError{"Tego pliku nie ma w załącznikach wybranego zadania."}
	}
	if err = a.ensure(ctx); err != nil {
		return homeworkAttachment{}, "", err
	}
	endpoint := "https://synergia.librus.pl/homework/downloadFile/" + fileID
	resp, raw, err := a.homeworkHTTP(ctx, "GET", endpoint, nil, nil, 64*1024)
	if err != nil {
		return homeworkAttachment{}, "", err
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return homeworkAttachment{}, "", ProtocolError{"Sesja plików Librusa wygasła. Zaloguj się ponownie."}
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		if resp.StatusCode == 200 && hasHTMLPayload(resp, raw) {
			return homeworkAttachment{}, "", ProtocolError{"Sesja plików Librusa wymaga ponownego zalogowania."}
		}
		return homeworkAttachment{}, "", ProtocolError{"Librus nie zwrócił adresu pobierania załącznika."}
	}
	u, e := url.Parse(endpoint)
	if e != nil {
		return homeworkAttachment{}, "", e
	}
	target, e := u.Parse(loc)
	if e != nil {
		return homeworkAttachment{}, "", ProtocolError{"Nieprawidłowe przekierowanie pobierania."}
	}
	link, e := sandboxDownloadLink(target.String())
	if e != nil {
		return homeworkAttachment{}, "", e
	}
	return attachment, link, nil
}

func sandboxSleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Expose `pause` for deterministic mock tests without sleeping minutes.
func (a *APIClient) sandboxFile(ctx context.Context, link, filename string, pause func(context.Context, time.Duration) error) (homeworkFile, error) {
	info, e := url.Parse(link)
	if e != nil {
		return homeworkFile{}, ProtocolError{"Niepoprawny link załącznika."}
	}
	if _, e = sandboxDownloadLink(link); e != nil {
		return homeworkFile{}, e
	}
	headers := map[string]string{"Referer": link}
	resp, _, e := a.homeworkHTTP(ctx, "GET", link, nil, headers, 64*1024)
	if e != nil {
		return homeworkFile{}, e
	}
	if resp.StatusCode != 200 {
		return homeworkFile{}, ProtocolError{"Serwer plików Librusa odrzucił przygotowanie pobierania."}
	}

	if info.Path == "/index.php" {
		key := info.Query().Get("singleUseKey")
		for attempt := 0; attempt < 15; attempt++ {
			form := url.Values{"singleUseKey": {key}}
			response, raw, err := a.homeworkHTTP(ctx, "POST", sandboxHome+"?action=CSCheckKey", strings.NewReader(form.Encode()), map[string]string{
				"Content-Type": "application/x-www-form-urlencoded", "Referer": link,
			}, 64*1024)
			if err != nil {
				return homeworkFile{}, err
			}
			if response.StatusCode != 200 {
				return homeworkFile{}, ProtocolError{"Serwer plików odrzucił sprawdzenie dostępności załącznika."}
			}
			var answer struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(raw, &answer) != nil {
				return homeworkFile{}, ProtocolError{"Serwer plików zwrócił niepoprawny stan pobierania."}
			}
			switch answer.Status {
			case "ready":
				downloadURL := sandboxHome + "?action=CSDownload&singleUseKey=" + url.QueryEscape(key)
				r, data, er := a.homeworkHTTP(ctx, "GET", downloadURL, nil, headers, maxHomeworkFile)
				if er != nil {
					return homeworkFile{}, er
				}
				return readHomeworkBinary(r, data, filename)
			case "not_downloaded_yet":
				if attempt == 14 {
					break
				}
				delay := 10 * time.Second
				if attempt < 3 {
					delay = 3 * time.Second
				}
				if er := pause(ctx, delay); er != nil {
					return homeworkFile{}, ProtocolError{"Przekroczono czas pobierania załącznika."}
				}
			case "download_failed":
				// This is the upstream single-use-key failure, not a write operation.
				return homeworkFile{}, errSandboxKeyFailed
			default:
				return homeworkFile{}, ProtocolError{"Serwer plików zwrócił nieznany status pobierania."}
			}
		}
	} else if strings.HasPrefix(info.Path, "/GetFile/") {
		for attempt := 0; attempt < 15; attempt++ {
			r, raw, err := a.homeworkHTTP(ctx, "GET", link+"/get", nil, headers, maxHomeworkFile)
			if err != nil {
				return homeworkFile{}, err
			}
			if r.StatusCode == 200 && !hasHTMLPayload(r, raw) {
				return readHomeworkBinary(r, raw, filename)
			}
			if r.StatusCode >= 400 && r.StatusCode != 404 {
				return homeworkFile{}, ProtocolError{"Serwer plików odmówił pobrania załącznika."}
			}
			if attempt < 14 {
				if err := pause(ctx, 2*time.Second); err != nil {
					return homeworkFile{}, ProtocolError{"Przekroczono czas pobierania załącznika."}
				}
			}
		}
	}
	return homeworkFile{}, ProtocolError{"Załącznik nie jest gotowy. Spróbuj ponownie później."}
}

func (a *APIClient) downloadHomeworkAttachment(ctx context.Context, assignmentID, fileID string) (homeworkFile, error) {
	ctx, cancel := context.WithTimeout(ctx, maxHomeworkWait)
	defer cancel()
	if !idPattern.MatchString(fileID) || len(fileID) > 40 {
		return homeworkFile{}, ProtocolError{"Niepoprawny identyfikator pliku."}
	}
	// A single user-triggered download may request up to three fresh
	// sandbox keys if Librus explicitly says download_failed. No other
	// failures are automatically retried, and nothing is written to Librus.
	for round := 0; round < 3; round++ {
		meta, link, err := a.startHomeworkDownload(ctx, assignmentID, fileID)
		if err != nil {
			return homeworkFile{}, err
		}
		result, err := a.sandboxFile(ctx, link, meta.Name, sandboxSleep)
		if err == nil {
			return result, nil
		}
		if !errors.Is(err, errSandboxKeyFailed) {
			return homeworkFile{}, err
		}
		if round < 2 {
			if err := sandboxSleep(ctx, 2*time.Second); err != nil {
				return homeworkFile{}, ProtocolError{"Przekroczono czas pobierania załącznika."}
			}
		}
	}
	return homeworkFile{}, ProtocolError{"Librus nie przygotował załącznika po trzech próbach. Spróbuj ponownie później."}
}

// Route helper is installed by main.go only; errors from this module do not
// alter any shared login/session functions or other routes.
func (app *LocalApp) homeworkFilesHandler(w http.ResponseWriter, r *http.Request, download bool) {
	if !app.validRequest(w, r) {
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method", 405)
		return
	}
	profile := r.URL.Query().Get("profile")
	if profile != "0" && profile != "1" {
		serveJSON(w, 400, map[string]string{"error": "Nieprawidłowy profil."})
		return
	}
	idx := 0
	if profile == "1" {
		idx = 1
	}
	assignment := r.URL.Query().Get("assignment")
	file := r.URL.Query().Get("file")
	if !idPattern.MatchString(assignment) || len(assignment) > 40 ||
		(download && (!idPattern.MatchString(file) || len(file) > 40)) {
		serveJSON(w, 400, map[string]string{"error": "Nieprawidłowy identyfikator zadania lub pliku."})
		return
	}
	app.mu.RLock()
	client := app.profiles[idx]
	app.mu.RUnlock()
	if client == nil {
		serveJSON(w, 401, map[string]string{"error": "Najpierw zaloguj wybrany profil."})
		return
	}
	if !download {
		client.mu.Lock()
		files, err := client.verifiedHomeworkFiles(r.Context(), assignment)
		client.mu.Unlock()
		if err != nil {
			serverError(w, err)
			return
		}
		serveJSON(w, 200, map[string]any{"files": files})
		return
	}
	data, err := client.downloadHomeworkAttachment(r.Context(), assignment, file)
	if err != nil {
		serverError(w, err)
		return
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": data.Name})
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("Content-Length", fmt.Sprint(len(data.Content)))
	w.WriteHeader(200)
	_, _ = w.Write(data.Content)
}
