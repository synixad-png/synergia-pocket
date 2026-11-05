# Zapamiętywanie logowania — 5 listopada 2026

Sesja aplikacji ma ważność 365 dni, odnawianą podczas korzystania. Profile odtwarzają się z zaszyfrowanego magazynu po restarcie procesu. Każde konto zachowuje osobne ciasteczka Librusa; przeglądarka otrzymuje wyłącznie losowy identyfikator HttpOnly, Secure i SameSite=Strict. Hasła nie są zapisywane. Wylogowanie unieważnia zapis serwerowy.

Dostęp jest odnawiany bezhasłowo podczas pracy serwera. Nieudane wymuszone odświeżenie nie jest traktowane jako dowód utraty nadal ważnego dostępu. Stara sesja jest weryfikowana rzeczywistą odpowiedzią dostawcy; jego odmowa nadal wymaga logowania. Chwilowa awaria magazynu nie podmienia ciasteczka na nową pustą sesję.

Pocket TEST v0.17 używa oddzielnego prefiksu Redis i własnego 32-bajtowego klucza AES-256-GCM. Nazwy konfiguracji: SYNERGIA_REDIS_URL i SYNERGIA_SESSION_FERNET_KEY; żadna wartość sekretu nie jest częścią repozytorium. W zwykłej Synergii Pocket istniejący format Fernet i klucz pozostają niezmienione.

Ograniczenia: Librus może unieważnić sesję lub odmówić jej odnowienia. Usunięcie ciasteczek, prywatny tryb przeglądarki lub reset darmowego magazynu Redis również może wymagać logowania. Bez przechowywania hasła nie można zapewnić automatycznego zalogowania po każdym takim zdarzeniu. Safari i osobna aplikacja PWA mogą posiadać odrębne magazyny ciasteczek. Obie witryny loguje się niezależnie.

Dla Pocket TEST dotychczasowe profile w RAM mogą wymagać jednego logowania po pierwszym wdrożeniu, aby utworzyć pierwszy zaszyfrowany zapis.
