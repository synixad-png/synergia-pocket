# Podtrzymanie serwera Synergia Pocket w godzinach szkolnych

Workflow `.github/workflows/keep-warm.yml` wysyła żądanie GET do
`https://synergia-pocket.onrender.com/api/health` w dni od poniedziałku do piątku
o 07:03, 07:13, ..., 17:53 czasu Europe/Warsaw.

## Ograniczenia

- Jest to mechanizm **best effort**, nie gwarancja działania 24/7. GitHub
  może opóźnić albo pominąć zaplanowane wywołania.
- Poza wyznaczonymi godzinami Render Free może zasypiać po 15 minutach
  braku ruchu, co jest zamierzone. Pracę usługi mogą też przerwać restarty.
- Celowo nie ma podtrzymania 24/7. GitHub Actions w prywatnym repozytorium
  rozlicza każde uruchomienie za co najmniej minutę. Całodobowa praca co
  10 minut generowałaby 4320–4464 uruchomień na miesiąc i mogłaby
  przekroczyć 2000 darmowych minut GitHub Free.
- Dzienny zakres to maksymalnie 66 uruchomień na dzień roboczy
  (1518 na miesiąc obejmujący 23 dni robocze), przy czym te minuty
  współdzielą budżet z testami projektu i innymi workflows.
- Serwis Render Free ma 750 darmowych instance-hours na workspace
  miesięcznie. Inne usługi Render współdzielą ten limit.
- Jest to wyłącznie żądanie publicznego endpointu zdrowia. Nie pobiera
  wiadomości, nie sprawdza prywatnych danych dzieci, nie loguje do Librusa
  i **nie wysyła powiadomień Push**.
- Zarządzaj uruchomieniami i ewentualnymi opłatami w GitHub Settings → Billing
  oraz Render Dashboard → Billing. Ustaw limity wydatków na 0 / blokadę
  po darmowym limicie, jeśli nie chcesz żadnych opłat.
- Wyłączenie: GitHub → Actions → "Synergia Pocket — dzienne podtrzymanie serwera"
  → menu "..." → Disable workflow.
- Ręczny test: ten sam ekran Actions → Run workflow.
