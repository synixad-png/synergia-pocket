package main

// Read-only observer: no feature module depends on it. Each probe uses only
// the shared authenticated transport, never opens a message or downloads a file.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"
)

const updatesIntervalV13 = 5 * time.Minute

type watchDefinitionV13 struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Endpoint string   `json:"-"`
	Key      string   `json:"-"`
	Fields   []string `json:"-"`
}

var watchModulesV13 = []watchDefinitionV13{
	{"messages", "Wiadomości odebrane", "", "", []string{"topic", "sendDate", "senderId", "content", "isAnyFileAttached"}},
	{"grades", "Oceny", "Grades", "Grades", []string{"Grade", "Subject", "Category", "Date", "AddDate", "Comments", "Weight", "IsFinal", "IsSemester"}},
	{"timetable", "Plan lekcji i zastępstwa", "Timetables", "Timetable", nil},
	{"attendance", "Frekwencja", "Attendances", "Attendances", []string{"Date", "LessonNo", "Type", "Subject", "AddDate"}},
	{"agenda", "Terminarz", "HomeWorks", "HomeWorks", []string{"Date", "TimeFrom", "TimeTo", "Content", "Category", "Subject"}},
	{"homework", "Zadania domowe", "HomeWorkAssignments", "HomeWorkAssignments", []string{"Date", "DueDate", "Topic", "Text", "Teacher", "Category", "MustSendAttachFile", "HomeworkAssigmentFiles"}},
	{"notes", "Uwagi", "Notes", "Notes", []string{"Date", "Text", "Content", "Category", "Type", "Positive"}},
	{"notices", "Ogłoszenia", "SchoolNotices", "SchoolNotices", []string{"Subject", "Content", "CreationDate", "StartDate", "EndDate"}},
	{"schoolfiles", "Dokumenty szkoły", "SchoolFiles", "Data", []string{"displayName", "addedOnDate", "downloadUrl"}},
}

func watchDefinitionForV13(id string) (watchDefinitionV13, bool) {
	for _, d := range watchModulesV13 {
		if d.ID == id {
			return d, true
		}
	}
	return watchDefinitionV13{}, false
}

type watchSnapshotV13 struct {
	Records map[string]string
	Scope   string
}

func watchHashV13(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func watchParseV13(d watchDefinitionV13, raw any, scope string) (watchSnapshotV13, error) {
	fail := func() (watchSnapshotV13, error) {
		return watchSnapshotV13{}, ProtocolError{"Monitor: nieznany format danych modułu " + d.ID + "."}
	}
	out := watchSnapshotV13{Records: map[string]string{}, Scope: scope}
	if d.ID == "timetable" {
		days, ok := obj(raw)["Timetable"].(map[string]any)
		if !ok {
			return fail()
		}
		for date, v := range days {
			if parsed, e := time.Parse("2006-01-02", date); e != nil || parsed.Format("2006-01-02") != date {
				return fail()
			}
			slots, ok := v.([]any)
			if !ok {
				return fail()
			}
			hashes := []string{}
			for _, slot := range slots {
				rows, ok := slot.([]any)
				if !ok {
					return fail()
				}
				for _, item := range rows {
					row, ok := item.(map[string]any)
					if !ok || row["LessonNo"] == nil {
						return fail()
					}
					hashes = append(hashes, watchHashV13(row))
				}
			}
			sort.Strings(hashes)
			out.Records[date] = watchHashV13(hashes)
		}
		return out, nil
	}
	var values []any
	var ok bool
	if d.ID == "messages" {
		values, ok = raw.([]any)
		if !ok {
			values, ok = obj(raw)["data"].([]any)
		}
	} else {
		values, ok = obj(raw)[d.Key].([]any)
	}
	if !ok || len(values) > 20000 {
		return fail()
	}
	for _, v := range values {
		row, ok := v.(map[string]any)
		if !ok {
			return fail()
		}
		key := "Id"
		if d.ID == "messages" {
			key = "messageId"
		}
		if d.ID == "schoolfiles" {
			key = "id"
		}
		id := str(row[key])
		if !schoolIDPattern.MatchString(id) {
			return fail()
		}
		if d.ID != "schoolfiles" && (!idPattern.MatchString(id) || len(id) > 40) {
			return fail()
		}
		if _, exists := out.Records[id]; exists {
			return fail()
		}
		selected := map[string]any{}
		for _, field := range d.Fields {
			if value, exists := row[field]; exists {
				selected[field] = value
			}
		}
		// A changed schema must not silently turn existing data into empty records.
		if len(selected) == 0 {
			return fail()
		}
		if d.ID == "grades" && row["Grade"] == nil {
			return fail()
		}
		out.Records[id] = watchHashV13(selected)
	}
	return out, nil
}
func (c *APIClient) watchSnapshotV13(ctx context.Context, d watchDefinitionV13) (watchSnapshotV13, error) {
	var raw any
	var err error
	scope := ""
	if d.ID == "messages" {
		raw, err = c.messageGet(ctx, "inbox/messages", url.Values{"limit": {"30"}})
	} else {
		var query url.Values
		if d.ID == "timetable" {
			zone, _ := time.LoadLocation("Europe/Warsaw")
			now := time.Now().In(zone)
			scope = now.AddDate(0, 0, -(int(now.Weekday())+6)%7).Format("2006-01-02")
			query = url.Values{"weekStart": {scope}}
		}
		raw, err = c.data(ctx, d.Endpoint, query)
	}
	if err != nil {
		return watchSnapshotV13{}, err
	}
	return watchParseV13(d, raw, scope)
}

// Waiting for another operation's shared session lock respects cancellation.
func watchLockV13(ctx context.Context, c *APIClient) bool {
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		if ctx.Err() != nil {
			return false
		}
		if c.mu.TryLock() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-tick.C:
		}
	}
}

type watchStatusV13 struct {
	Module  string `json:"module"`
	Name    string `json:"name"`
	State   string `json:"state"`
	Detail  string `json:"detail"`
	Checked string `json:"checked"`
	Count   int    `json:"count"`
	Kind    string `json:"errorKind,omitempty"`
}
type watchEventV13 struct {
	ID      int64  `json:"id"`
	Module  string `json:"module"`
	Name    string `json:"name"`
	Added   int    `json:"added"`
	Changed int    `json:"changed"`
	At      string `json:"at"`
}
type watchProfileV13 struct {
	client      *APIClient
	enabled     bool
	running     bool
	cancel      context.CancelFunc
	kick        chan struct{}
	baseline    map[string]watchSnapshotV13
	statuses    map[string]watchStatusV13
	events      []watchEventV13
	sequence    int64
	readThrough int64
	lastStarted time.Time
	next        time.Time
}
type updatesServiceV13 struct {
	app      *LocalApp
	mu       sync.Mutex
	profiles [2]*watchProfileV13
	probe    func(context.Context, *APIClient, watchDefinitionV13) (watchSnapshotV13, error)
}

func newUpdatesServiceV13(a *LocalApp) *updatesServiceV13 {
	return &updatesServiceV13{app: a, probe: func(ctx context.Context, c *APIClient, d watchDefinitionV13) (watchSnapshotV13, error) {
		return c.watchSnapshotV13(ctx, d)
	}}
}
func (s *updatesServiceV13) current(idx int, c *APIClient) bool {
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	return s.app.profiles[idx] == c
}

// Caller holds s.mu. Re-authentication creates a new identity, never retaining
// notifications or permission to poll from the previous login in this slot.
func (s *updatesServiceV13) state(idx int, c *APIClient) *watchProfileV13 {
	p := s.profiles[idx]
	if p == nil || p.client != c {
		if p != nil && p.cancel != nil {
			p.cancel()
		}
		p = &watchProfileV13{client: c, baseline: map[string]watchSnapshotV13{}, statuses: map[string]watchStatusV13{}, events: []watchEventV13{}}
		s.profiles[idx] = p
	}
	return p
}
func watchMergeV13(previous watchSnapshotV13, next watchSnapshotV13) (watchSnapshotV13, int, int, error) {
	if previous.Records == nil || previous.Scope != next.Scope {
		return next, 0, 0, nil
	}
	merged := watchSnapshotV13{Records: map[string]string{}, Scope: next.Scope}
	for k, v := range previous.Records {
		merged.Records[k] = v
	}
	added, changed := 0, 0
	for k, v := range next.Records {
		old, exists := merged.Records[k]
		if !exists {
			added++
		} else if old != v {
			changed++
		}
		merged.Records[k] = v
	}
	if len(merged.Records) > 20000 {
		return previous, 0, 0, ProtocolError{"Monitor: przekroczono limit historii identyfikatorów."}
	}
	// Retaining IDs avoids duplicate alerts when a limited inbox slides or a
	// temporarily absent item reappears. Deletions are not claimed as detected.
	return merged, added, changed, nil
}
func (s *updatesServiceV13) scan(ctx context.Context, idx int, p *watchProfileV13) {
	for _, d := range watchModulesV13 {
		if ctx.Err() != nil || !s.current(idx, p.client) {
			return
		}
		child, cancel := context.WithTimeout(ctx, 32*time.Second)
		var snap watchSnapshotV13
		var err error
		if watchLockV13(child, p.client) {
			snap, err = s.probe(child, p.client, d)
			p.client.mu.Unlock()
		} else {
			err = child.Err()
		}
		cancel()
		s.mu.Lock()
		if ctx.Err() != nil || s.profiles[idx] != p || !p.enabled || !s.current(idx, p.client) {
			s.mu.Unlock()
			return
		}
		status := watchStatusV13{Module: d.ID, Name: d.Name, Checked: time.Now().Format(time.RFC3339), Count: len(snap.Records), State: "ok", Detail: "Odczyt i format poprawne."}
		if err == nil {
			var added, changed int
			var merged watchSnapshotV13
			merged, added, changed, err = watchMergeV13(p.baseline[d.ID], snap)
			if err == nil {
				p.baseline[d.ID] = merged
				if len(snap.Records) == 0 {
					status.State = "empty"
					status.Detail = "Poprawna odpowiedź; brak wpisów."
				}
				if added+changed > 0 {
					p.sequence++
					p.events = append(p.events, watchEventV13{p.sequence, d.ID, d.Name, added, changed, status.Checked})
					if len(p.events) > 100 {
						p.events = p.events[len(p.events)-100:]
					}
				}
			}
		}
		if err != nil {
			status.State = "failed"
			status.Kind, status.Detail = diagnosticError(err)
		}
		p.statuses[d.ID] = status
		s.mu.Unlock()
	}
}
func (s *updatesServiceV13) loop(ctx context.Context, idx int, p *watchProfileV13) {
	for {
		s.scan(ctx, idx, p)
		s.mu.Lock()
		if ctx.Err() != nil || s.profiles[idx] != p || !p.enabled || !s.current(idx, p.client) {
			if s.profiles[idx] == p {
				p.running = false
				p.enabled = false
				p.next = time.Time{}
			}
			s.mu.Unlock()
			return
		}
		p.running = false
		p.next = time.Now().Add(updatesIntervalV13)
		s.mu.Unlock()
		timer := time.NewTimer(updatesIntervalV13)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-p.kick:
			timer.Stop()
		case <-timer.C:
		}
		s.mu.Lock()
		if ctx.Err() != nil || !p.enabled || s.profiles[idx] != p {
			s.mu.Unlock()
			return
		}
		p.running = true
		p.lastStarted = time.Now()
		p.next = time.Time{}
		s.mu.Unlock()
	}
}
func (s *updatesServiceV13) handler(w http.ResponseWriter, r *http.Request) {
	if !s.app.validRequest(w, r) {
		return
	}
	method := "GET"
	if r.URL.Path != "/api/updates/state" {
		method = "POST"
	}
	if r.Method != method {
		http.Error(w, "Method", 405)
		return
	}
	idx, err := strconv.Atoi(r.URL.Query().Get("profile"))
	if err != nil || idx < 0 || idx > 1 {
		serveJSON(w, 400, map[string]string{"error": "Nieprawidłowy profil."})
		return
	}
	s.app.mu.RLock()
	c := s.app.profiles[idx]
	s.app.mu.RUnlock()
	if c == nil {
		serveJSON(w, 401, map[string]string{"error": "Najpierw zaloguj profil."})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.current(idx, c) {
		serveJSON(w, 409, map[string]string{"error": "Profil zmienił sesję. Odśwież widok."})
		return
	}
	p := s.state(idx, c)
	switch r.URL.Path {
	case "/api/updates/enable":
		value := r.URL.Query().Get("enabled")
		if value != "true" && value != "false" {
			serveJSON(w, 400, map[string]string{"error": "Nieprawidłowy wybór."})
			return
		}
		enable := value == "true"
		if enable && !p.enabled {
			fresh := *p
			p = &fresh
			s.profiles[idx] = p
			ctx, cancel := context.WithCancel(context.Background())
			p.cancel = cancel
			p.kick = make(chan struct{}, 1)
			p.enabled = true
			p.running = true
			p.lastStarted = time.Now()
			go s.loop(ctx, idx, p)
		}
		if !enable && p.enabled {
			p.enabled = false
			p.running = false
			p.next = time.Time{}
			p.cancel()
		}
	case "/api/updates/check":
		if !p.enabled {
			serveJSON(w, 409, map[string]string{"error": "Najpierw włącz obserwowanie zmian."})
			return
		}
		if !p.running {
			if time.Since(p.lastStarted) < time.Minute {
				serveJSON(w, 429, map[string]string{"error": "Kolejne sprawdzenie będzie dostępne minutę po poprzednim."})
				return
			}
			p.running = true
			p.lastStarted = time.Now()
			p.next = time.Time{}
			select {
			case p.kick <- struct{}{}:
			default:
			}
		}
	case "/api/updates/ack":
		through, e := strconv.ParseInt(r.URL.Query().Get("through"), 10, 64)
		if e != nil || through < 0 || through > p.sequence {
			serveJSON(w, 400, map[string]string{"error": "Nieprawidłowy numer powiadomienia."})
			return
		}
		if through > p.readThrough {
			p.readThrough = through
		}
	}
	statuses := []watchStatusV13{}
	for _, d := range watchModulesV13 {
		item, ok := p.statuses[d.ID]
		if !ok {
			item = watchStatusV13{Module: d.ID, Name: d.Name, State: "unverified", Detail: "Jeszcze nie sprawdzono."}
		}
		statuses = append(statuses, item)
	}
	unread := 0
	for _, e := range p.events {
		if e.ID > p.readThrough {
			unread++
		}
	}
	next := ""
	if !p.next.IsZero() {
		next = p.next.Format(time.RFC3339)
	}
	serveJSON(w, 200, map[string]any{"profile": idx, "enabled": p.enabled, "running": p.running, "intervalSeconds": int(updatesIntervalV13.Seconds()), "next": next, "events": p.events, "unread": unread, "through": p.sequence, "statuses": statuses})
}

// Extended diagnostics remains separate from the frozen v0.9 routes. It is
// manually invoked and never tests writes or actual file transfers.
func (s *updatesServiceV13) health(w http.ResponseWriter, r *http.Request) {
	if !s.app.validRequest(w, r) {
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method", 405)
		return
	}
	idx, e := strconv.Atoi(r.URL.Query().Get("profile"))
	if e != nil || idx < 0 || idx > 1 {
		serveJSON(w, 400, map[string]string{"error": "Nieprawidłowy profil."})
		return
	}
	id := r.URL.Query().Get("module")
	d, watch := watchDefinitionForV13(id)
	if !watch && id != "outbox" && id != "recipients" && id != "freedays" {
		serveJSON(w, 400, map[string]string{"error": "Nieznany moduł."})
		return
	}
	s.app.mu.RLock()
	c := s.app.profiles[idx]
	s.app.mu.RUnlock()
	if c == nil {
		serveJSON(w, 401, map[string]string{"error": "Najpierw zaloguj profil."})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 32*time.Second)
	defer cancel()
	result := watchStatusV13{Module: id, State: "ok", Detail: "Odczyt i format poprawne.", Checked: time.Now().Format(time.RFC3339)}
	var err error
	if !watchLockV13(ctx, c) {
		err = ctx.Err()
	} else {
		if watch {
			var snapshot watchSnapshotV13
			snapshot, err = s.probe(ctx, c, d)
			result.Count = len(snapshot.Records)
		} else {
			switch id {
			case "outbox":
				var rows []messageRowV12
				rows, err = c.messagesListV12(ctx, "outbox")
				result.Count = len(rows)
			case "recipients":
				var rows []messageRecipientV12
				rows, err = c.messageRecipientsV12(ctx)
				result.Count = len(rows)
			case "freedays":
				var rows moduleRows
				rows, err = c.getFreeDaysV11(ctx)
				result.Count = len(rows.items)
				if len(rows.warnings) > 0 {
					result.State = "partial"
					result.Detail = "Niepełne dane uzupełniające."
				}
			}
		}
		c.mu.Unlock()
	}
	if !s.current(idx, c) {
		serveJSON(w, 409, map[string]string{"error": "Profil zmienił sesję. Powtórz sprawdzenie."})
		return
	}
	if err != nil {
		result.State = "failed"
		result.Kind, result.Detail = diagnosticError(err)
	} else if result.Count == 0 && result.State == "ok" {
		result.State = "empty"
		result.Detail = "Poprawna odpowiedź; brak wpisów."
	}
	serveJSON(w, 200, result)
}
func installUpdatesRoutesV13(mux *http.ServeMux, a *LocalApp) *updatesServiceV13 {
	s := newUpdatesServiceV13(a)
	mux.HandleFunc("/api/updates/profiles", func(w http.ResponseWriter, r *http.Request) {
		if !a.validRequest(w, r) {
			return
		}
		if r.Method != "GET" {
			http.Error(w, "Method", 405)
			return
		}
		a.mu.RLock()
		logged := [2]bool{a.profiles[0] != nil, a.profiles[1] != nil}
		a.mu.RUnlock()
		serveJSON(w, 200, map[string]any{"logged": logged})
	})
	for _, path := range []string{"state", "enable", "check", "ack"} {
		mux.HandleFunc("/api/updates/"+path, s.handler)
	}
	mux.HandleFunc("/api/health/check", s.health)
	return s
}
