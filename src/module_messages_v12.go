package main

// v0.12: independent Wiadomosci operations. Uses the shared HTTPS/session
// transport only; never calls another feature module or changes v0.5 reads.
import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type messageRowV12 struct {
	ID             string `json:"id"`
	Date           string `json:"date"`
	Sender         string `json:"sender"`
	Receiver       string `json:"receiver"`
	Subject        string `json:"subject"`
	Preview        string `json:"preview"`
	Unread         bool   `json:"unread"`
	HasAttachments bool   `json:"hasAttachments"`
}
type messageAttachmentV12 struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type messageFullV12 struct {
	ID          string                 `json:"id"`
	Box         string                 `json:"box"`
	Sender      string                 `json:"sender"`
	SenderID    string                 `json:"senderId"`
	Subject     string                 `json:"subject"`
	Content     string                 `json:"content"`
	Date        string                 `json:"date"`
	NoReply     bool                   `json:"noReply"`
	Attachments []messageAttachmentV12 `json:"attachments"`
}
type messageRecipientV12 struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group"`
}

func messageIDV12(id string) bool {
	return len(id) > 0 && len(id) <= 18 && idPattern.MatchString(id) && strings.TrimLeft(id, "0") != ""
}
func messageBoxV12(box string) bool { return box == "inbox" || box == "outbox" }
func messageFlagV12(value any) bool {
	return value == true || str(value) == "1" || strings.EqualFold(str(value), "true")
}
func messageNameV12(m map[string]any, prefix string) string {
	if name := strings.TrimSpace(str(m[prefix+"Name"])); name != "" {
		return name
	}
	return strings.TrimSpace(str(m[prefix+"FirstName"]) + " " + str(m[prefix+"LastName"]))
}
func messageTextV12(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || !utf8.Valid(decoded) {
		return "", ProtocolError{"Wiadomość ma niepoprawnie zakodowaną treść."}
	}
	text := string(decoded)
	if strings.HasPrefix(strings.TrimSpace(text), "<Message") {
		var shell struct {
			Content string `xml:"Content"`
		}
		if xml.Unmarshal(decoded, &shell) != nil {
			return "", ProtocolError{"Niepoprawna struktura treści wiadomości."}
		}
		text = shell.Content
	}
	text = brTag.ReplaceAllString(text, "\n")
	text = regexp.MustCompile(`(?i)</(?:p|div|li)>`).ReplaceAllString(text, "\n")
	return strings.TrimSpace(html.UnescapeString(htmlTag.ReplaceAllString(text, ""))), nil
}
func (c *APIClient) messagesListV12(ctx context.Context, box string) ([]messageRowV12, error) {
	if !messageBoxV12(box) {
		return nil, ProtocolError{"Nieprawidłowa skrzynka."}
	}
	raw, err := c.messageGet(ctx, box+"/messages", url.Values{"limit": {"30"}})
	if err != nil {
		return nil, err
	}
	rows, ok := raw.([]any)
	if !ok {
		rows, ok = obj(raw)["data"].([]any)
	}
	if !ok {
		return nil, ProtocolError{"Wiadomości: brak poprawnej listy data."}
	}
	out := make([]messageRowV12, 0, len(rows))
	for _, v := range rows {
		m, ok := v.(map[string]any)
		if !ok || !messageIDV12(str(m["messageId"])) {
			return nil, ProtocolError{"Wiadomości: niepoprawny wpis listy."}
		}
		out = append(out, messageRowV12{ID: str(m["messageId"]), Date: str(m["sendDate"]), Sender: messageNameV12(m, "sender"), Receiver: messageNameV12(m, "receiver"), Subject: str(m["topic"]), Preview: decodePreview(str(m["content"])), Unread: m["readDate"] == nil, HasAttachments: messageFlagV12(m["isAnyFileAttached"])})
	}
	return out, nil
}
func (c *APIClient) messageOpenV12(ctx context.Context, box, id string) (messageFullV12, error) {
	if !messageBoxV12(box) || !messageIDV12(id) {
		return messageFullV12{}, ProtocolError{"Nieprawidłowa skrzynka lub identyfikator wiadomości."}
	}
	raw, err := c.messageGet(ctx, box+"/messages/"+id, nil)
	if err != nil {
		return messageFullV12{}, err
	}
	m, ok := obj(raw)["data"].(map[string]any)
	if !ok {
		return messageFullV12{}, ProtocolError{"Wiadomości: niepoprawna pełna wiadomość."}
	}
	if val := str(m["messageId"]); val != "" && val != id {
		return messageFullV12{}, ProtocolError{"Wiadomości: odpowiedź dotyczy innej wiadomości."}
	}
	content, ok := m["Message"].(string)
	if !ok {
		return messageFullV12{}, ProtocolError{"Wiadomości: brak pełnej treści Message."}
	}
	text, err := messageTextV12(content)
	if err != nil {
		return messageFullV12{}, err
	}
	files := []messageAttachmentV12{}
	if m["attachments"] != nil {
		list, ok := m["attachments"].([]any)
		if !ok {
			return messageFullV12{}, ProtocolError{"Wiadomości: niepoprawna lista załączników."}
		}
		seen := map[string]bool{}
		for _, v := range list {
			f := obj(v)
			fid := str(f["id"])
			if !messageIDV12(fid) {
				return messageFullV12{}, ProtocolError{"Wiadomości: niepoprawny identyfikator załącznika."}
			}
			if !seen[fid] {
				seen[fid] = true
				files = append(files, messageAttachmentV12{fid, messageSafeNameV12(str(f["filename"]))})
			}
		}
	}
	return messageFullV12{ID: id, Box: box, Sender: messageNameV12(m, "sender"), SenderID: str(m["senderId"]), Subject: str(m["topic"]), Content: text, Date: str(m["sendDate"]), NoReply: messageFlagV12(m["noReply"]), Attachments: files}, nil
}
func (c *APIClient) messageRecipientsV12(ctx context.Context) ([]messageRecipientV12, error) {
	raw, err := c.messageGet(ctx, "receivers/types", url.Values{"includeClass": {"true"}})
	if err != nil {
		return nil, err
	}
	groups, ok := obj(obj(raw)["data"])["list"].([]any)
	if !ok {
		return nil, ProtocolError{"Wiadomości: niepoprawna lista grup odbiorców."}
	}
	out := []messageRecipientV12{}
	seen := map[string]bool{}
	for _, v := range groups {
		g := obj(v)
		code := str(g["id"])
		if !groupPattern.MatchString(code) || len(code) > 60 || code == "parents" || code == "students" || code == "class" || code == "all" || code == "parentsCouncil" || code == "schoolParentsCouncil" || code == "customGroups" || code == "sadmin" {
			continue
		}
		raw, err := c.messageGet(ctx, "receivers/groups/school-employees", url.Values{"receiverType": {code}})
		if err != nil {
			if strings.Contains(err.Error(), "HTTP 403") || strings.Contains(err.Error(), "HTTP 404") {
				continue
			}
			return nil, err
		}
		people, ok := obj(raw)["receivers"].([]any)
		if !ok {
			return nil, ProtocolError{"Wiadomości: niepoprawna grupa odbiorców."}
		}
		for _, p := range people {
			m := obj(p)
			id := str(m["accountId"])
			if !messageIDV12(id) {
				return nil, ProtocolError{"Wiadomości: niepoprawne konto odbiorcy."}
			}
			if !seen[id] {
				seen[id] = true
				out = append(out, messageRecipientV12{id, str(m["label"]), str(g["name"])})
			}
		}
	}
	return out, nil
}

type messageSendInputV12 struct {
	Recipients []string `json:"recipients"`
	Topic      string   `json:"topic"`
	Text       string   `json:"text"`
	ReplyTo    string   `json:"replyTo"`
	RequestID  string   `json:"requestId"`
}
type messageSendResultV12 struct {
	State   string `json:"state"`
	Message string `json:"message"`
	ID      string `json:"id,omitempty"`
}
type messageAttemptV12 struct {
	Hash   string
	At     time.Time
	Result messageSendResultV12
	Done   bool
}
type messagesServiceV12 struct {
	app      *LocalApp
	mu       sync.Mutex
	attempts map[string]*messageAttemptV12
}

func validMessageSendV12(in messageSendInputV12) error {
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{20,90}$`).MatchString(in.RequestID) {
		return ProtocolError{"Nieprawidłowy identyfikator próby wysyłki."}
	}
	if !utf8.ValidString(in.Topic) || !utf8.ValidString(in.Text) || strings.TrimSpace(in.Topic) == "" || utf8.RuneCountInString(in.Topic) > 200 || strings.TrimSpace(in.Text) == "" || utf8.RuneCountInString(in.Text) > 10000 {
		return ProtocolError{"Podaj temat (do 200 znaków) i treść (do 10000 znaków)."}
	}
	if in.ReplyTo != "" {
		if !messageIDV12(in.ReplyTo) {
			return ProtocolError{"Nieprawidłowa wiadomość źródłowa odpowiedzi."}
		}
		return nil
	}
	if len(in.Recipients) < 1 || len(in.Recipients) > 50 {
		return ProtocolError{"Wybierz od 1 do 50 odbiorców."}
	}
	seen := map[string]bool{}
	for _, id := range in.Recipients {
		if !messageIDV12(id) || seen[id] {
			return ProtocolError{"Nieprawidłowa lub powtórzona osoba na liście odbiorców."}
		}
		seen[id] = true
	}
	return nil
}
func (c *APIClient) sendMessageV12(ctx context.Context, in messageSendInputV12, guards ...func() bool) messageSendResultV12 {
	refused := func(text string) messageSendResultV12 { return messageSendResultV12{State: "not_sent", Message: text} }
	unknown := messageSendResultV12{State: "unknown", Message: "Brak pewnego potwierdzenia. Wiadomość mogła zostać wysłana. Sprawdź Wysłane przed ponowieniem."}
	// All authentication recovery happens on reads BEFORE the only POST.
	if _, err := c.messageGet(ctx, "me", nil); err != nil {
		return refused("Nie wysłano: nie udało się sprawdzić sesji Wiadomości.")
	}
	ids := in.Recipients
	endpoint := "messages"
	if in.ReplyTo != "" {
		original, err := c.messageOpenV12(ctx, "inbox", in.ReplyTo)
		if err != nil {
			return refused("Nie wysłano: nie udało się zweryfikować oryginalnej wiadomości.")
		}
		if original.NoReply || !messageIDV12(original.SenderID) {
			return refused("Ta wiadomość nie pozwala na odpowiedź.")
		}
		ids = []string{original.SenderID}
		endpoint = "messages/reply"
	} else {
		contacts, err := c.messageRecipientsV12(ctx)
		if err != nil {
			return refused("Nie wysłano: nie udało się sprawdzić odbiorców.")
		}
		available := map[string]bool{}
		for _, p := range contacts {
			available[p.ID] = true
		}
		for _, id := range ids {
			if !available[id] {
				return refused("Nie wysłano: odbiorca nie należy do dostępnych kontaktów tego profilu.")
			}
		}
	}
	receivers := []map[string]string{}
	for _, id := range ids {
		receivers = append(receivers, map[string]string{"accountId": id})
	}
	plain := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(strings.ReplaceAll(in.Text, "\r\n", "\n"))
	payload := map[string]any{"receivers": map[string]any{"schoolReceivers": receivers}, "topic": base64.StdEncoding.EncodeToString([]byte(in.Topic)), "content": base64.StdEncoding.EncodeToString([]byte(plain)), "storageId": nil, "category": "normal"}
	if in.ReplyTo != "" {
		id, _ := strconv.ParseInt(in.ReplyTo, 10, 64)
		payload["originalMessageId"] = id
	}
	body, _ := json.Marshal(payload)
	for _, current := range guards {
		if current != nil && !current() {
			return refused("Nie wysłano: podczas przygotowania zmieniono sesję profilu.")
		}
	}
	resp, raw, err := c.request(ctx, "POST", msgBase+"/"+endpoint, strings.NewReader(string(body)), map[string]string{"Content-Type": "application/json", "Origin": "https://wiadomosci.librus.pl", "Referer": "https://wiadomosci.librus.pl/"})
	if err != nil {
		return unknown
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 500 && resp.StatusCode != 408 {
		return refused(fmt.Sprintf("Librus odrzucił wysyłkę (HTTP %d).", resp.StatusCode))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return unknown
	}
	answer := map[string]any{}
	if len(strings.TrimSpace(string(raw))) > 0 && json.Unmarshal(raw, &answer) != nil {
		return unknown
	}
	if e := answer["errors"]; e != nil && str(e) != "[]" && str(e) != "map[]" && str(e) != "" {
		return unknown
	}
	status := strings.ToLower(str(obj(answer["data"])["status"]))
	if status == "" {
		status = strings.ToLower(str(answer["status"]))
	}
	if status == "error" || status == "failed" || status == "rejected" {
		return unknown
	}
	id := str(obj(answer["data"])["messageId"])
	if !messageIDV12(id) {
		id = ""
	}
	return messageSendResultV12{State: "sent", Message: "Librus przyjął wiadomość.", ID: id}
}

func (s *messagesServiceV12) profile(w http.ResponseWriter, r *http.Request, method string) (*APIClient, int, bool) {
	if !s.app.validRequest(w, r) {
		return nil, 0, false
	}
	if r.Method != method {
		http.Error(w, "Method", 405)
		return nil, 0, false
	}
	idx, err := strconv.Atoi(r.URL.Query().Get("profile"))
	if err != nil || idx < 0 || idx > 1 {
		serveJSON(w, 400, map[string]string{"error": "Nieprawidłowy profil."})
		return nil, 0, false
	}
	s.app.mu.RLock()
	c := s.app.profiles[idx]
	s.app.mu.RUnlock()
	if c == nil {
		serveJSON(w, 401, map[string]string{"error": "Najpierw zaloguj wybrany profil."})
		return nil, 0, false
	}
	return c, idx, true
}
func (s *messagesServiceV12) current(c *APIClient, idx int) bool {
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	return s.app.profiles[idx] == c
}
func (s *messagesServiceV12) read(w http.ResponseWriter, r *http.Request, full bool) {
	method := "GET"
	if full {
		method = "POST"
	}
	c, idx, ok := s.profile(w, r, method)
	if !ok {
		return
	}
	box, id := r.URL.Query().Get("box"), r.URL.Query().Get("id")
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	c.mu.Lock()
	var result any
	var err error
	if full {
		result, err = c.messageOpenV12(ctx, box, id)
	} else {
		result, err = c.messagesListV12(ctx, box)
	}
	c.mu.Unlock()
	if !s.current(c, idx) {
		serveJSON(w, 409, map[string]string{"error": "Zmieniono sesję profilu. Otwórz moduł ponownie."})
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	serveJSON(w, 200, map[string]any{"items": result})
}
func (s *messagesServiceV12) recipients(w http.ResponseWriter, r *http.Request) {
	c, idx, ok := s.profile(w, r, "GET")
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	c.mu.Lock()
	rows, err := c.messageRecipientsV12(ctx)
	c.mu.Unlock()
	if !s.current(c, idx) {
		serveJSON(w, 409, map[string]string{"error": "Zmieniono sesję profilu."})
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	serveJSON(w, 200, map[string]any{"items": rows})
}
func (s *messagesServiceV12) send(w http.ResponseWriter, r *http.Request) {
	c, idx, ok := s.profile(w, r, "POST")
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	var in messageSendInputV12
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil || dec.Decode(&struct{}{}) != io.EOF {
		serveJSON(w, 400, map[string]string{"error": "Nieprawidłowy formularz wiadomości."})
		return
	}
	if err := validMessageSendV12(in); err != nil {
		serveJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	data, _ := json.Marshal(in)
	digest := sha256.Sum256(data)
	hash := hex.EncodeToString(digest[:])
	key := fmt.Sprintf("%p/%d/%s", c, idx, in.RequestID)
	s.mu.Lock()
	for k, v := range s.attempts {
		if time.Since(v.At) > 24*time.Hour && v.Done {
			delete(s.attempts, k)
		}
	}
	if prev := s.attempts[key]; prev != nil {
		previousHash, done, result := prev.Hash, prev.Done, prev.Result
		s.mu.Unlock()
		if previousHash != hash {
			serveJSON(w, 409, map[string]string{"error": "Identyfikator wysyłki dotyczy innej treści."})
			return
		}
		if !done {
			serveJSON(w, 409, map[string]string{"error": "Ta próba wysyłki już trwa."})
			return
		}
		serveJSON(w, 200, result)
		return
	}
	if len(s.attempts) >= 2000 {
		s.mu.Unlock()
		serveJSON(w, 429, map[string]string{"error": "Limit prób w tej sesji programu. Sprawdź Wysłane."})
		return
	}
	attempt := &messageAttemptV12{Hash: hash, At: time.Now()}
	s.attempts[key] = attempt
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	c.mu.Lock()
	result := messageSendResultV12{State: "not_sent", Message: "Zmieniono sesję profilu. Nie wysłano."}
	if s.current(c, idx) {
		result = c.sendMessageV12(ctx, in, func() bool { return s.current(c, idx) })
	}
	c.mu.Unlock()
	s.mu.Lock()
	attempt.Result = result
	attempt.Done = true
	s.mu.Unlock()
	serveJSON(w, 200, result)
}
func installMessagesRoutesV12(mux *http.ServeMux, app *LocalApp) {
	s := &messagesServiceV12{app: app, attempts: map[string]*messageAttemptV12{}}
	mux.HandleFunc("/api/messages/list", func(w http.ResponseWriter, r *http.Request) { s.read(w, r, false) })
	mux.HandleFunc("/api/messages/open", func(w http.ResponseWriter, r *http.Request) { s.read(w, r, true) })
	mux.HandleFunc("/api/messages/recipients", s.recipients)
	mux.HandleFunc("/api/messages/send", s.send)
	mux.HandleFunc("/api/messages/download", s.download)
}
