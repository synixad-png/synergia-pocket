package main

// Hosted adapter. The transport and functional API modules remain the own
// v0.13 client. A LocalApp is private to one opaque browser session; it is
// never a singleton shared by visitors of the public Render URL.
import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed web/*
var cloudAssets embed.FS

const cloudSessionIdle = 12 * time.Hour

type cloudSession struct {
	app      *LocalApp
	handler  http.Handler
	seen     time.Time
	inFlight int
}
type cloudApp struct {
	origin     string
	mu         sync.Mutex
	sessions   map[string]*cloudSession
	loginSlots chan struct{}
	now        func() time.Time
	html       string
}

func newCloudApp(origin string) *cloudApp {
	return &cloudApp{origin: origin, sessions: map[string]*cloudSession{}, loginSlots: make(chan struct{}, 4), now: time.Now, html: cloudHTML()}
}
func (s *updatesServiceV13) stopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.profiles {
		if p != nil {
			p.enabled = false
			p.running = false
			if p.cancel != nil {
				p.cancel()
			}
		}
	}
}
func (session *cloudSession) close() {
	if session.app.stopUpdates != nil {
		session.app.stopUpdates()
	}
	session.app.mu.Lock()
	clients := session.app.profiles
	session.app.profiles = [2]*APIClient{}
	session.app.mu.Unlock()
	for _, c := range clients {
		if c != nil {
			c.client.CloseIdleConnections()
		}
	}
}
func (c *cloudApp) prune() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for key, s := range c.sessions {
		if s.inFlight == 0 && now.Sub(s.seen) > cloudSessionIdle {
			delete(c.sessions, key)
			s.close()
		}
	}
}
func (c *cloudApp) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, s := range c.sessions {
		delete(c.sessions, key)
		s.close()
	}
}
func (c *cloudApp) cookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: "synergia_local", Value: value, Path: "/", Secure: strings.HasPrefix(c.origin, "https://"), HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: maxAge})
}
func cloudHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline' 'self'; connect-src 'self'; img-src 'self'; manifest-src 'self'; worker-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
}
func (c *cloudApp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cloudHeaders(w)
	if r.URL.Path == "/api/health" {
		if r.Method != "GET" {
			http.Error(w, "Method", 405)
			return
		}
		serveJSON(w, 200, map[string]string{"status": "ok", "service": "synergia-pocket-own-api", "version": "web-0.16", "client": "own-go-v0.13"})
		return
	}
	origin, _ := url.Parse(c.origin)
	if !strings.EqualFold(r.Host, origin.Host) {
		http.Error(w, "Nieprawidłowy adres aplikacji.", 400)
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("Origin") != c.origin {
		http.Error(w, "Nieprawidłowe pochodzenie żądania.", 403)
		return
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" && r.URL.Path != "/" {
		http.Error(w, "Brak dostępu z innej strony.", 403)
		return
	}
	if path, ok := map[string]string{"/manifest.webmanifest": "manifest.webmanifest", "/sw.js": "sw.js", "/offline.html": "offline.html", "/icon.svg": "icon.svg", "/icon-192.png": "icon-192.png", "/icon-512.png": "icon-512.png", "/apple-touch-icon.png": "apple-touch-icon.png"}[r.URL.Path]; ok {
		if r.Method != "GET" && r.Method != "HEAD" {
			http.Error(w, "Method", 405)
			return
		}
		data, e := cloudAssets.ReadFile("web/" + path)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		mime := "image/png"
		switch {
		case strings.HasSuffix(path, ".js"):
			mime = "application/javascript; charset=utf-8"
			w.Header().Set("Service-Worker-Allowed", "/")
		case strings.HasSuffix(path, ".svg"):
			mime = "image/svg+xml"
		case strings.HasSuffix(path, ".html"):
			mime = "text/html; charset=utf-8"
		case strings.HasSuffix(path, ".webmanifest"):
			mime = "application/manifest+json"
		}
		w.Header().Set("Content-Type", mime)
		w.Write(data)
		return
	}
	if r.URL.Path == "/robots.txt" {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("User-agent: *\nDisallow: /\n"))
		return
	}
	if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("k") != "" {
		http.Error(w, "Nieprawidłowy adres strony.", 400)
		return
	}
	if r.URL.Path == "/" && r.Method != "GET" {
		http.Error(w, "Method", 405)
		return
	}
	var key string
	if cookie, e := r.Cookie("synergia_local"); e == nil && len(cookie.Value) == 64 {
		key = cookie.Value
	}
	c.mu.Lock()
	session := c.sessions[key]
	if session != nil && session.inFlight == 0 && c.now().Sub(session.seen) > cloudSessionIdle {
		delete(c.sessions, key)
		session.close()
		session = nil
	}
	if session == nil && r.URL.Path == "/" {
		if len(c.sessions) >= 200 {
			c.mu.Unlock()
			http.Error(w, "Aplikacja jest zajęta. Spróbuj później.", 503)
			return
		}
		key = token()
		a := &LocalApp{token: key, origin: c.origin}
		session = &cloudSession{app: a, handler: a.routes()}
		c.sessions[key] = session
		c.cookie(w, key, int(cloudSessionIdle.Seconds()))
	}
	if session == nil {
		c.mu.Unlock()
		serveJSON(w, 401, map[string]string{"error": "Sesja testowa wygasła. Odśwież stronę i zaloguj profil ponownie."})
		return
	}
	session.seen = c.now()
	session.inFlight++
	c.mu.Unlock()
	defer func() { c.mu.Lock(); session.inFlight--; c.mu.Unlock() }()
	if r.URL.Path == "/" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(c.html))
		return
	}
	if r.URL.Path == "/api/exit" {
		if r.Method != "POST" {
			http.Error(w, "Method", 405)
			return
		}
		c.mu.Lock()
		delete(c.sessions, key)
		c.mu.Unlock()
		session.close()
		c.cookie(w, "", -1)
		serveJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	// Resource controls apply only to login attempts; a slow login cannot starve
	// requests to another visitor's already authenticated functional modules.
	if r.URL.Path == "/api/login" {
		if r.Method != "POST" {
			http.Error(w, "Method", 405)
			return
		}
		select {
		case c.loginSlots <- struct{}{}:
			defer func() { <-c.loginSlots }()
		default:
			serveJSON(w, 429, map[string]string{"error": "Trwa kilka logowań. Spróbuj za chwilę."})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 100*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
	}
	session.handler.ServeHTTP(w, r)
}

func cloudHTML() string {
	page := runtimeHTMLV13()
	replacements := []string{
		"<title>SYNERGIA API", "<title>Synergia Pocket Test",
		"<h1>SYNERGIA API</h1>", "<h1>SYNERGIA POCKET TEST</h1>",
		"Niezależny klient • Windows x64 • v0.13 — powiadomienia i kontrola API", "Własne API • wersja testowa web 0.14",
		"LOKALNIE NA TWOIM KOMPUTERZE", "ODDZIELNA WERSJA TESTOWA",
		"Panel klienta Librus", "Dziennik — własne API",
		"Osobne sesje dwóch kont • bez integracji z Synergia Pocket", "Osobna aplikacja testowa • dwa profile",
		"Hasła i historia powiadomień pozostają tylko w pamięci programu.", "Hasła nie są zapisywane. Sesje i historia powiadomień pozostają w pamięci serwera testowego.",
		"Dane wpisujesz tylko w tej lokalnej aplikacji. Klient łączy się bezpośrednio z domenami Librusa, bez serwera Render.", "Zaloguj się danymi Synergii. Aplikacja łączy się z Librusem przez nasz własny klient. Hasło nie jest zapisywane.",
		"Zamknij program", "Wyloguj oba profile",
		"Zamknąć lokalny program SYNERGIA API?", "Wylogować oba profile z aplikacji testowej?",
		"Program został zamknięty. Możesz zamknąć tę kartę.", "Wylogowano. Odśwież stronę, aby zalogować się ponownie.",
		"Uruchomiono program.", "Otworzono aplikację testową.",
		"Witaj w SYNERGIA API", "Witaj w Synergia Pocket Test",
		"Nowy klient został uruchomiony. To nie jest obecna Synergia Pocket — tamta aplikacja działa niezależnie.", "Zaloguj profil, aby sprawdzić własne API. Dotychczasowa Synergia Pocket działa pod swoim dotychczasowym adresem.",
		"Połączenie lokalne: tylko 127.0.0.1 • brak publicznego serwera", "Połączenie HTTPS • oddzielne sesje przeglądarek",
		"Nieprawidłowa odpowiedź lokalnego serwera.", "Nieprawidłowa odpowiedź serwera.",
		"Sesja istnieje tylko w pamięci tego programu.", "Sesja testowa istnieje w pamięci serwera. Restart lub uśpienie serwera może wymagać ponownego logowania.",
		"Sprawdzanie co 5 minut, gdy ten program jest uruchomiony.", "Sprawdzanie co 5 minut podczas pracy serwera testowego.",
		"To powiadomienia wewnątrz programu Windows. Po zamknięciu programu historia znika. Nie są to powiadomienia push na iPhone.", "Powiadomienia są widoczne w aplikacji. Restart lub uśpienie serwera może wyczyścić historię i wymagać ponownego logowania. Powiadomienia push iPhone nie są włączone w tej wersji testowej.",
	}
	page = strings.NewReplacer(replacements...).Replace(page)
	pwa := `<meta name="theme-color" content="#0d1b2c"><meta name="apple-mobile-web-app-capable" content="yes"><meta name="apple-mobile-web-app-status-bar-style" content="black-translucent"><meta name="apple-mobile-web-app-title" content="Pocket Test"><link rel="manifest" href="/manifest.webmanifest"><link rel="apple-touch-icon" href="/apple-touch-icon.png"><link rel="icon" href="/icon.svg"><style>body{padding-bottom:env(safe-area-inset-bottom)}header{padding-top:max(18px,env(safe-area-inset-top))}.brand h1{font-size:clamp(17px,3vw,26px)}.subtitle{line-height:1.6}@media(max-width:700px){.nav button{min-height:44px;white-space:normal}.notice{font-size:12px}}.cloud-hint{padding:12px 16px;color:#bdd3e9;background:#112137;border:1px solid #304c69;border-radius:10px;line-height:1.6;margin-bottom:16px}</style>`
	page = strings.Replace(page, "</head>", pwa+"</head>", 1)
	page = strings.Replace(page, `<main>`, `<main><p class="cloud-hint">Na iPhonie: Safari → Udostępnij → Do ekranu głównego. To osobna aplikacja testowa; logowanie wykonujesz tutaj niezależnie.</p>`, 1)
	page = librusGUI(page)
	return strings.Replace(page, "</body>", `<script>if('serviceWorker' in navigator&&location.protocol==='https:'){navigator.serviceWorker.register('/sw.js').catch(()=>{});}</script></body>`, 1)
}
func runCloud() error {
	origin := os.Getenv("PUBLIC_ORIGIN")
	if origin == "" {
		if host := os.Getenv("RENDER_EXTERNAL_HOSTNAME"); host != "" {
			origin = "https://" + host
		}
	}
	u, e := url.Parse(origin)
	if e != nil || u.Host == "" || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return errors.New("PUBLIC_ORIGIN must be the HTTPS application origin")
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "10000"
	}
	n, e := strconv.Atoi(port)
	if e != nil || n < 1 || n > 65535 {
		return errors.New("invalid PORT")
	}
	app := newCloudApp(origin)
	defer app.close()
	server := &http.Server{Addr: net.JoinHostPort("0.0.0.0", port), Handler: app, ReadHeaderTimeout: 8 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 32 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		timer := time.NewTicker(time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				shutdown, cancel := context.WithTimeout(context.Background(), 25*time.Second)
				defer cancel()
				server.Shutdown(shutdown)
				return
			case <-timer.C:
				app.prune()
			}
		}
	}()
	log.Print("Synergia Pocket Test web-0.16 listening; own API client enabled")
	e = server.ListenAndServe()
	if errors.Is(e, http.ErrServerClosed) {
		return nil
	}
	return e
}
func main() {
	if err := runCloud(); err != nil {
		log.Fatal(err)
	}
}

// Assert bundled assets are a filesystem at compile time.
var _ fs.FS = cloudAssets
