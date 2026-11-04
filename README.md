# Synergia Pocket Test — własny klient API

Osobna aplikacja web/PWA do testowania klienta Go z SYNERGIA API v0.13.
Gałąź `test/independent-pocket-web` i serwis `synergia-pocket-own-api-test`.
Produkcja `main` oraz `synergia-pocket` pozostają bez zmian.

## Funkcje

Dwa profile na każdą sesję przeglądarki. Logowanie do Synergii własnym kodem,
oceny, plan i zastępstwa, frekwencja, terminarz, uwagi, zadania domowe,
wiadomości odebrane/wysłane/pełna treść/nowa wiadomość/odpowiedzi,
kontakty, dokumenty szkoły, pobieranie otrzymanych załączników,
ogłoszenia, dni wolne, obserwowanie zmian i diagnostyka.

Nie korzysta z librus-synergia ani z Pythonowego serwera Pocket.
Kod i wdrożenie są przechowywane na prywatnym GitHubie użytkownika.

Na iPhonie: Safari → Udostępnij → Do ekranu głównego. Własny manifest,
ikony i service worker. Dane dziennika i pobierane pliki nie są zapisywane
w cache offline. Powiadomienia są widoczne w panelu; push nie jest włączony.

## Sesje

Każda przeglądarka otrzymuje oddzielny, losowy identyfikator HttpOnly,
Secure, SameSite=Strict. Każda ma własne dwa profile, ciasteczka Librusa
i historię powiadomień. Kontrola Origin chroni operacje POST. Host jest
ograniczony do PUBLIC_ORIGIN. Wylogowanie usuwa tylko bieżącą sesję;
nie kończy serwera i nie wpływa na inne przeglądarki.

Hasła nie są zapisywane. Sesje są przechowywane wyłącznie w pamięci
i wygasają po 12 godzinach bez aktywności. Restart/uśpienie darmowego
serwera może wymagać ponownego logowania. To osobny serwis testowy,
bez współdzielenia cookies, kluczy lub magazynu sesji z dotychczasowym Pocket.

## Wdrożenie

Dockerfile buduje i testuje kod Go. PORT pochodzi od Render; nasłuch 0.0.0.0.
PUBLIC_ORIGIN musi być dokładnym adresem HTTPS (bez końcowego ukośnika).
Gdy brak zmiennej, używane jest RENDER_EXTERNAL_HOSTNAME.
GET /api/health zwraca status bez logowania i bez żądań do Librusa.

Testy: `cd src && go test -race -count=1 ./... && go vet ./...`.
Testy klienta i izolacji hostowanych sesji korzystają z danych fikcyjnych.
Test prawdziwych danych Synergii wymaga logowania użytkownika na stronie.
W tym wdrożeniu nie wysyłano rzeczywistych wiadomości szkolnych.
