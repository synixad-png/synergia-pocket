package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func watchJSONV13(t *testing.T, id, body string) watchSnapshotV13 {
	t.Helper()
	d, ok := watchDefinitionForV13(id)
	if !ok {
		t.Fatal(id)
	}
	var raw any
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatal(err)
	}
	snap, err := watchParseV13(d, raw, "week")
	if err != nil {
		t.Fatal(err)
	}
	return snap
}
func watchFixtureV13(t *testing.T) (*updatesServiceV13, http.Handler) {
	t.Helper()
	c, _ := fakeAccount()
	a := &LocalApp{token: "private", origin: "http://127.0.0.1:3123", profiles: [2]*APIClient{c, nil}}
	mux := http.NewServeMux()
	s := installUpdatesRoutesV13(mux, a)
	t.Cleanup(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, p := range s.profiles {
			if p != nil && p.cancel != nil {
				p.cancel()
			}
		}
	})
	return s, mux
}
func waitWatchV13(t *testing.T, predicate func() bool) {
	t.Helper()
	until := time.Now().Add(3 * time.Second)
	for !predicate() {
		if time.Now().After(until) {
			t.Fatal("watch worker did not settle")
		}
		time.Sleep(time.Millisecond * 5)
	}
}
func TestV13ParsesKnownShapesAndRejectsSchemaChanges(t *testing.T) {
	for _, d := range watchModulesV13 {
		t.Run(d.ID, func(t *testing.T) {
			var raw any
			if d.ID == "messages" {
				raw = map[string]any{"data": []any{map[string]any{"messageId": float64(2), "topic": "Test"}}}
			} else if d.ID == "timetable" {
				raw = map[string]any{"Timetable": map[string]any{"2026-11-04": []any{[]any{map[string]any{"LessonNo": float64(1)}}}}}
			} else {
				key := "Id"
				if d.ID == "schoolfiles" {
					key = "id"
				}
				raw = map[string]any{d.Key: []any{map[string]any{key: "2", d.Fields[0]: "value"}}}
			}
			snap, e := watchParseV13(d, raw, "")
			if e != nil || len(snap.Records) != 1 {
				t.Fatal(snap, e)
			}
			for _, bad := range []any{nil, false, map[string]any{}, map[string]any{d.Key: false}} {
				if _, e := watchParseV13(d, bad, ""); e == nil {
					t.Fatal("bad shape accepted", bad)
				}
			}
		})
	}
	for _, body := range []string{`{"Grades":[{"Grade":"5"}]}`, `{"Grades":[{"Id":true,"Grade":"5"}]}`, `{"Grades":[{"Id":1,"Other":"secret"}]}`, `{"Grades":[{"Id":1,"Grade":"5"},{"Id":1,"Grade":"4"}]}`, `{"Grades":[null]}`} {
		d, _ := watchDefinitionForV13("grades")
		var raw any
		json.Unmarshal([]byte(body), &raw)
		if _, e := watchParseV13(d, raw, ""); e == nil {
			t.Fatal(body)
		}
	}
}
func TestV13FirstBaselineNoOldAlertsNewAndEditedEntries(t *testing.T) {
	a := watchJSONV13(t, "grades", `{"Grades":[{"Id":1,"Grade":"5"}]}`)
	baseline, n, c, e := watchMergeV13(watchSnapshotV13{}, a)
	if e != nil || n+c != 0 {
		t.Fatal(n, c, e)
	}
	b := watchJSONV13(t, "grades", `{"Grades":[{"Id":2,"Grade":"4"},{"Id":1,"Grade":"6"}]}`)
	next, n, c, e := watchMergeV13(baseline, b)
	if e != nil || n != 1 || c != 1 {
		t.Fatal(n, c, e)
	}
	_, n, c, e = watchMergeV13(next, b)
	if e != nil || n+c != 0 {
		t.Fatal(n, c, e)
	}
}
func TestV13ReadFlagsAndJSONOrderingDoNotNotify(t *testing.T) {
	a := watchJSONV13(t, "messages", `{"data":[{"messageId":42,"topic":"T","content":"x","readDate":null}]}`)
	b := watchJSONV13(t, "messages", `[{"readDate":"2026-11-04","content":"x","topic":"T","messageId":42}]`)
	if a.Records["42"] != b.Records["42"] {
		t.Fatal("read flag changed fingerprint")
	}
}
func TestV13InboxWindowRetainsSeenIDsAndWeekResets(t *testing.T) {
	a := watchSnapshotV13{Records: map[string]string{"1": "a", "2": "b"}, Scope: ""}
	b := watchSnapshotV13{Records: map[string]string{"2": "b", "3": "c"}}
	merged, n, _, _ := watchMergeV13(a, b)
	if n != 1 {
		t.Fatal(n)
	}
	_, n, c, e := watchMergeV13(merged, a)
	if n+c != 0 || e != nil {
		t.Fatal(n, c, e)
	}
	b.Scope = "next week"
	_, n, c, e = watchMergeV13(a, b)
	if n+c != 0 || e != nil {
		t.Fatal("new week is not a fresh baseline")
	}
}
func TestV13TimetableCanonicalizationAndChanges(t *testing.T) {
	a := watchJSONV13(t, "timetable", `{"Timetable":{"2026-11-04":[[{"LessonNo":1,"IsCanceled":false},{"LessonNo":2}]]}}`)
	b := watchJSONV13(t, "timetable", `{"Timetable":{"2026-11-04":[[{"LessonNo":2},{"IsCanceled":false,"LessonNo":1}]]}}`)
	if a.Records["2026-11-04"] != b.Records["2026-11-04"] {
		t.Fatal("order changed hash")
	}
	c := watchJSONV13(t, "timetable", `{"Timetable":{"2026-11-04":[[{"LessonNo":1,"IsCanceled":true}]]}}`)
	_, n, changed, e := watchMergeV13(a, c)
	if n != 0 || changed != 1 || e != nil {
		t.Fatal(n, changed, e)
	}
}
func TestV13HomeworkTextChangesAreObserved(t *testing.T) {
	a := watchJSONV13(t, "homework", `{"HomeWorkAssignments":[{"Id":1,"Topic":"T","Text":"Before"}]}`)
	b := watchJSONV13(t, "homework", `{"HomeWorkAssignments":[{"Id":1,"Topic":"T","Text":"After"}]}`)
	if a.Records["1"] == b.Records["1"] {
		t.Fatal("Text not observed")
	}
}
func TestV13ProbeNeverOpensOrMarksMessage(t *testing.T) {
	c, m := fakeAccount()
	c.messagesReady = true
	m.add("GET", msgBase+"/inbox/messages?limit=30", record{200, `{"data":[{"messageId":2,"topic":"Private title"}]}`, "", ""})
	d, _ := watchDefinitionForV13("messages")
	snap, e := c.watchSnapshotV13(context.Background(), d)
	if e != nil || len(snap.Records) != 1 || len(m.calls) != 1 || m.calls[0] != "GET "+msgBase+"/inbox/messages?limit=30" {
		t.Fatal(e, m.calls)
	}
	b, _ := json.Marshal(snap)
	if strings.Contains(string(b), "Private") {
		t.Fatal("snapshot retained school content")
	}
}
func TestV13ScanIsolatesErrorsAndPreservesBaseline(t *testing.T) {
	s, _ := watchFixtureV13(t)
	c := s.app.profiles[0]
	s.mu.Lock()
	p := s.state(0, c)
	p.enabled = true
	p.baseline["grades"] = watchSnapshotV13{Records: map[string]string{"1": "old"}}
	s.mu.Unlock()
	var calls []string
	s.probe = func(_ context.Context, _ *APIClient, d watchDefinitionV13) (watchSnapshotV13, error) {
		calls = append(calls, d.ID)
		if d.ID == "grades" {
			return watchSnapshotV13{}, ProtocolError{"nieznany format SECRET"}
		}
		return watchSnapshotV13{Records: map[string]string{}}, nil
	}
	s.scan(context.Background(), 0, p)
	if len(calls) != len(watchModulesV13) || p.statuses["grades"].Kind != "format" || p.statuses["schoolfiles"].State != "empty" || p.baseline["grades"].Records["1"] != "old" {
		t.Fatal(calls, p.statuses, p.baseline)
	}
	if strings.Contains(p.statuses["grades"].Detail, "SECRET") {
		t.Fatal("opaque upstream error leaked")
	}
}
func TestV13NotificationsBoundedAndAckIsLocal(t *testing.T) {
	s, mux := watchFixtureV13(t)
	s.mu.Lock()
	p := s.state(0, s.app.profiles[0])
	for i := int64(1); i <= 3; i++ {
		p.events = append(p.events, watchEventV13{ID: i, Module: "grades"})
		p.sequence = i
	}
	s.mu.Unlock()
	w := localMessageRequestV12(mux, "POST", "/api/updates/ack?profile=0&through=2", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"unread":1`) {
		t.Fatal(w.Code, w.Body)
	}
	if w = localMessageRequestV12(mux, "POST", "/api/updates/ack?profile=0&through=999", ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
}
func TestV13AuthMethodsAndProfileValidation(t *testing.T) {
	s, mux := watchFixtureV13(t)
	var calls atomic.Int32
	s.probe = func(context.Context, *APIClient, watchDefinitionV13) (watchSnapshotV13, error) {
		calls.Add(1)
		return watchSnapshotV13{}, nil
	}
	for _, test := range []struct {
		method, path string
		code         int
	}{{"GET", "/api/updates/enable?profile=0&enabled=true", 405}, {"POST", "/api/updates/enable?profile=0&enabled=maybe", 400}, {"GET", "/api/updates/state?profile=2", 400}, {"GET", "/api/updates/state?profile=1", 401}, {"POST", "/api/updates/check?profile=0", 409}, {"GET", "/api/health/check?profile=0&module=send", 400}} {
		w := localMessageRequestV12(mux, test.method, test.path, "")
		if w.Code != test.code {
			t.Fatal(test, w.Code, w.Body)
		}
	}
	r := httptest.NewRequest("POST", "http://127.0.0.1:3123/api/updates/enable?profile=0&enabled=true", nil)
	r.AddCookie(&http.Cookie{Name: "synergia_local", Value: "private"})
	r.Header.Set("Origin", "https://evil.invalid")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 403 || calls.Load() != 0 {
		t.Fatal(w.Code, calls.Load())
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/updates/state?profile=0", nil))
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
func TestV13BackgroundBaselineCooldownAndProfileSeparation(t *testing.T) {
	s, mux := watchFixtureV13(t)
	s.probe = func(context.Context, *APIClient, watchDefinitionV13) (watchSnapshotV13, error) {
		return watchSnapshotV13{Records: map[string]string{"1": "hash"}}, nil
	}
	c2, _ := fakeAccount()
	s.app.profiles[1] = c2
	w := localMessageRequestV12(mux, "POST", "/api/updates/enable?profile=0&enabled=true", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	waitWatchV13(t, func() bool { s.mu.Lock(); defer s.mu.Unlock(); return !s.profiles[0].running })
	s.mu.Lock()
	p := s.profiles[0]
	if len(p.baseline) != len(watchModulesV13) || len(p.events) != 0 || s.profiles[1] != nil {
		s.mu.Unlock()
		t.Fatal("initial baseline/profile leak")
	}
	s.mu.Unlock()
	w = localMessageRequestV12(mux, "POST", "/api/updates/check?profile=0", "")
	if w.Code != 429 {
		t.Fatal(w.Code)
	}
	w = localMessageRequestV12(mux, "GET", "/api/updates/state?profile=1", "")
	if !strings.Contains(w.Body.String(), `"enabled":false`) || !strings.Contains(w.Body.String(), `"events":[]`) {
		t.Fatal(w.Body)
	}
}
func TestV13DisableThenEnableCannotBeStoppedByOldWorker(t *testing.T) {
	s, mux := watchFixtureV13(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls atomic.Int32
	s.probe = func(ctx context.Context, _ *APIClient, _ watchDefinitionV13) (watchSnapshotV13, error) {
		if calls.Add(1) == 1 {
			started <- struct{}{}
			<-release
		}
		return watchSnapshotV13{Records: map[string]string{}}, nil
	}
	localMessageRequestV12(mux, "POST", "/api/updates/enable?profile=0&enabled=true", "")
	<-started
	localMessageRequestV12(mux, "POST", "/api/updates/enable?profile=0&enabled=false", "")
	localMessageRequestV12(mux, "POST", "/api/updates/enable?profile=0&enabled=true", "")
	close(release)
	waitWatchV13(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		p := s.profiles[0]
		return !p.running && len(p.statuses) == len(watchModulesV13)
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.profiles[0].enabled {
		t.Fatal("old worker stopped new one")
	}
}
func TestV13ReloginDiscardsOldScanAndHistory(t *testing.T) {
	s, mux := watchFixtureV13(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	s.probe = func(context.Context, *APIClient, watchDefinitionV13) (watchSnapshotV13, error) {
		started <- struct{}{}
		<-release
		return watchSnapshotV13{Records: map[string]string{"private": "hash"}}, nil
	}
	localMessageRequestV12(mux, "POST", "/api/updates/enable?profile=0&enabled=true", "")
	<-started
	replacement, _ := fakeAccount()
	s.app.mu.Lock()
	s.app.profiles[0] = replacement
	s.app.mu.Unlock()
	close(release)
	w := localMessageRequestV12(mux, "GET", "/api/updates/state?profile=0", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"enabled":false`) || strings.Contains(w.Body.String(), "private") {
		t.Fatal(w.Code, w.Body)
	}
}
func TestV13LockCancellationAndPrivacyOfHealthErrors(t *testing.T) {
	c, _ := fakeAccount()
	c.mu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if watchLockV13(ctx, c) {
		t.Fatal("lock ignored canceled context")
	}
	c.mu.Unlock()
	s, mux := watchFixtureV13(t)
	s.probe = func(context.Context, *APIClient, watchDefinitionV13) (watchSnapshotV13, error) {
		return watchSnapshotV13{}, errors.New("PASSWORD STUDENT MESSAGE CONTENT")
	}
	w := localMessageRequestV12(mux, "GET", "/api/health/check?profile=0&module=grades", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "PASSWORD") || !strings.Contains(w.Body.String(), `"state":"failed"`) {
		t.Fatal(w.Code, w.Body)
	}
}
func TestV13SeenHistoryLimitDoesNotEvictSilently(t *testing.T) {
	previous := watchSnapshotV13{Records: map[string]string{}}
	for i := 0; i < 20000; i++ {
		previous.Records[time.Unix(int64(i), 0).String()] = "h"
	}
	_, _, _, e := watchMergeV13(previous, watchSnapshotV13{Records: map[string]string{"new": "h"}})
	if e == nil {
		t.Fatal("history evicted silently")
	}
}

func TestV13ScanProducesBoundedEventsAndRecoversAfterFailure(t *testing.T) {
	s, _ := watchFixtureV13(t)
	p := s.state(0, s.app.profiles[0])
	p.enabled = true
	version := 0
	fail := false
	s.probe = func(_ context.Context, _ *APIClient, d watchDefinitionV13) (watchSnapshotV13, error) {
		if d.ID == "grades" {
			if fail {
				return watchSnapshotV13{}, ProtocolError{"HTTP 503"}
			}
			return watchSnapshotV13{Records: map[string]string{"1": time.Unix(int64(version), 0).String()}}, nil
		}
		return watchSnapshotV13{Records: map[string]string{}}, nil
	}
	s.scan(context.Background(), 0, p)
	if len(p.events) != 0 {
		t.Fatal("initial history generated events")
	}
	version = 1
	fail = true
	s.scan(context.Background(), 0, p)
	if len(p.events) != 0 {
		t.Fatal("failure generated event")
	}
	fail = false
	s.scan(context.Background(), 0, p)
	if len(p.events) != 1 || p.events[0].Changed != 1 || p.statuses["grades"].State != "ok" {
		t.Fatal(p.events, p.statuses)
	}
	for version = 2; version <= 105; version++ {
		s.scan(context.Background(), 0, p)
	}
	if len(p.events) != 100 || p.sequence != 105 || p.events[0].ID != 6 {
		t.Fatal(len(p.events), p.sequence, p.events[0])
	}
}

func TestV13RuntimeServesIntegratedPage(t *testing.T) {
	a := &LocalApp{token: "private", origin: "http://127.0.0.1:3123"}
	w := localMessageRequestV12(a.routes(), "GET", "/", "")
	for _, fragment := range []string{"v0.13", `data-section="updates"`, `data-section="health"`, "window.SynergiaUpdatesV13=", "window.SynergiaMessagesV12=", "frame-ancestors"} {
		text := w.Body.String() + w.Header().Get("Content-Security-Policy")
		if !strings.Contains(text, fragment) {
			t.Fatal("missing integrated page fragment", fragment)
		}
	}
	if w.Code != 200 || strings.Contains(w.Body.String(), "v0.12") {
		t.Fatal(w.Code)
	}
}

func TestV13ExistingProfilesCanBeRestoredAfterPanelReload(t *testing.T) {
	_, mux := watchFixtureV13(t)
	w := localMessageRequestV12(mux, "GET", "/api/updates/profiles", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"logged":[true,false]`) {
		t.Fatal(w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "username") || strings.Contains(w.Body.String(), "token") {
		t.Fatal("private data in profile restore")
	}
	w = localMessageRequestV12(mux, "POST", "/api/updates/profiles", "")
	if w.Code != 405 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/updates/profiles", nil))
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
