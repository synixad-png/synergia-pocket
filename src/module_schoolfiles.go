package main

// v0.10 — SchoolFiles only. The module neither invokes nor changes any
// other feature module. HTTPS authentication is the only shared dependency.
// Listing requires an explicit tab opening; downloads require another click.

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	maxSchoolFile int64 = 20 << 20
	maxSchoolWait       = 150 * time.Second
	schoolSandbox       = "https://sandbox.librus.pl/index.php"
)

var schoolIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,90}$`)
var schoolPathPattern = regexp.MustCompile(`^/pliki_szkoly/pobierz/[A-Za-z0-9_-]{1,90}$`)
var schoolKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,200}$`)
var errSchoolKeyFailed = errors.New("school file sandbox key failed")

type schoolDocument struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Date         string `json:"date"`
	Status       string `json:"status"`
	Downloadable bool   `json:"downloadable"`
	Path         string `json:"-"`
}
type schoolDownload struct {
	Name string
	Data []byte
}

func safeSchoolFileName(input string) string {
	input = strings.ReplaceAll(input, "\\", "/")
	parts := strings.Split(input, "/")
	name := strings.TrimSpace(parts[len(parts)-1])
	var runes []rune
	for _, r := range name {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"|?*`, r) || r == '\u202e' || r == '\u202d' || (r >= '\u2066' && r <= '\u2069') {
			continue
		}
		runes = append(runes, r)
		if len(runes) >= 130 {
			break
		}
	}
	result := strings.Trim(string(runes), " .")
	if result == "" {
		result = "dokument.bin"
	}
	root := strings.ToUpper(strings.Split(result, ".")[0])
	switch root {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		result = "plik_" + result
	}
	return result
}

func parseSchoolDownloadPath(v string) string {
	// Exact ASCII relative paths only. Never trust URLs, query parameters,
	// encoded slashes, fragments, dot segments, other origins or ports.
	if !schoolPathPattern.MatchString(v) {
		return ""
	}
	return v
}

func (c *APIClient) schoolDocuments(ctx context.Context) ([]schoolDocument, error) {
	raw, err := c.data(ctx, "SchoolFiles", nil)
	if err != nil {
		return nil, err
	}
	values, ok := raw["Data"].([]any)
	if !ok {
		return nil, ProtocolError{"Dokumenty szkoły: nieznany format SchoolFiles.Data."}
	}
	out := make([]schoolDocument, 0, len(values))
	seen := map[string]bool{}
	for _, v := range values {
		item, ok := v.(map[string]any)
		if !ok {
			continue
		}
		id := str(item["id"])
		if !schoolIDPattern.MatchString(id) || seen[id] {
			continue
		}
		seen[id] = true
		name := strings.TrimSpace(str(item["displayName"]))
		if name == "" {
			name = "Dokument " + id
		}
		path := parseSchoolDownloadPath(str(item["downloadUrl"]))
		out = append(out, schoolDocument{ID: id, Name: safeSchoolFileName(name), Date: str(item["addedOnDate"]), Status: str(item["fileStatus"]), Downloadable: path != "", Path: path})
	}
	if len(values) > 0 && len(out) == 0 {
		return nil, ProtocolError{"Dokumenty szkoły: brak rozpoznanych identyfikatorów w danych SchoolFiles."}
	}
	return out, nil
}

// Validate the currently selected account's fresh SchoolFiles list, not a
// caller-provided path. In particular, the same file id on another account
// does not let the browser reuse a link across profiles.
func (c *APIClient) verifiedSchoolDocument(ctx context.Context, id string) (schoolDocument, error) {
	if !schoolIDPattern.MatchString(id) {
		return schoolDocument{}, ProtocolError{"Niepoprawny identyfikator dokumentu."}
	}
	docs, err := c.schoolDocuments(ctx)
	if err != nil {
		return schoolDocument{}, err
	}
	for _, doc := range docs {
		if doc.ID == id {
			if !doc.Downloadable {
				return schoolDocument{}, ProtocolError{"Dokument nie ma bezpiecznej ścieżki pobierania."}
			}
			return doc, nil
		}
	}
	return schoolDocument{}, ProtocolError{"Dokument nie istnieje na wybranym koncie."}
}

func schoolSandboxLink(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Port() != "" || !strings.EqualFold(u.Hostname(), "sandbox.librus.pl") {
		return "", ProtocolError{"Dokumenty: zablokowano zewnętrzną domenę pobierania."}
	}
	if u.Path == "/index.php" && u.Query().Get("action") == "CSTryToDownload" && schoolKeyPattern.MatchString(u.Query().Get("singleUseKey")) && len(u.Query()) == 2 {
		return u.String(), nil
	}
	if strings.HasPrefix(u.Path, "/GetFile/") && len(u.Path) > len("/GetFile/") && u.RawQuery == "" && !strings.Contains(u.EscapedPath(), "..") && !strings.Contains(strings.TrimPrefix(u.EscapedPath(), "/GetFile/"), "/") && u.RawPath == "" {
		return u.String(), nil
	}
	return "", ProtocolError{"Dokumenty: nieznany format linku pobierania."}
}

// No unreviewed redirects: the account HTTP client has disabled automatic
// redirects. Every target is validated on every request.
func (c *APIClient) schoolHTTP(ctx context.Context, method, target string, body io.Reader, headers map[string]string, limit int64) (*http.Response, []byte, error) {
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Port() != "" {
		return nil, nil, ProtocolError{"Dokumenty: niedozwolony adres żądania."}
	}
	host := strings.ToLower(u.Hostname())
	if host == "synergia.librus.pl" {
		if method != "GET" || parseSchoolDownloadPath(u.Path) == "" || u.RawQuery != "" {
			return nil, nil, ProtocolError{"Dokumenty: niedozwolona ścieżka Synergii."}
		}
	} else if host == "sandbox.librus.pl" {
		if u.Path == "/index.php" {
			action := u.Query().Get("action")
			switch action {
			case "CSTryToDownload", "CSDownload":
				if method != "GET" || !schoolKeyPattern.MatchString(u.Query().Get("singleUseKey")) || len(u.Query()) != 2 {
					return nil, nil, ProtocolError{"Dokumenty: niepoprawne żądanie pobierania."}
				}
			case "CSCheckKey":
				if method != "POST" || len(u.Query()) != 1 {
					return nil, nil, ProtocolError{"Dokumenty: niepoprawny odczyt statusu."}
				}
			default:
				return nil, nil, ProtocolError{"Dokumenty: nieznany tryb pobierania."}
			}
		} else if strings.HasPrefix(u.Path, "/GetFile/") {
			rawPath := strings.TrimPrefix(u.EscapedPath(), "/GetFile/")
			if method != "GET" || u.RawQuery != "" || u.RawPath != "" || rawPath == "" || strings.Contains(rawPath, "..") || strings.Contains(rawPath, "%") || strings.Count(rawPath, "/") > 1 || (strings.Contains(rawPath, "/") && !strings.HasSuffix(rawPath, "/get")) {
				return nil, nil, ProtocolError{"Dokumenty: niepoprawna ścieżka pobierania."}
			}
		} else {
			return nil, nil, ProtocolError{"Dokumenty: niedozwolona ścieżka sandbox."}
		}
	} else {
		return nil, nil, ProtocolError{"Dokumenty: zablokowano obcy serwer."}
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, nil, ProtocolError{"Dokumenty: błędne żądanie."}
	}
	req.Header.Set("User-Agent", ua)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, nil, ProtocolError{"Dokumenty: błąd połączenia z serwerem plików."}
	}
	defer resp.Body.Close()
	if resp.ContentLength > limit {
		return nil, nil, ProtocolError{"Dokument przekracza limit 20 MiB."}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, nil, ProtocolError{"Dokumenty: błąd odczytu pliku."}
	}
	if int64(len(data)) > limit {
		return nil, nil, ProtocolError{"Dokument przekracza limit 20 MiB."}
	}
	return resp, data, nil
}

func schoolHTML(resp *http.Response, body []byte) bool {
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml") {
		return true
	}
	preview := strings.ToLower(strings.TrimSpace(string(body[:min(len(body), 256)])))
	return strings.HasPrefix(preview, "<html") || strings.HasPrefix(preview, "<!doctype html")
}

// A filename can arrive without an extension (SchoolFiles.displayName is often
// only a display label), and the browser must not infer one from .blob().
// Classify only formats with an explicit MIME type or a reliable file signature.
// Unknown files keep a neutral .bin extension rather than being mislabelled PDF.
func schoolFormatExtension(resp *http.Response, body []byte) string {
	if bytes.HasPrefix(body, []byte("%PDF-")) {
		return ".pdf"
	}
	if len(body) >= 3 && bytes.Equal(body[:3], []byte{0xff, 0xd8, 0xff}) {
		return ".jpg"
	}
	if bytes.HasPrefix(body, []byte("\x89PNG\r\n\x1a\n")) {
		return ".png"
	}
	if bytes.HasPrefix(body, []byte("GIF87a")) || bytes.HasPrefix(body, []byte("GIF89a")) {
		return ".gif"
	}
	if len(body) >= 12 && bytes.Equal(body[:4], []byte("RIFF")) && bytes.Equal(body[8:12], []byte("WEBP")) {
		return ".webp"
	}
	if len(body) >= 4 && bytes.Equal(body[:4], []byte{'P', 'K', 3, 4}) {
		zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		if err == nil {
			var word, excel, slides, contentTypes bool
			for _, f := range zr.File {
				switch f.Name {
				case "[Content_Types].xml":
					contentTypes = true
				case "word/document.xml":
					word = true
				case "xl/workbook.xml":
					excel = true
				case "ppt/presentation.xml":
					slides = true
				}
			}
			if contentTypes {
				if word {
					return ".docx"
				}
				if excel {
					return ".xlsx"
				}
				if slides {
					return ".pptx"
				}
			}
		}
		return ".zip"
	}
	ctype, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		ctype = strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	}
	switch strings.ToLower(ctype) {
	case "application/pdf":
		return ".pdf"
	case "image/jpeg", "image/pjpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	case "image/tiff":
		return ".tif"
	case "text/plain":
		return ".txt"
	case "text/csv", "application/csv":
		return ".csv"
	case "application/json":
		return ".json"
	case "application/rtf", "text/rtf":
		return ".rtf"
	case "application/msword":
		return ".doc"
	case "application/vnd.ms-excel":
		return ".xls"
	case "application/vnd.ms-powerpoint":
		return ".ppt"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ".docx"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return ".xlsx"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return ".pptx"
	case "application/vnd.oasis.opendocument.text":
		return ".odt"
	case "application/vnd.oasis.opendocument.spreadsheet":
		return ".ods"
	case "application/vnd.oasis.opendocument.presentation":
		return ".odp"
	case "application/zip", "application/x-zip-compressed":
		return ".zip"
	case "application/xml", "text/xml":
		return ".xml"
	case "audio/mpeg":
		return ".mp3"
	case "video/mp4":
		return ".mp4"
	}
	return ".bin"
}

// Keep a real existing suffix; only add a format if the name has none or
// ends in a numeric year/version, or replace generic .bin/.dat placeholders.
func schoolNameWithExtension(source, ext string) string {
	name := safeSchoolFileName(source)
	if ext == "" {
		ext = ".bin"
	}
	dot := strings.LastIndexByte(name, '.')
	if dot >= 0 && dot < len(name)-1 {
		suffix := strings.ToLower(name[dot:])
		if suffix == ext {
			return name
		}
		if suffix == ".bin" || suffix == ".dat" {
			return name[:dot] + ext
		}
		valid := len(suffix) >= 2 && len(suffix) <= 11
		hasLetter := false
		for _, r := range suffix[1:] {
			if r >= 'a' && r <= 'z' {
				hasLetter = true
			} else if r < '0' || r > '9' {
				valid = false
			}
		}
		if valid && hasLetter {
			return name
		}
	}
	return name + ext
}

func schoolBinary(resp *http.Response, body []byte, fallback string) (schoolDownload, error) {
	if resp.StatusCode != 200 || len(body) == 0 || schoolHTML(resp, body) {
		return schoolDownload{}, ProtocolError{"Serwer Librusa nie zwrócił gotowego dokumentu."}
	}
	filename := fallback
	if header := resp.Header.Get("Content-Disposition"); header != "" {
		_, params, err := mime.ParseMediaType(header)
		if err == nil && strings.TrimSpace(params["filename"]) != "" {
			filename = params["filename"]
		}
	}
	return schoolDownload{Name: schoolNameWithExtension(filename, schoolFormatExtension(resp, body)), Data: body}, nil
}

// Lock only for the account's metadata verification and initial Synergia
// redirect. Long sandbox polling does not block other tabs on that account.
func (c *APIClient) startSchoolDownload(ctx context.Context, id string) (schoolDocument, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	doc, err := c.verifiedSchoolDocument(ctx, id)
	if err != nil {
		return schoolDocument{}, "", err
	}
	if err = c.ensure(ctx); err != nil {
		return schoolDocument{}, "", err
	}
	target := "https://synergia.librus.pl" + doc.Path
	resp, raw, err := c.schoolHTTP(ctx, "GET", target, nil, nil, 64*1024)
	if err != nil {
		return schoolDocument{}, "", err
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return schoolDocument{}, "", ProtocolError{"Sesja dokumentów wygasła lub brak dostępu. Zaloguj się ponownie."}
	}
	if resp.StatusCode == 200 && resp.Header.Get("Location") == "" && schoolHTML(resp, raw) {
		return schoolDocument{}, "", ProtocolError{"Sesja pobierania plików wygasła. Zaloguj się ponownie."}
	}
	if resp.StatusCode < 300 || resp.StatusCode >= 400 || resp.Header.Get("Location") == "" {
		return schoolDocument{}, "", ProtocolError{"Librus nie zwrócił przekierowania do dokumentu."}
	}
	base, _ := url.Parse(target)
	location, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		return schoolDocument{}, "", ProtocolError{"Librus zwrócił niepoprawny link dokumentu."}
	}
	linked := base.ResolveReference(location)
	link, err := schoolSandboxLink(linked.String())
	if err != nil {
		return schoolDocument{}, "", err
	}
	return doc, link, nil
}

func (c *APIClient) readSchoolSandbox(ctx context.Context, link, filename string, pause func(context.Context, time.Duration) error) (schoolDownload, error) {
	if _, err := schoolSandboxLink(link); err != nil {
		return schoolDownload{}, err
	}
	u, _ := url.Parse(link)
	hdr := map[string]string{"Referer": link}
	resp, _, err := c.schoolHTTP(ctx, "GET", link, nil, hdr, 64*1024)
	if err != nil {
		return schoolDownload{}, err
	}
	if resp.StatusCode != 200 {
		return schoolDownload{}, ProtocolError{"Serwer dokumentów odrzucił przygotowanie pliku."}
	}
	if u.Path == "/index.php" {
		key := u.Query().Get("singleUseKey")
		for attempt := 0; attempt < 15; attempt++ {
			form := url.Values{"singleUseKey": {key}}
			response, data, e := c.schoolHTTP(ctx, "POST", schoolSandbox+"?action=CSCheckKey", strings.NewReader(form.Encode()), map[string]string{"Referer": link, "Content-Type": "application/x-www-form-urlencoded"}, 64*1024)
			if e != nil {
				return schoolDownload{}, e
			}
			if response.StatusCode != 200 {
				return schoolDownload{}, ProtocolError{"Serwer plików odrzucił sprawdzenie statusu."}
			}
			var answer struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(data, &answer) != nil {
				return schoolDownload{}, errSchoolKeyFailed
			}
			switch answer.Status {
			case "ready":
				r, b, e := c.schoolHTTP(ctx, "GET", schoolSandbox+"?action=CSDownload&singleUseKey="+url.QueryEscape(key), nil, hdr, maxSchoolFile)
				if e != nil {
					return schoolDownload{}, e
				}
				return schoolBinary(r, b, filename)
			case "download_failed":
				return schoolDownload{}, errSchoolKeyFailed
			case "not_downloaded_yet":
				if attempt < 14 {
					delay := 10 * time.Second
					if attempt < 3 {
						delay = 3 * time.Second
					}
					if e := pause(ctx, delay); e != nil {
						return schoolDownload{}, ProtocolError{"Przekroczono czas pobierania dokumentu."}
					}
				}
			default:
				return schoolDownload{}, ProtocolError{"Nieznany status pobierania dokumentu."}
			}
		}
	} else {
		for attempt := 0; attempt < 15; attempt++ {
			r, b, e := c.schoolHTTP(ctx, "GET", link+"/get", nil, hdr, maxSchoolFile)
			if e != nil {
				return schoolDownload{}, e
			}
			if r.StatusCode == 200 && !schoolHTML(r, b) {
				return schoolBinary(r, b, filename)
			}
			if r.StatusCode >= 400 && r.StatusCode != 404 {
				return schoolDownload{}, ProtocolError{"Serwer odmówił pobrania dokumentu."}
			}
			if attempt < 14 {
				if e := pause(ctx, 2*time.Second); e != nil {
					return schoolDownload{}, ProtocolError{"Przekroczono czas pobierania dokumentu."}
				}
			}
		}
	}
	return schoolDownload{}, ProtocolError{"Dokument nie jest jeszcze gotowy do pobrania."}
}

func (c *APIClient) downloadSchoolDocument(parent context.Context, id string) (schoolDownload, error) {
	return c.downloadSchoolDocumentWithPause(parent, id, sandboxSleep)
}

// Pause injection lets mock tests exercise retries without real sleeps.
func (c *APIClient) downloadSchoolDocumentWithPause(parent context.Context, id string, pause func(context.Context, time.Duration) error) (schoolDownload, error) {
	ctx, cancel := context.WithTimeout(parent, maxSchoolWait)
	defer cancel()
	for round := 0; round < 3; round++ {
		doc, link, err := c.startSchoolDownload(ctx, id)
		if err != nil {
			return schoolDownload{}, err
		}
		file, err := c.readSchoolSandbox(ctx, link, doc.Name, pause)
		if err == nil {
			return file, nil
		}
		if !errors.Is(err, errSchoolKeyFailed) {
			return schoolDownload{}, err
		}
		if round < 2 {
			if err = pause(ctx, 2*time.Second); err != nil {
				return schoolDownload{}, ProtocolError{"Przekroczono czas pobierania dokumentu."}
			}
		}
	}
	return schoolDownload{}, ProtocolError{"Serwer plików Librusa nie przygotował dokumentu po trzech próbach."}
}

func (app *LocalApp) schoolFilesHandler(w http.ResponseWriter, r *http.Request, download bool) {
	if !app.validRequest(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method", 405)
		return
	}
	index, err := strconv.Atoi(r.URL.Query().Get("profile"))
	if err != nil || index < 0 || index > 1 {
		serveJSON(w, 400, map[string]string{"error": "Nieprawidłowy profil."})
		return
	}
	app.mu.RLock()
	client := app.profiles[index]
	app.mu.RUnlock()
	if client == nil {
		serveJSON(w, 401, map[string]string{"error": "Najpierw zaloguj wybrany profil."})
		return
	}
	if download {
		id := r.URL.Query().Get("file")
		if !schoolIDPattern.MatchString(id) {
			serveJSON(w, 400, map[string]string{"error": "Niepoprawny identyfikator dokumentu."})
			return
		}
		file, err := client.downloadSchoolDocument(r.Context(), id)
		if err != nil {
			serverError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Name}))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Length", fmt.Sprint(len(file.Data)))
		w.WriteHeader(200)
		w.Write(file.Data)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 32*time.Second)
	defer cancel()
	client.mu.Lock()
	docs, err := client.schoolDocuments(ctx)
	client.mu.Unlock()
	if err != nil {
		serverError(w, err)
		return
	}
	serveJSON(w, 200, map[string]any{"items": docs})
}

func installSchoolRoutes(mux *http.ServeMux, app *LocalApp) {
	mux.HandleFunc("/api/school/files", func(w http.ResponseWriter, r *http.Request) { app.schoolFilesHandler(w, r, false) })
	mux.HandleFunc("/api/school/download", func(w http.ResponseWriter, r *http.Request) { app.schoolFilesHandler(w, r, true) })
}
