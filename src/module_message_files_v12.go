package main

// Message attachments are isolated from Homework and SchoolFiles. Membership
// is checked on the selected account before requesting an observed sandbox URL.
import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const messageFileLimitV12 int64 = 20 << 20

func messageSafeNameV12(input string) string {
	input = strings.ReplaceAll(input, "\\", "/")
	name := path.Base(input)
	var out []rune
	for _, r := range name {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"|?*`, r) || r == '\u202d' || r == '\u202e' || (r >= '\u2066' && r <= '\u2069') {
			continue
		}
		out = append(out, r)
		if len(out) == 125 {
			break
		}
	}
	name = strings.Trim(string(out), " .")
	if name == "" {
		name = "zalacznik"
	}
	root := strings.ToUpper(strings.Split(name, ".")[0])
	if regexp.MustCompile(`^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])$`).MatchString(root) {
		name = "plik_" + name
	}
	return name
}
func messageFileNameV12(name, disposition, typ string, data []byte) string {
	if _, params, e := mime.ParseMediaType(disposition); e == nil && params["filename"] != "" {
		name = params["filename"]
	}
	name = messageSafeNameV12(name)
	if regexp.MustCompile(`(?i)\.[a-z][a-z0-9]{0,9}$`).MatchString(name) {
		return name
	}
	ext := ""
	switch {
	case bytes.HasPrefix(data, []byte("%PDF-")):
		ext = ".pdf"
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		ext = ".png"
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		ext = ".jpg"
	case bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")):
		ext = ".gif"
	case len(data) > 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		ext = ".webp"
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		ext = ".zip"
		if z, e := zip.NewReader(bytes.NewReader(data), int64(len(data))); e == nil {
			for _, f := range z.File {
				if strings.HasPrefix(f.Name, "word/") {
					ext = ".docx"
					break
				}
				if strings.HasPrefix(f.Name, "xl/") {
					ext = ".xlsx"
					break
				}
				if strings.HasPrefix(f.Name, "ppt/") {
					ext = ".pptx"
					break
				}
			}
		}
	}
	if ext == "" {
		ext = map[string]string{"application/pdf": ".pdf", "image/jpeg": ".jpg", "image/png": ".png", "text/plain": ".txt", "application/zip": ".zip", "application/msword": ".doc", "application/vnd.ms-excel": ".xls", "application/vnd.ms-powerpoint": ".ppt"}[strings.Split(typ, ";")[0]]
	}
	if ext == "" {
		ext = ".bin"
	}
	return name + ext
}
func messageSandboxV12(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "sandbox.librus.pl" || u.User != nil || u.Fragment != "" || strings.ContainsAny(u.EscapedPath(), "%\\") || strings.Contains(u.Path, "..") {
		return nil, ProtocolError{"Załącznik: niedozwolony serwer plików."}
	}
	if regexp.MustCompile(`^/GetFile/[A-Za-z0-9_-]+(?:/[A-Za-z0-9_-]+)*$`).MatchString(u.Path) && u.RawQuery == "" {
		return u, nil
	}
	q := u.Query()
	if u.Path == "/index.php" && len(q) == 2 && q.Get("action") == "CSTryToDownload" && regexp.MustCompile(`^[A-Za-z0-9_-]{8,200}$`).MatchString(q.Get("singleUseKey")) {
		return u, nil
	}
	return nil, ProtocolError{"Załącznik: nieznany format linku pobierania."}
}
func messageFileHTTPV12(ctx context.Context, client *http.Client, method, target string, body io.Reader, referer string, limit int64) (*http.Response, []byte, error) {
	u, e := url.Parse(target)
	if e != nil || u.Scheme != "https" || u.Host != "sandbox.librus.pl" || u.User != nil || u.Fragment != "" {
		return nil, nil, ProtocolError{"Załącznik: niedozwolony adres pobierania."}
	}
	req, e := http.NewRequestWithContext(ctx, method, target, body)
	if e != nil {
		return nil, nil, ProtocolError{"Nieprawidłowe żądanie pliku."}
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", referer)
	if method == "POST" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, e := client.Do(req)
	if e != nil {
		return nil, nil, ProtocolError{"Nie udało się połączyć z serwerem plików."}
	}
	defer resp.Body.Close()
	if resp.ContentLength > limit {
		return nil, nil, ProtocolError{"Załącznik przekracza limit 20 MiB."}
	}
	data, e := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if e != nil || int64(len(data)) > limit {
		return nil, nil, ProtocolError{"Nie można pobrać pliku lub przekroczono limit."}
	}
	return resp, data, nil
}
func messagePauseV12(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func messageHTMLV12(resp *http.Response, data []byte) bool {
	typ := strings.ToLower(resp.Header.Get("Content-Type"))
	prefix := strings.ToLower(strings.TrimSpace(string(data[:min(len(data), 512)])))
	return strings.Contains(typ, "html") || strings.HasPrefix(prefix, "<!doctype html") || strings.HasPrefix(prefix, "<html") || strings.HasPrefix(prefix, "<script")
}
func messageSandboxFileV12(ctx context.Context, client *http.Client, link string, pause func(context.Context, time.Duration) error) (*http.Response, []byte, error) {
	u, e := messageSandboxV12(link)
	if e != nil {
		return nil, nil, e
	}
	r, raw, e := messageFileHTTPV12(ctx, client, "GET", link, nil, link, messageFileLimitV12)
	if e != nil {
		return nil, nil, e
	}
	if r.StatusCode == 200 && len(raw) > 0 && !messageHTMLV12(r, raw) {
		return r, raw, nil
	}
	if r.StatusCode != 200 {
		return nil, nil, ProtocolError{"Serwer plików odmówił przygotowania załącznika."}
	}
	key := u.Query().Get("singleUseKey")
	for i := 0; i < 15; i++ {
		target := link + "/get"
		if key != "" {
			form := url.Values{"singleUseKey": {key}}
			rr, b, err := messageFileHTTPV12(ctx, client, "POST", "https://sandbox.librus.pl/index.php?action=CSCheckKey", strings.NewReader(form.Encode()), link, 64<<10)
			if err != nil {
				return nil, nil, err
			}
			var state struct {
				Status string `json:"status"`
			}
			if rr.StatusCode != 200 || json.Unmarshal(b, &state) != nil {
				return nil, nil, ProtocolError{"Niepoprawna odpowiedź przygotowania załącznika."}
			}
			if state.Status == "download_failed" {
				return nil, nil, ProtocolError{"Serwer plików nie przygotował załącznika. Spróbuj ponownie później."}
			}
			if state.Status == "not_downloaded_yet" {
				if pause(ctx, 3*time.Second) != nil {
					return nil, nil, ProtocolError{"Przekroczono czas pobierania."}
				}
				continue
			}
			if state.Status != "ready" {
				return nil, nil, ProtocolError{"Nieznany stan serwera plików."}
			}
			target = "https://sandbox.librus.pl/index.php?action=CSDownload&singleUseKey=" + url.QueryEscape(key)
		}
		r, raw, e = messageFileHTTPV12(ctx, client, "GET", target, nil, link, messageFileLimitV12)
		if e != nil {
			return nil, nil, e
		}
		if r.StatusCode == 200 && len(raw) > 0 && !messageHTMLV12(r, raw) {
			return r, raw, nil
		}
		if r.StatusCode != 200 && r.StatusCode != 404 {
			return nil, nil, ProtocolError{"Serwer plików odmówił pobrania."}
		}
		if key != "" {
			return nil, nil, ProtocolError{"Zamiast pliku otrzymano stronę oczekiwania lub logowania."}
		}
		if pause(ctx, 2*time.Second) != nil {
			return nil, nil, ProtocolError{"Przekroczono czas pobierania."}
		}
	}
	return nil, nil, ProtocolError{"Załącznik nie jest jeszcze gotowy."}
}
func (s *messagesServiceV12) download(w http.ResponseWriter, r *http.Request) {
	c, idx, ok := s.profile(w, r, "POST")
	if !ok {
		return
	}
	q := r.URL.Query()
	id, file, box := q.Get("id"), q.Get("file"), q.Get("box")
	if !messageIDV12(file) || !messageIDV12(id) || !messageBoxV12(box) {
		serveJSON(w, 400, map[string]string{"error": "Nieprawidłowe identyfikatory załącznika."})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 150*time.Second)
	defer cancel()
	// The shared profile lock is released before slow sandbox polling.
	c.mu.Lock()
	full, err := c.messageOpenV12(ctx, box, id)
	name := ""
	link := ""
	if err == nil {
		for _, f := range full.Attachments {
			if f.ID == file {
				name = f.Name
				break
			}
		}
		if name == "" {
			err = ProtocolError{"Załącznik nie należy do tej wiadomości w wybranym profilu."}
		}
	}
	if err == nil {
		var payload any
		payload, err = c.messageGet(ctx, "attachments/"+file+"/messages/"+id, nil)
		if err == nil {
			link = str(obj(obj(payload)["data"])["downloadLink"])
			_, err = messageSandboxV12(link)
		}
	}
	client := *c.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c.mu.Unlock()
	if err != nil {
		serverError(w, err)
		return
	}
	if !s.current(c, idx) {
		serveJSON(w, 409, map[string]string{"error": "Zmieniono sesję profilu."})
		return
	}
	response, data, err := messageSandboxFileV12(ctx, &client, link, messagePauseV12)
	if err != nil {
		serverError(w, err)
		return
	}
	if !s.current(c, idx) {
		serveJSON(w, 409, map[string]string{"error": "Zmieniono sesję profilu podczas pobierania."})
		return
	}
	name = messageFileNameV12(name, response.Header.Get("Content-Disposition"), response.Header.Get("Content-Type"), data)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	w.Write(data)
}
