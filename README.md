# SYNERGIA POCKET WEB v3 — przeglądarka telefonu, bez programu Windows

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

`SYNERGIA_POCKET_WEB_v3_DEMO.html` otwiera się jako lokalna strona HTML — bez serwera i bez logowania. Dane są fikcyjne. **Nie wpisuj tam prawdziwego hasła.**

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
