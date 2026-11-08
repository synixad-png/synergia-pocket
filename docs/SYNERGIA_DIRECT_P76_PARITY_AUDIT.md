# SYNERGIA DIRECT / POCKET WEB — funkcje i status (2026-11-08)

Weryfikowano funkcje w repozytorium `synixad-png/synergia-pocket` (main): `app.py`, `grade_features.py`, `message_features.py`, `school_files.py`, `message_attachments.py`, `canteen_sp3.py`, `canteen_calendar.py`, `canteen_payments.py`, `static/app.js`, `static/messaging.js`, `static/canteen-sp3.js`. Baza natywna P76 wywodzi się z P75/P74, bez wycinania dotychczasowych modułów. Zestawienie dotyczy obecności kodu i poziomu weryfikacji, a nie gwarantowanego działania na dowolnej szkole.

| Funkcja Pocket Web | Direct iOS P76 | Stan weryfikacji / ograniczenie |
|---|---|---|
| Połączenie do Synergii i konta rodzica | Natywne logowanie, sesja WebKit + OAuth, przywracanie i osłona danych | P73 poczta potwierdzona przez użytkownika; żywe testy pozostałych API niewykonane |
| Pulpit / oś czasu | Natywny widok główny | Testy źródłowe; faktyczna kompletność z realnego konta niepotwierdzona |
| Oceny standardowe, semestralne, opisowe i punktowe | Natywne ekrany i podgląd szczegółów od P74 | Komentarze/autorzy zależą od rzeczywistego JSON; brak testu na koncie |
| Plan lekcji, tygodnie, odróżnianie minionych zajęć | Natywny widok planu | Bez testu spójności dla konkretnej szkoły |
| Frekwencja, kategorie i statystyki | Natywna | Bez testu spójności dla konkretnej szkoły |
| Terminarz | Natywny | Brak testu rzeczywistych wydarzeń |
| Zadania domowe | Natywne | Brak testu rzeczywistych danych |
| Ogłoszenia | Natywne | Brak testu rzeczywistych ogłoszeń |
| Wiadomości — odebrane, wysłane, zawartość | Natywne | P73 odczyt potwierdzony przez użytkownika; pozostałe skrzynki wymagają prób |
| Wiadomości — nowa, odpowiedź, wątek, zabezpieczenia | Formularz P74 | Nie wykonano żywej wysyłki ani sprawdzenia odbiorcy na koncie; nie oznaczać 100% |
| Załączniki Wiadomości | Istniejące pobieranie i podgląd | Nie potwierdzono pobierania na prawdziwych załącznikach |
| Dokumenty szkoły i pobieranie plików | Istniejące ekrany | Nie potwierdzono dla rzeczywistych dokumentów |
| Uwagi wychowawcze — rodzaje, daty, kategorie, szczegóły | Dodane natywnie P75 | Parser 27/27 PASS; żywy odczyt Notes/Categories nieprzetestowany |
| Stołówka SP3 — jadłospisy wg dat, tygodni | Dodane natywnie P76 (publiczne dane) | Parser 40/40 PASS; aktualny feed SP3 na iOS nieprzetestowany |
| Stołówka SP3 — komunikaty o opłatach i formularze | Dodane publiczne odczyty i linki źródłowe P76 | Natywny parser opłat obejmuje jednoznaczny `YYYY-MM`; **nie ma pełnej równoważności z backendem Pocket**; brak indywidualnego salda (niepubliczne) |
| Odczyt na dwóch profilach | Identyfikacja/izolacja kont i ekranów | Wymaga końcowej weryfikacji dla rzeczywistych dwóch profili |
| Lokalna synchronizacja / offline | Mechanizmy działające w źródłach | Bez testów po restarcie iOS oraz długiej przerwie |
| Powiadomienia lokalne / push | Mechanizmy iOS zaimplementowane | Brak realnych testów dostarczania i zawartości na urządzeniu |
| Motyw, dostępność iPhone/iPad | Natywne SwiftUI, adaptacja ekranów | Build Xcode potwierdza kompilację, nie ergonomię na urządzeniu |
| Serwer backend Pocket Web / Render | Nie przenoszono Python/Render do iOS | Celowo: Direct iOS pozostaje natywny i nie wymaga serwera pośredniego; funkcje zależne od backendu nie mogą być 1:1 bez osobnej implementacji |

## Wyraźnie niedomknięte zadania

1. Rzeczywista wysyłka/odpowiedź, w tym ograniczenia `noReply`, `originalMessageId`, treść i potwierdzenie serwera.
2. Kompletny odczyt i podgląd ocen (w tym komentarze/opisowe/punktowe) na prawdziwym koncie.
3. Weryfikacja załączników i plików szkoły oraz powrotów z ekranów.
4. Publiczna stołówka: pozostałe historyczne formaty komunikatów, płatności i ewentualne datowane zrzuty bieżących jadłospisów; nie wyświetlać niezweryfikowanych kwot.
5. Długotrwałość sesji, wieloprofilowość i testy iOS na realnych urządzeniach.

**Uwaga:** Testy jednostkowe i kompilacja Xcode nie dowodzą zgodności działania z żywą Synergią ani EduPage. Nie wolno oznaczać powyższych pozycji jako w pełni ukończonych bez osobnego dowodu.