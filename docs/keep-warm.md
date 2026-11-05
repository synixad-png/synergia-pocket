# Podtrzymywanie Synergia Pocket — Netlify

Od 5 listopada 2026 zadanie działa niezależnie od GitHub Actions.

- Projekt Netlify: synergia-pocket-heartbeat (istniejący darmowy plan zespołu synixad).
- Źródła: gałąź ops/heartbeat-netlify, commit 545282809863a260624e0edf1a36d8ec52c5fda2.
- Funkcja: keep-warm, produkcyjny harmonogram `3,13,23,33,43,53 * * * *` (UTC). Odstęp 10 minut, codziennie i przez całą dobę.
- Cele: keep-warm → GET https://synergia-pocket.onrender.com/api/health; keep-warm-test → GET https://synergia-pocket-own-api-test.onrender.com/api/health. Każda funkcja ma osobny harmonogram co 10 minut przez całą dobę i osobny limit żądania; błąd jednego serwera nie blokuje drugiego.
- Brak haseł, ciasteczek, danych kont szkolnych i sekretów w zadaniu.
- Sprawdzana jest odpowiedź JSON status=ok i właściwy service (synergia-pocket lub synergia-pocket-own-api); błędne statusy HTTP powodują błąd funkcji.
- Potwierdzenie: ręczne uruchomienie 16:22:14 i automatyczne 16:23:04 Europe/Warsaw; obydwa zakończone heartbeat ok=true i odpowiadającymi wpisami Render GET /api/health 200 OK.

- Pocket TEST: produkcyjne wdrożenie 6aeca588e50896fe4ad0b736, ręczne wykonanie 16:52 Europe/Warsaw zakończone heartbeat-test ok=true. Obie funkcje mają status Scheduled.

## Poprzedni mechanizm

Workflow .github/workflows/keep-warm.yml jest wyłączony w GitHub Actions (stan Disabled), ale plik pozostaje dla historii. Jego uruchomienia były blokowane komunikatem GitHub o rozliczeniach lub limicie wydatków. Nie należy go ponownie włączać jako równoległego podstawowego harmonogramu.

## Kontrola i ograniczenia

Panel: https://app.netlify.com/projects/synergia-pocket-heartbeat/functions/keep-warm

Sprawdź status Scheduled, następny termin i wpisy heartbeat ok=true. Wyłączenie: usuń config.schedule z funkcji i opublikuj zmianę na gałęzi ops/heartbeat-netlify.

To mechanizm best effort. Limity Netlify/Render i awarie mogą przerwać podtrzymywanie. Zadanie ma limit żądania 25 s; zimny start Rendera może wymagać kolejnego wykonania. Nie zmieniono planów ani nie włączono płatnych opcji. Render współdzieli darmowe godziny pomiędzy usługami.
