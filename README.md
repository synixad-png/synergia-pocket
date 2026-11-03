# SYNERGIA POCKET WEB v4 — przeglądarka telefonu, bez programu Windows

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

`SYNERGIA_POCKET_WEB_v4_DEMO.html` otwiera się jako lokalna strona HTML — bez serwera i bez logowania. Dane są fikcyjne. **Nie wpisuj tam prawdziwego hasła.**

## ZAKŁADKI

Pulpit, oceny, plan lekcji, frekwencja, terminarz, zadania domowe, wiadomości, uwagi, ogłoszenia. W trybie online jest to klient *tylko do odczytu*. Sam odczyt pełnej wiadomości może oznaczyć ją jako przeczytaną w Librus.

## BEZPIECZEŃSTWO I OGRANICZENIA

- Nie zweryfikowano logowania na rzeczywistym koncie. Oficjalne wsparcie i zgodność z regulaminem Librusa nie są potwierdzone. Użyta jest nieoficjalna biblioteka https://github.com/MichalZaniewicz/librus-synergia (MIT).
- Login/hasło wysyłane są z przeglądarki do **Twojego serwera** przez HTTPS, a następnie do Synergii przez klienta biblioteki. Projekt nie zapisuje ich na dysk, ale proces serwera musi je przetworzyć; mogą znajdować się w pamięci procesu. Publiczne wdrożenie do obsługi danych uczniów wymaga odrębnego audytu zabezpieczeń i oceny zasad przetwarzania danych. Nie traktuj tego kodu jako zaudytowanego produktu produkcyjnego.
- Sesja jest utrzymywana w pamięci pojedynczego procesu: przy restarcie aplikacji na hostingu następuje wylogowanie. Hosting Free może się usypiać, a dostępność cen planów może się zmieniać.
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

**Zasady ochrony danych:** profile przechowywane są tylko w RAM serwera w ramach jednej sesji przeglądarki; osobne klienty Synergia i cache dla każdej córki. Login i hasło trafiają przez HTTPS do serwera do autoryzacji; aplikacja nie zapisuje ich w plikach ani pamięci przeglądarki. Biblioteka uwierzytelniająca może przetrzymywać dane w pamięci procesu. Wspólna sesja przeglądarki daje dostęp do obu profili, więc pożyczając telefon należy się wylogować. Sesja kończy się po godzinie nieaktywności; restart/usypianie bezpłatnego serwera może ją przerwać wcześniej. Po wylogowaniu trzeba wprowadzić dane obu kont na nowo.

**Testy:** `python -m pytest -q` (17 testów backend i kontraktów) oraz opcjonalnie `python tests/browser_check.py` z zainstalowanym Playwright i Chromium (test interfejsu z fikcyjnymi danymi, bez kontaktu z Librusem). Nie wykonano próby logowania na prawdziwych kontach.
