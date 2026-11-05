# Podtrzymywanie Synergia Pocket — Netlify

Od 5 listopada 2026 zadanie działa niezależnie od GitHub Actions.

- Projekt Netlify: synergia-pocket-heartbeat (istniejący darmowy plan zespołu synixad).
- Źródła: gałąź ops/heartbeat-netlify, commit 2382a468c65da83985cd9c0d68031a2f1496392f.
- Funkcja: keep-warm, produkcyjny harmonogram `3,13,23,33,43,53 * * * *` (UTC). Odstęp 10 minut, codziennie i przez całą dobę.
- Cel: GET https://synergia-pocket.onrender.com/api/health. Pocket TEST pozostaje bez podtrzymywania.
- Brak haseł, ciasteczek, danych kont szkolnych i sekretów w zadaniu.
- Sprawdzana jest odpowiedź JSON status=ok, service=synergia-pocket; błędne statusy HTTP powodują błąd funkcji.
- Potwierdzenie: ręczne uruchomienie 16:22:14 i automatyczne 16:23:04 Europe/Warsaw; obydwa zakończone heartbeat ok=true i odpowiadającymi wpisami Render GET /api/health 200 OK.

## Poprzedni mechanizm

Workflow .github/workflows/keep-warm.yml jest wyłączony w GitHub Actions (stan Disabled), ale plik pozostaje dla historii. Jego uruchomienia były blokowane komunikatem GitHub o rozliczeniach lub limicie wydatków. Nie należy go ponownie włączać jako równoległego podstawowego harmonogramu.

## Kontrola i ograniczenia

Panel: https://app.netlify.com/projects/synergia-pocket-heartbeat/functions/keep-warm

Sprawdź status Scheduled, następny termin i wpisy heartbeat ok=true. Wyłączenie: usuń config.schedule z funkcji i opublikuj zmianę na gałęzi ops/heartbeat-netlify.

To mechanizm best effort. Limity Netlify/Render i awarie mogą przerwać podtrzymywanie. Zadanie ma limit żądania 25 s; zimny start Rendera może wymagać kolejnego wykonania. Nie zmieniono planów ani nie włączono płatnych opcji. Render współdzieli darmowe godziny pomiędzy usługami.
