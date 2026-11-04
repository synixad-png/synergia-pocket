package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const attemptIDV12 = "TEST_ATTEMPT_V12_00000001"

func fullPayloadV12(sender string, noReply bool, files []messageAttachmentV12) string {
	list := []map[string]string{}
	for _, f := range files {
		list = append(list, map[string]string{"id": f.ID, "filename": f.Name})
	}
	b, _ := json.Marshal(map[string]any{"data": map[string]any{"messageId": 42, "senderName": "Nauczyciel testowy", "senderId": sender, "topic": "Test <img src=x>", "Message": base64.StdEncoding.EncodeToString([]byte("<Message><Content><![CDATA[Pierwszy akapit<br>Drugi &amp; trzeci]]></Content></Message>")), "attachments": list, "noReply": noReply}})
	return string(b)
}
func prepareSendV12(c *APIClient, m *mockedHTTP) {
	c.messagesReady = true
	m.add("GET", msgBase+"/me", record{200, `{"data":{"accountId":"123"}}`, "", ""})
	m.add("GET", msgBase+"/receivers/types?includeClass=true", record{200, `{"data":{"list":[{"id":"teachers","name":"Nauczyciele"}]}}`, "", ""})
	m.add("GET", msgBase+"/receivers/groups/school-employees?receiverType=teachers", record{200, `{"receivers":[{"accountId":"70","label":"Nauczyciel testowy"}]}`, "", ""})
}
func inputV12() messageSendInputV12 {
	return messageSendInputV12{Recipients: []string{"70"}, Topic: "Ważny temat", Text: "A < B & C\nDrugi akapit", RequestID: attemptIDV12}
}
func localMessageRequestV12(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:3123"+path, strings.NewReader(body))
	r.AddCookie(&http.Cookie{Name: "synergia_local", Value: "private"})
	if method == "POST" {
		r.Header.Set("Origin", "http://127.0.0.1:3123")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}
func TestV12ListKeepsIDsAndDoesNotOpenMessages(t *testing.T) {
	c, m := fakeAccount()
	c.messagesReady = true
	m.add("GET", msgBase+"/inbox/messages?limit=30", record{200, `{"data":[{"messageId":42,"topic":"Temat","senderName":"Szkoła","readDate":null,"isAnyFileAttached":1}]}`, "", ""})
	rows, err := c.messagesListV12(context.Background(), "inbox")
	if err != nil || len(rows) != 1 || rows[0].ID != "42" || !rows[0].Unread || !rows[0].HasAttachments || len(m.calls) != 1 {
		t.Fatal(rows, err, m.calls)
	}
	m.add("GET", msgBase+"/outbox/messages?limit=30", record{200, `[{"messageId":"43","receiverFirstName":"Anna","receiverLastName":"Testowa"}]`, "", ""})
	out, err := c.messagesListV12(context.Background(), "outbox")
	if err != nil || len(out) != 1 || out[0].Receiver != "Anna Testowa" {
		t.Fatal(out, err)
	}
}
func TestV12FullContentCDATAFilesAndNoReply(t *testing.T) {
	c, m := fakeAccount()
	c.messagesReady = true
	m.add("GET", msgBase+"/inbox/messages/42", record{200, fullPayloadV12("70", true, []messageAttachmentV12{{"9", "Plan.pdf"}}), "", ""})
	got, err := c.messageOpenV12(context.Background(), "inbox", "42")
	if err != nil || got.Content != "Pierwszy akapit\nDrugi & trzeci" || !got.NoReply || got.Attachments[0].ID != "9" {
		t.Fatal(got, err)
	}
}
func TestV12MalformedFullBodiesAndMessageIDsAreErrors(t *testing.T) {
	for _, body := range []string{`{"data":[]}`, `{"data":{"Message":false}}`, `{"data":{"Message":"!bad"}}`, `{"data":{"Message":"","messageId":99}}`, `{"data":{"Message":"","attachments":{}}}`} {
		c, m := fakeAccount()
		c.messagesReady = true
		m.add("GET", msgBase+"/inbox/messages/42", record{200, body, "", ""})
		if _, err := c.messageOpenV12(context.Background(), "inbox", "42"); err == nil {
			t.Fatal("bad response accepted", body)
		}
	}
}
func TestV12BadIDsAndMailboxNeverReachNetwork(t *testing.T) {
	c, m := fakeAccount()
	for _, box := range []string{"../inbox", "https://evil.example", "trash"} {
		if _, err := c.messagesListV12(context.Background(), box); err == nil {
			t.Fatal(box)
		}
	}
	for _, id := range []string{"../42", "42?other=9", "-1", "1e6", ""} {
		if _, err := c.messageOpenV12(context.Background(), "inbox", id); err == nil {
			t.Fatal(id)
		}
	}
	if len(m.calls) != 0 {
		t.Fatal(m.calls)
	}
}
func TestV12OpenRequiresExplicitPostAndLocalOrigin(t *testing.T) {
	c, m := fakeAccount()
	app := &LocalApp{token: "private", origin: "http://127.0.0.1:3123"}
	app.profiles[0] = c
	h := app.routes()
	if w := localMessageRequestV12(h, "GET", "/api/messages/open?profile=0&box=inbox&id=42", ""); w.Code != 405 {
		t.Fatal(w.Code)
	}
	r := httptest.NewRequest("POST", "http://127.0.0.1:3123/api/messages/open?profile=0&box=inbox&id=42", nil)
	r.AddCookie(&http.Cookie{Name: "synergia_local", Value: "private"})
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 || len(m.calls) != 0 {
		t.Fatal(w.Code, m.calls)
	}
}

type messageRoundTripV12 func(*http.Request) (*http.Response, error)

func (f messageRoundTripV12) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestV12SendPayloadPlainUTF8AndOnlyOnePost(t *testing.T) {
	c, m := fakeAccount()
	prepareSendV12(c, m)
	var posts int
	c.client.Transport = messageRoundTripV12(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" {
			return m.RoundTrip(r)
		}
		posts++
		if r.URL.String() != msgBase+"/messages" {
			t.Fatal(r.URL)
		}
		var p map[string]any
		if json.NewDecoder(r.Body).Decode(&p) != nil {
			t.Fatal("bad JSON")
		}
		topic, _ := base64.StdEncoding.DecodeString(str(p["topic"]))
		text, _ := base64.StdEncoding.DecodeString(str(p["content"]))
		if string(topic) != "Ważny temat" || string(text) != "A &lt; B &amp; C\nDrugi akapit" || p["storageId"] != nil {
			t.Fatal(p, string(text))
		}
		if r.Header.Get("Origin") != "https://wiadomosci.librus.pl" {
			t.Fatal(r.Header)
		}
		return &http.Response{StatusCode: 201, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"data":{"status":"sent","messageId":88}}`)), Request: r}, nil
	})
	result := c.sendMessageV12(context.Background(), inputV12())
	if posts != 1 || result.State != "sent" || result.ID != "88" {
		t.Fatal(result, posts)
	}
}
func TestV12SendFailureStatesNeverRetryPost(t *testing.T) {
	for _, tc := range []struct {
		code        int
		body, state string
	}{{500, `error`, "unknown"}, {422, `bad`, "not_sent"}, {401, `no`, "not_sent"}, {302, ``, "not_sent"}, {408, ``, "unknown"}, {200, `{"errors":["failed"]}`, "unknown"}, {200, `{"data":{"status":"rejected"}}`, "unknown"}, {204, ``, "sent"}, {200, `{}`, "sent"}, {200, `not JSON`, "unknown"}} {
		c, m := fakeAccount()
		prepareSendV12(c, m)
		m.add("POST", msgBase+"/messages", record{tc.code, tc.body, "", ""})
		got := c.sendMessageV12(context.Background(), inputV12())
		count := 0
		for _, call := range m.calls {
			if strings.HasPrefix(call, "POST ") {
				count++
			}
		}
		if got.State != tc.state || count != 1 {
			t.Fatal(tc, got, m.calls)
		}
	}
}
func TestV12ReplyUsesOriginalSenderAndRespectsNoReply(t *testing.T) {
	for _, noReply := range []bool{true, false} {
		c, m := fakeAccount()
		c.messagesReady = true
		m.add("GET", msgBase+"/me", record{200, `{}`, "", ""})
		m.add("GET", msgBase+"/inbox/messages/42", record{200, fullPayloadV12("70", noReply, nil), "", ""})
		in := inputV12()
		in.ReplyTo = "42"
		in.Recipients = []string{"999"}
		if !noReply {
			c.client.Transport = messageRoundTripV12(func(r *http.Request) (*http.Response, error) {
				if r.Method != "POST" {
					return m.RoundTrip(r)
				}
				var p map[string]any
				json.NewDecoder(r.Body).Decode(&p)
				if str(p["originalMessageId"]) != "42" || str(obj(arr(obj(p["receivers"])["schoolReceivers"])[0])["accountId"]) != "70" {
					t.Fatal(p)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`)), Request: r}, nil
			})
		}
		got := c.sendMessageV12(context.Background(), in)
		want := "sent"
		if noReply {
			want = "not_sent"
		}
		if got.State != want {
			t.Fatal(got, m.calls)
		}
	}
}
func TestV12NoWriteToUnlistedRecipientOrBeforeValidSession(t *testing.T) {
	c, m := fakeAccount()
	prepareSendV12(c, m)
	in := inputV12()
	in.Recipients = []string{"999"}
	if got := c.sendMessageV12(context.Background(), in); got.State != "not_sent" {
		t.Fatal(got)
	}
	for _, call := range m.calls {
		if strings.HasPrefix(call, "POST ") {
			t.Fatal(m.calls)
		}
	}
	c, m = fakeAccount()
	c.messagesReady = true
	m.add("GET", msgBase+"/me", record{503, `down`, "", ""})
	if c.sendMessageV12(context.Background(), inputV12()).State != "not_sent" || len(m.calls) != 1 {
		t.Fatal(m.calls)
	}
}
func TestV12SendLedgerDeduplicatesUnknownAndRejectsChangedPayload(t *testing.T) {
	c, m := fakeAccount()
	prepareSendV12(c, m)
	posts := 0
	c.client.Transport = messageRoundTripV12(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" {
			return m.RoundTrip(r)
		}
		posts++
		return nil, errors.New("timeout after submitting")
	})
	app := &LocalApp{token: "private", origin: "http://127.0.0.1:3123"}
	app.profiles[0] = c
	h := app.routes()
	body, _ := json.Marshal(inputV12())
	for i := 0; i < 2; i++ {
		w := localMessageRequestV12(h, "POST", "/api/messages/send?profile=0", string(body))
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"unknown"`) {
			t.Fatal(w.Code, w.Body)
		}
	}
	if posts != 1 {
		t.Fatal("duplicate external send", posts)
	}
	changed := inputV12()
	changed.Text = "Inna treść"
	body, _ = json.Marshal(changed)
	if w := localMessageRequestV12(h, "POST", "/api/messages/send?profile=0", string(body)); w.Code != 409 {
		t.Fatal(w.Code)
	}
}
func TestV12ConcurrentDuplicateSendDoesNotIssueSecondPost(t *testing.T) {
	c, m := fakeAccount()
	prepareSendV12(c, m)
	started, release := make(chan struct{}), make(chan struct{})
	var count atomic.Int32
	c.client.Transport = messageRoundTripV12(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" {
			return m.RoundTrip(r)
		}
		count.Add(1)
		close(started)
		<-release
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`)), Request: r}, nil
	})
	app := &LocalApp{token: "private", origin: "http://127.0.0.1:3123"}
	app.profiles[0] = c
	h := app.routes()
	b, _ := json.Marshal(inputV12())
	done := make(chan int, 1)
	go func() { done <- localMessageRequestV12(h, "POST", "/api/messages/send?profile=0", string(b)).Code }()
	<-started
	second := localMessageRequestV12(h, "POST", "/api/messages/send?profile=0", string(b))
	close(release)
	if second.Code != 409 || <-done != 200 || count.Load() != 1 {
		t.Fatal(second.Code, count.Load())
	}
}
func TestV12AttachmentMembershipAndNoOtherProfileFiles(t *testing.T) {
	c, m := fakeAccount()
	c.messagesReady = true
	m.add("GET", msgBase+"/inbox/messages/42", record{200, fullPayloadV12("70", false, []messageAttachmentV12{{"8", "Inny.pdf"}}), "", ""})
	app := &LocalApp{token: "private", origin: "http://127.0.0.1:3123"}
	app.profiles[0] = c
	w := localMessageRequestV12(app.routes(), "POST", "/api/messages/download?profile=0&box=inbox&id=42&file=9", "")
	if w.Code != 502 || len(m.calls) != 1 {
		t.Fatal(w.Code, m.calls)
	}
}
func TestV12SandboxURLAllowlist(t *testing.T) {
	for _, u := range []string{"https://evil.example/GetFile/123", "http://sandbox.librus.pl/GetFile/123", "https://sandbox.librus.pl:443/GetFile/123", "https://sandbox.librus.pl/GetFile/../123", "https://sandbox.librus.pl/GetFile/%2e%2e/x", "https://sandbox.librus.pl/GetFile/a?url=http://localhost", "https://user:pass@sandbox.librus.pl/GetFile/a"} {
		if _, err := messageSandboxV12(u); err == nil {
			t.Fatal(u)
		}
	}
	for _, u := range []string{"https://sandbox.librus.pl/GetFile/ABC_123", "https://sandbox.librus.pl/index.php?action=CSTryToDownload&singleUseKey=TESTKEY12345"} {
		if _, err := messageSandboxV12(u); err != nil {
			t.Fatal(u, err)
		}
	}
}
func TestV12SandboxPollingAndActualFilename(t *testing.T) {
	c, m := fakeAccount()
	link := "https://sandbox.librus.pl/GetFile/FILE123"
	m.add("GET", link, record{200, "<html>waiting</html>", "", ""})
	m.add("GET", link+"/get", record{200, "<html>waiting</html>", "", ""})
	m.add("GET", link+"/get", record{200, "%PDF-1.7 TEST", "", ""})
	sleeps := 0
	r, b, err := messageSandboxFileV12(context.Background(), c.client, link, func(context.Context, time.Duration) error { sleeps++; return nil })
	if err != nil || sleeps != 1 || string(b) != "%PDF-1.7 TEST" {
		t.Fatal(err, sleeps)
	}
	if name := messageFileNameV12("Statut 2026.11", "", r.Header.Get("Content-Type"), b); name != "Statut 2026.11.pdf" {
		t.Fatal(name)
	}
	if name := messageFileNameV12("Bez nazwy", `attachment; filename*=UTF-8''Za%C5%82%C4%85cznik.pdf`, "", b); name != "Załącznik.pdf" {
		t.Fatal(name)
	}
}
func TestV12AttachmentRejectsHTMLAndOversizedBody(t *testing.T) {
	c, m := fakeAccount()
	link := "https://sandbox.librus.pl/GetFile/FILE123"
	m.add("GET", link, record{200, "<html>login</html>", "", ""})
	m.add("GET", link+"/get", record{200, "<html>login</html>", "", ""})
	_, _, err := messageSandboxFileV12(context.Background(), c.client, link, func(context.Context, time.Duration) error { return context.Canceled })
	if err == nil {
		t.Fatal("saved HTML as file")
	}
	c.client.Transport = messageRoundTripV12(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: messageFileLimitV12 + 1, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("x")), Request: r}, nil
	})
	if _, _, err := messageFileHTTPV12(context.Background(), c.client, "GET", link, nil, link, messageFileLimitV12); err == nil {
		t.Fatal("oversize accepted")
	}
}
func TestV12ProfileReplacementDuringFileDownloadDropsOldDataAndDoesNotLockOtherModules(t *testing.T) {
	c, m := fakeAccount()
	c.messagesReady = true
	m.add("GET", msgBase+"/inbox/messages/42", record{200, fullPayloadV12("70", false, []messageAttachmentV12{{"9", "plik.pdf"}}), "", ""})
	m.add("GET", msgBase+"/attachments/9/messages/42", record{200, `{"data":{"downloadLink":"https://sandbox.librus.pl/GetFile/ABC"}}`, "", ""})
	started, release := make(chan struct{}), make(chan struct{})
	c.client.Transport = messageRoundTripV12(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "sandbox.librus.pl" {
			return m.RoundTrip(r)
		}
		close(started)
		<-release
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("%PDF-OLD PROFILE")), Request: r}, nil
	})
	app := &LocalApp{token: "private", origin: "http://127.0.0.1:3123"}
	app.profiles[0] = c
	h := app.routes()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- localMessageRequestV12(h, "POST", "/api/messages/download?profile=0&box=inbox&id=42&file=9", "")
	}()
	<-started
	unlocked := c.mu.TryLock()
	if unlocked {
		c.mu.Unlock()
	}
	replacement, _ := fakeAccount()
	app.mu.Lock()
	app.profiles[0] = replacement
	app.mu.Unlock()
	close(release)
	w := <-done
	if !unlocked || w.Code != 409 || strings.Contains(w.Body.String(), "OLD PROFILE") {
		t.Fatal(unlocked, w.Code, w.Body)
	}
}
func TestV12PriorSourceFilesFrozen(t *testing.T) {
	raw, err := os.ReadFile("../BASELINE_V12_PRIOR_SOURCES.json")
	if err != nil {
		t.Fatal(err)
	}
	var checks map[string]string
	if json.Unmarshal(raw, &checks) != nil {
		t.Fatal("baseline invalid")
	}
	for name, want := range checks {
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != want {
			t.Fatal("prior file changed", name)
		}
	}
}

func TestV12InvalidSendsNeverReachNetwork(t *testing.T) {
	c, m := fakeAccount()
	app := &LocalApp{token: "private", origin: "http://127.0.0.1:3123"}
	app.profiles[0] = c
	h := app.routes()
	changes := []func(*messageSendInputV12){
		func(i *messageSendInputV12) { i.Recipients = nil }, func(i *messageSendInputV12) { i.Recipients = []string{"70", "70"} },
		func(i *messageSendInputV12) { i.Topic = strings.Repeat("x", 201) }, func(i *messageSendInputV12) { i.Text = strings.Repeat("x", 10001) },
		func(i *messageSendInputV12) { i.RequestID = "bad" }, func(i *messageSendInputV12) { i.ReplyTo = "0" },
	}
	for _, change := range changes {
		in := inputV12()
		change(&in)
		b, _ := json.Marshal(in)
		w := localMessageRequestV12(h, "POST", "/api/messages/send?profile=0", string(b))
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body)
		}
	}
	b, _ := json.Marshal(inputV12())
	w := localMessageRequestV12(h, "POST", "/api/messages/send?profile=0", string(b)+"{}")
	if w.Code != 400 || len(m.calls) != 0 {
		t.Fatal(w.Code, m.calls)
	}
}
func TestV12ProfileReplacementAfterRecipientCheckPreventsSend(t *testing.T) {
	c, m := fakeAccount()
	prepareSendV12(c, m)
	app := &LocalApp{token: "private", origin: "http://127.0.0.1:3123"}
	app.profiles[0] = c
	replacement, _ := fakeAccount()
	c.client.Transport = messageRoundTripV12(func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			t.Error("write after replacing profile")
		}
		response, err := m.RoundTrip(r)
		if strings.Contains(r.URL.Path, "school-employees") {
			app.mu.Lock()
			app.profiles[0] = replacement
			app.mu.Unlock()
		}
		return response, err
	})
	b, _ := json.Marshal(inputV12())
	w := localMessageRequestV12(app.routes(), "POST", "/api/messages/send?profile=0", string(b))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"not_sent"`) {
		t.Fatal(w.Code, w.Body)
	}
}
