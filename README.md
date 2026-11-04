# SYNERGIA POCKET WEB — ciągłość sesji (wersja robocza) — przeglądarka telefonu, bez programu Windows

**Nieoficjalny prototyp do użytku prywatnego**, niepowiązany z firmą Librus.

## SZYBKI START — BEZ KOMEND

Otwórz plik `ZACZNIJ_TUTAJ.html` z paczki w przeglądarce. Jest to prosty przewodnik po wdrożeniu, który podaje właściwe przyciski i otwiera strony GitHub i Render. Nie zbiera haseł.

Żeby logować się do **prawdziwej** Synergii, nie wystarczy lokalny plik HTML. Potrzebny jest **jednorazowy** hosting serwera Python (na przykład Render). Po wdrożeniu na telefonie wpisujesz jedynie URL swojej aplikacji, potem własny login i hasło do *Librus Synergia* (nie do Konta LIBRUS).

1. Załóż bezpłatne konto na https://github.com/ i https://render.com/ (jeśli jeszcze nie masz).
2. W GitHub kliknij `New repository`. Nazwij nowe repozytorium `synergia-pocket`. Wybierz `Private`, **nie zaznaczaj** opcji tworzenia README, `.gitignore` lub licencji. Kliknij `Create repository`.
3. Na stronie nowego repozytorium kliknij `uploading an existing file`. **Rozpakuj** ZIP na komputerze i przeciągnij do GitHub wszystkie pliki i foldery z katalogu głównego paczki (`app.py`, `demo.py`, `Dockerfile`, `render.yaml`, `requirements.txt`, folder `static` itd.). Nie przesyłaj ZIP jako pojedynczego pliku. Kliknij `Commit changes`.
4. Na https://render.com/ zaloguj się, połącz GitHub z Render i nadaj Render dostęp do repozytorium `synergia-pocket`. Otwórz **Deploy to Render** z kreatora `ZACZNIJ_TUTAJ.html` lub `New → Blueprint`, wybierz repozytorium. Sprawdź, że plan to `Free`, a `SYNERGIA_FORCE_HTTPS=1`, następnie zatwierdź utworzenie usługi. W razie braku planu Free sprawdź ofertę i koszty, nie zatwierdzaj płatnego planu bez decyzji.
5. Render wyświetli adres `https://...onrender.com`. Otwórz go na telefonie w Chrome/Safari. Na stronie podaj login i hasło **wyłącznie wtedy, gdy URL jest Twoją wdrożoną usługą HTTPS**.
6. Opcjonalnie: Chrome na Androidzie — menu `⋮` → `Dodaj do ekranu głównego` / `Zainstaluj`. Safari iPhone — `Udostępnij` → `Do ekranu początkowego`.

To jest jednorazowe opublikowanie aplikacji. Codzienne używanie nie wymaga Windows ani Pythona na telefonie.

## TRYB DEMO

Aktualny podgląd `PODGLAD_MOBILNY_OFFLINE.html` znajduje się w paczce wydania i działa lokalnie bez serwera. Dane są fikcyjne. **Nie wpisuj tam prawdziwego hasła.** Starszy `SYNERGIA_POCKET_WEB_v3_DEMO.html` w repozytorium to archiwalny wygląd v3.

## ZAKŁADKI

Pulpit, oceny, plan lekcji, frekwencja, terminarz, zadania domowe, wiadomości, uwagi, ogłoszenia. Dane o ocenach, planie i frekwencji są tylko do odczytu. Moduł **Wiadomości** umożliwia świadomą wysyłkę oraz odpowiedź z dwóch niezależnych profili. Sam odczyt pełnej wiadomości może oznaczyć ją jako przeczytaną w Librus.

## BEZPIECZEŃSTWO I OGRANICZENIA

- Nie zweryfikowano logowania na rzeczywistym koncie. Oficjalne wsparcie i zgodność z regulaminem Librusa nie są potwierdzone. Użyta jest nieoficjalna biblioteka https://github.com/MichalZaniewicz/librus-synergia (MIT).
- Login/hasło wysyłane są z przeglądarki do **Twojego serwera** przez HTTPS, a następnie do Synergii przez klienta biblioteki. Projekt nie zapisuje ich na dysk, ale proces serwera musi je przetworzyć; mogą znajdować się w pamięci procesu. Publiczne wdrożenie do obsługi danych uczniów wymaga odrębnego audytu zabezpieczeń i oceny zasad przetwarzania danych. Nie traktuj tego kodu jako zaudytowanego produktu produkcyjnego.
- Domyślnie sesje nadal działają tylko w RAM, więc uśpienie lub restart darmowego Render może wymagać ponownego logowania. Opcjonalny zewnętrzny Redis umożliwia przywrócenie ciasteczek sesji po restarcie, dopóki Librus uznaje je za ważne. Nie zapisujemy haseł. Szczegóły poniżej.
- Wymagany Python 3.12 na serwerze (Docker sam to instaluje). Nie instalujesz go na telefonie ani swoim Windows.
- Nigdy nie przesyłaj haseł do ChatGPT, repozytorium GitHub ani do plików konfiguracyjnych.

## TESTY DLA DEWELOPERA (nie dla zwykłego użytkownika)

`python -m pytest -q` (testy mock/demo, kontrakt frontend, podstawowe bezpieczeństwo). Nie zastępują testu integracyjnego Synergia i interaktywnego testu na smartfonie.

`python -m uvicorn app:app --host 127.0.0.1 --port 8765` — wyłącznie opcjonalny lokalny test deweloperski.

## DWA OSOBNE KONTA — PRZEŁĄCZANIE PROFILI (v4)

1. Otwórz aplikację i zaloguj się na konto pierwszej córki. Opcjonalnie nadaj jej nazwę w polu „Nazwa pierwszego profilu”.
2. W prawym górnym rogu kliknij nazwę profilu i wybierz „Dodaj drugie konto”.
3. Podaj nazwę oraz oddzielny login i hasło Synergia drugiej córki. Nie wpisuj danych w GitHubie ani Renderze.
4. Od tej chwili kliknięciem nazwy w nagłówku możesz przełączać aktywną córkę. Wyświetlane oceny, plan i wiadomości są przypisane do danego profilu. Można zmienić etykietę profilu lub usunąć jego sesję.

**Zasady ochrony danych:** profile przechowywane są tylko w RAM serwera w ramach jednej sesji przeglądarki; osobne klienty Synergia i cache dla każdej córki. Login i hasło trafiają przez HTTPS do serwera do autoryzacji; aplikacja nie zapisuje ich w plikach ani pamięci przeglądarki. Biblioteka uwierzytelniająca może przetrzymywać dane w pamięci procesu. Wspólna sesja przeglądarki daje dostęp do obu profili, więc pożyczając telefon należy się wylogować. Sesja aplikacji ma obecnie 7-dniowy limit bezczynności, odnawiany przy aktywnym korzystaniu. Bez zewnętrznego Redis restart darmowego serwera nadal wymaga nowego logowania. Z Redis można wznowić sesję tylko do czasu, gdy Librus pozwoli na odświeżenie jej bez hasła. Po wylogowaniu zapis serwerowy jest usuwany.

**Testy:** `python -m pytest -q` (17 testów backend i kontraktów) oraz opcjonalnie `python tests/browser_check.py` z zainstalowanym Playwright i Chromium (test interfejsu z fikcyjnymi danymi, bez kontaktu z Librusem). Nie wykonano próby logowania na prawdziwych kontach.

## Motywy i układ mobilny (v4.1)

Na ekranie logowania oraz w górnym pasku aplikacji kliknij ikonę słońca/księżyca i wybierz: **Jasny, Ciemny lub Automatyczny**. Automatyczny śledzi ustawienie systemu. W przeglądarce zapisywany jest wyłącznie wybór motywu (`localStorage`); brak tokenów i haseł w tym miejscu.

Na telefonie ekran startowy ma oś czasu, kolory rozróżniające typy wpisów, pasek dolnej nawigacji, przełącznik dwóch profili i osobne listy ocen. Projekt jest wizualnie inspirowany konwencją dzienników szkolnych, ale nie kopiuje marki ani kodu Librusa.

Nie wprowadzono sejfu na telefonie ani zapisu haseł. Pozostają dotychczasowe endpointy; opcjonalnie aktywuje się przechowywanie ciasteczek po stronie serwera.

## CIĄGŁOŚĆ SESJI — BEZ SEJFU NA TELEFONIE (GAŁĄŹ ROBOCZA)

- Nasza sesja przeglądarkowa trwa maksymalnie **7 dni bezczynności**; ważność ciasteczka jest odnawiana w trakcie korzystania. Własny limit z 60 minut został wydłużony, ale reset RAM na darmowym Render nadal zrywa sesję, jeśli nie podłączono trwałego magazynu.
- Biblioteka `librus-synergia` sama obsługuje `/refreshToken`. Przy ponownym otwarciu Synergia Pocket żąda odświeżenia ważnej sesji aktywnego konta; nie wymusza ręcznego wylogowania w razie chwilowego problemu sieciowego.
- **Opcjonalny magazyn Redis** zapisuje wyłącznie zaszyfrowane ciasteczka sesji i nazwy profili, bez haseł. Nie są dodawane żadne sekrety do localStorage, sessionStorage, plików HTML, ani do repozytorium.
- Wznowienie z Redis działa wyłącznie, jeśli ważna sesja istnieje po stronie Librusa. Jeśli cookie zostało unieważnione i ponowne logowanie wymaga hasła lub CAPTCHA, trzeba wykonać je ręcznie; aplikacja tego nie obchodzi.
- Użycie Redis jest **dobrowolne**. Nieskonfigurowana opcja zachowuje tryb RAM. Darmowy hosting Render bez trwałego dysku nie gwarantuje odzyskania sesji.

### Uruchomienie opcji Redis (po osobnej konfiguracji serwera)

1. Przygotuj Redis: zewnętrzny serwer z TLS (`rediss://`) albo prywatny Render Key Value w tym samym regionie (`redis://red-...:6379`). Ten drugi działa tylko wewnątrz platformy Render. Przed użyciem oceń warunki przetwarzania danych kont uczniów. **Bezpłatny Render Key Value nie zapisuje danych na dysku, dlatego własny restart tego magazynu nadal może spowodować utratę sesji.**
2. W sekretach środowiskowych usługi Render ustaw `SYNERGIA_REDIS_URL` na adres połączenia, a `SYNERGIA_SESSION_FERNET_KEY` na osobny stały klucz utworzony lokalnie:
   `python -c "from cryptography.fernet import Fernet; print(Fernet.generate_key().decode())"`
3. **Nie publikuj klucza ani URL Redis** w GitHubie, wiadomościach i kodzie. Nie zmieniaj klucza podczas zwykłych wdrożeń, bo utracisz możliwość odczytu zapisanych sesji.
4. Po wdrożeniu zaloguj oba konta ponownie jeden raz. Przy restarcie serwera aplikacja spróbuje wznowić sesje tylko wtedy, gdy Librus nadal je honoruje.

To nie jest produkcyjny audyt bezpieczeństwa ani potwierdzenie logowania z prawdziwymi kontami. Przed użyciem osobnego dostawcy Redis należy ocenić jego zgodność z wymaganiami ochrony danych i zasadami Librusa.

### Testy

`python -m pytest -q` obejmuje testy mock dwóch kont, bez prawdziwych danych, a dodatkowo szyfrowania w Redis, wznowienia po usunięciu RAM, wygaszenia, wylogowania i zachowania bez Redis.

### Wdrożenie 03.11.2026

Instancja darmowego Render Key Value `synergia-pocket-sessions` została utworzona w regionie Frankfurt oraz podłączona do web service `synergia-pocket` przez zmienne środowiskowe hostingu. Sekrety nie są zapisywane w repozytorium. Free Key Value przechowuje stan wyłącznie w pamięci; restart samego Redis oznacza utratę zapisanych sesji. Sprawdzaj w logach startowych komunikat `Magazyn sesji Redis: PING i zapis/odczyt OK`. Bez sprawdzenia prawdziwego konta Librusa nie wolno uznawać 100% bezobsługowego wznawiania logowania za potwierdzone.

## WIADOMOŚCI I KONTAKTY PRACOWNIKÓW SZKOŁY

- Wybierz profil córki, następnie **Wiadomości → Napisz wiadomość**.
- Wyszukaj pracownika dostępnego na liście odbiorców danego konta. Opisy ról i przedmiotów pochodzą z API Librusa. Dla wychowawcy pokazujemy klasę danego dziecka. Nie zgadujemy pozostałych klas nauczycieli, których API nie udostępnia.
- Po wpisaniu tematu i treści wybierz **Sprawdź przed wysłaniem**, potem świadomie potwierdź przyciskiem **Wyślij wiadomość**. Sukces oznacza akceptację przez Librus, nie odczytanie przez odbiorcę.
- Aby odpowiedzieć, otwórz odebraną wiadomość i wybierz **Odpowiedz**. Gdy Librus zabrania odpowiedzi, przycisk jest ukryty.
- Wysyłka nie jest ponawiana automatycznie. Po błędzie bez potwierdzenia najpierw sprawdź **Wysłane**. Limit: 10 prób na godzinę na profil; blokada powtórzenia identyfikatora żądania w pamięci procesu.
- Lista kontaktów może być inna dla każdej córki. W trybie demo nic nie jest wysyłane. Brak wysyłania załączników i obsługi DW/UDW.
- Testy zostały wykonane na symulowanym API. Nie potwierdzono jeszcze prawdziwej wysyłki z konta Synergia ani pełnego audytu bezpieczeństwa.
