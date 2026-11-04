# SYNERGIA API — niezależny klient (wersja 0.1.0)

Oddzielny projekt eksperymentalny. Nie korzysta z pakietu librus-synergia,
nie importuje kodu Synergia Pocket, nie uruchamia publicznego serwera HTTP
i NIE został podłączony do działającej aplikacji ani do Render.

Jest to nieoficjalna implementacja klienta znanych endpointów Librus Synergia.
Nie jest produktem firmy Librus.

## Stan funkcji

| Funkcja | Stan |
|---|---|
| Logowanie przez portal i OAuth client 46 | Implementacja, testy z fikcyjnymi odpowiedziami |
| Osobna sesja i ciasteczka każdego dziecka | Implementacja, test |
| Bezhasłowy refreshToken ważnej sesji | Implementacja, test |
| Oddzielna sesja Wiadomości i ponowny bootstrap | Implementacja, test |
| Lista wiadomości, liczba nieprzeczytanych | Implementacja, test |
| Oceny i ich kategorie/przedmioty | Implementacja, test |
| Plan lekcji, zajęcia równoległe, zastępstwa | Implementacja, test |
| Frekwencja, terminarz, uwagi | Surowe odpowiedzi JSON |
| Kontakty i wysyłanie/odpowiadanie na wiadomości | Niezaimplementowane |
| Serwer REST, Web Push, trwała sesja | Niezaimplementowane |
| Test logowania na rzeczywistym koncie | Nieprzeprowadzony |

## Uruchomienie

Z folderu SYNERGIA_API_INDEPENDENT wykonaj:

    python -m venv .venv
    python -m pip install -e ".[test]"
    python -m pytest -q

Przykładowe uruchomienie po instalacji:

    python examples/read_account.py

Skrypt pyta interaktywnie o login i hasło, nie zapisuje ich do pliku.

## Zasady bezpieczeństwa

- Każde konto: osobny SynergiaClient, ClientSession i CookieJar.
- Hasło jest przekazywane wyłącznie do login() i nie jest przechowywane.
- Jeżeli nie można odświeżyć sesji, wymagane jest ponowne logowanie.
- OAuth ma walidację domen docelowych przekierowań.
- Żądania kierowane są tylko do jawnie określonych hostów Librusa.
- Odczyt pełnej treści wiadomości oznacza ją jako przeczytaną;
  w tej wersji celowo implementujemy tylko pobieranie podglądów i list.
- Program nic nie zapisuje do konta Librusa.
- Nie generujemy logów z hasłami, cookies ani treścią wiadomości.
- HTTP 403 nie jest automatycznie traktowany jak błędne hasło.
- Nieoficjalny interfejs może się zmienić bez uprzedzenia.
- Dane z rzeczywistych kont nie były użyte do testowania tego kodu.

## Źródła opisu protokołu — nie są zależnością programową

- https://github.com/MichalZaniewicz/librus-synergia/blob/main/docs/authentication.md
- https://github.com/MichalZaniewicz/librus-synergia/blob/main/docs/messages.md
- https://github.com/MichalZaniewicz/librus-synergia/blob/main/docs/endpoints.md

Projekt zachowuje własny kod transportu i modelowania, bez importowania
zewnętrznego SDK. Dane testowe są fikcyjne.

## Oddzielenie od Synergia Pocket

Całość pozostaje na osobnej gałęzi GitHub:

    feature/independent-synergia-api

Nie zmienia gałęzi main, działania iPhone PWA, ustawień Render ani Web Push.
Nie wolno zastępować produkcyjnego klienta bez sprawdzenia na legalnie
posiadanym koncie testowym i kontroli regresji funkcji dla obu córek.

## Następne etapy

1. Rzeczywisty, dobrowolny test logowania i odczytu na wydzielonym koncie.
2. Obsługa pełnych modeli frekwencji, uwag, wydarzeń i planu przedszkolnego.
3. Kontakty i bezpieczne, pojedyncze wysyłanie wiadomości bez automatycznych ponowień.
4. Monitorowanie zmian API, odporność na awarie i opt-in integracja.
