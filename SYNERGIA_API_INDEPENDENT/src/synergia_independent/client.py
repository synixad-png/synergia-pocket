"""Independent, minimal Synergia HTTP client.

No dependency on librus-synergia. All protocol URLs in this file reflect
the documented public observation of the unofficial gateway as at 2026-11-04.
No real account was accessed during development.
"""
from __future__ import annotations

import asyncio
from dataclasses import dataclass
from datetime import date, timedelta, datetime
import json
import time
from typing import Any
from urllib.parse import urljoin, urlsplit
from zoneinfo import ZoneInfo

import aiohttp
from yarl import URL

from .models import Grade, Lesson, MessagePreview, decode_preview, reference_names

PORTAL = "https://synergia.librus.pl/loguj/portalRodzina"
OAUTH = "https://api.librus.pl/OAuth/Authorization?client_id=46"
REFRESH = "https://synergia.librus.pl/refreshToken"
DATA = "https://synergia.librus.pl/gateway/api/2.0"
MESSAGE_BOOTSTRAP = "https://synergia.librus.pl/wiadomosci3"
MESSAGE_API = "https://wiadomosci.librus.pl/api"
AUTH_HOSTS = frozenset({"synergia.librus.pl", "api.librus.pl"})
KNOWN_HOSTS = AUTH_HOSTS | {"wiadomosci.librus.pl"}
UA = ("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36")


class SynergiaError(RuntimeError):
    """Base class; no credentials or cookies in exception messages."""


class NetworkError(SynergiaError):
    pass


class AuthenticationRequired(SynergiaError):
    pass


class CaptchaRequired(SynergiaError):
    pass


class MessagesUnavailable(SynergiaError):
    pass


class ProtocolError(SynergiaError):
    pass


class HTTPError(SynergiaError):
    def __init__(self, status: int, operation: str):
        super().__init__(f"Synergia {operation}: HTTP {status}")
        self.status = status


@dataclass(frozen=True)
class Response:
    status: int
    text: str
    location: str | None
    final_url: str
    refreshed_token: bool


def _url_allowed(url: str, hosts: frozenset[str] = KNOWN_HOSTS) -> bool:
    """Guard against credential leakage through hostile OAuth redirects."""
    try:
        p = urlsplit(url)
        return bool(p.scheme == "https" and p.hostname in hosts and
                    p.username is None and p.password is None and
                    p.port in (None, 443) and not p.fragment)
    except ValueError:
        return False


def _json_object(response: Response, operation: str) -> dict[str, Any]:
    try:
        obj = json.loads(response.text.lstrip("\ufeff"))
    except (ValueError, TypeError) as exc:
        raise ProtocolError(f"{operation}: odpowiedź nie jest JSON.") from exc
    if not isinstance(obj, dict):
        raise ProtocolError(f"{operation}: niespodziewany format JSON.")
    return obj


def _items(value: Any) -> list[dict[str, Any]]:
    return [v for v in value if isinstance(v, dict)] if isinstance(value, list) else []


class SynergiaClient:
    """One instance and one isolated cookie jar per account.

    The caller provides a password ONLY to login(). It is never saved
    or retained. Expired sessions requiring a fresh password raise
    AuthenticationRequired; they never retry with an empty password.
    """

    def __init__(self, username: str, *, session: aiohttp.ClientSession | None = None):
        if not username or len(username.strip()) > 128:
            raise ValueError("Podaj poprawny login.")
        self.username = username.strip()
        self._own_session = session is None
        self._session = session or aiohttp.ClientSession(
            cookie_jar=aiohttp.CookieJar(),
            timeout=aiohttp.ClientTimeout(total=30, connect=10),
            headers={"User-Agent": UA},
            trust_env=False,
        )
        self._lock = asyncio.Lock()
        self._authenticated_at = 0.0
        self._messages_ready = False
        self._refresh_failed_at = 0.0
        self._closed = False

    async def __aenter__(self) -> "SynergiaClient":
        return self

    async def __aexit__(self, *exc: Any) -> None:
        await self.close()

    async def close(self) -> None:
        if self._own_session and not self._session.closed:
            await self._session.close()
        self._closed = True

    def _has_cookie(self) -> bool:
        try:
            jar = self._session.cookie_jar.filter_cookies(URL("https://synergia.librus.pl/"))
            return "oauth_token" in jar
        except (AttributeError, ValueError):
            return False

    async def _send(self, method: str, url: str, *, data: dict | None = None,
                    params: dict | None = None, headers: dict | None = None,
                    allowed: frozenset[str] = KNOWN_HOSTS) -> Response:
        if self._closed:
            raise SynergiaError("Klient został zamknięty.")
        if not _url_allowed(url, allowed):
            raise ProtocolError("Zablokowano nieoczekiwany adres Synergii.")
        opts: dict[str, Any] = {"allow_redirects": False}
        if data is not None:
            opts["data"] = data
        if params is not None:
            opts["params"] = params
        if headers is not None:
            opts["headers"] = headers
        try:
            async with self._session.request(method, url, **opts) as response:
                raw = await response.read()
                if len(raw) > 4_000_000:
                    raise ProtocolError("Odpowiedź Synergii przekracza limit 4 MB.")
                body = raw.decode("utf-8-sig", errors="replace")
                return Response(
                    status=response.status,
                    text=body,
                    location=response.headers.get("Location"),
                    final_url=str(response.url),
                    refreshed_token="oauth_token" in response.cookies,
                )
        except (aiohttp.ClientError, asyncio.TimeoutError) as exc:
            raise NetworkError("Brak połączenia z Synergią.") from exc

    async def login(self, password: str) -> None:
        if not isinstance(password, str) or not password:
            raise ValueError("Hasło jest wymagane.")
        async with self._lock:
            self._authenticated_at = 0.0
            self._messages_ready = False
            portal = await self._send("GET", PORTAL, allowed=AUTH_HOSTS)
            if not portal.location:
                raise ProtocolError("Logowanie: brak przekierowania do OAuth.")
            auth_url = urljoin(portal.final_url, portal.location)
            if not _url_allowed(auth_url, AUTH_HOSTS) or urlsplit(auth_url).hostname != "api.librus.pl":
                raise ProtocolError("Nieoczekiwany adres logowania OAuth.")
            await self._send("GET", auth_url, allowed=AUTH_HOSTS)
            headers = {
                "User-Agent": UA, "Origin": "https://api.librus.pl",
                "Referer": OAUTH, "X-Requested-With": "XMLHttpRequest",
                "Sec-Fetch-Dest": "empty", "Sec-Fetch-Mode": "cors",
                "Sec-Fetch-Site": "same-origin", "Accept": "*/*",
            }
            response = await self._send(
                "POST", OAUTH,
                data={"action": "login", "login": self.username, "pass": password},
                headers=headers, allowed=AUTH_HOSTS)
            if any(mark in response.text.casefold() for mark in
                   ("recaptcha", "hcaptcha", "g-recaptcha", "captcha")):
                raise CaptchaRequired("Librus wymaga rozwiązania CAPTCHA.")
            if response.status >= 400:
                raise HTTPError(response.status, "logowanie")
            go_to = _json_object(response, "logowanie").get("goTo")
            if not isinstance(go_to, str) or not go_to:
                raise AuthenticationRequired("Librus odrzucił logowanie lub wymaga dodatkowego kroku.")
            next_url = urljoin(OAUTH, go_to)
            for _ in range(10):
                redirect = await self._send("GET", next_url, allowed=AUTH_HOSTS)
                if any(mark in redirect.text.casefold() for mark in
                       ("recaptcha", "hcaptcha", "g-recaptcha", "captcha")):
                    raise CaptchaRequired("Librus wymaga rozwiązania CAPTCHA.")
                if not redirect.location:
                    break
                next_url = urljoin(redirect.final_url, redirect.location)
            else:
                raise ProtocolError("Za dużo przekierowań logowania.")
            if not self._has_cookie():
                raise AuthenticationRequired("Librus nie utworzył ciasteczka sesji.")
            self._authenticated_at = time.monotonic()
            self._refresh_failed_at = 0.0

    async def refresh(self) -> bool:
        """Returns True only when refreshToken sets a new oauth_token cookie."""
        if not self._authenticated_at:
            return False
        try:
            response = await self._send("GET", REFRESH, allowed=AUTH_HOSTS)
        except (NetworkError, HTTPError):
            self._refresh_failed_at = time.monotonic()
            return False
        ok = response.status == 200 and response.refreshed_token and self._has_cookie()
        if ok:
            self._authenticated_at = time.monotonic()
            self._refresh_failed_at = 0.0
        else:
            self._refresh_failed_at = time.monotonic()
        return ok

    async def _ensure(self) -> None:
        if not self._authenticated_at or not self._has_cookie():
            raise AuthenticationRequired("Wymagane ponowne logowanie do Synergii.")
        if (time.monotonic() - self._authenticated_at > 7200 and
                time.monotonic() - self._refresh_failed_at > 1800):
            await self.refresh()
        if time.monotonic() - self._authenticated_at > 20 * 3600:
            raise AuthenticationRequired("Sesja wymaga ponownego zalogowania.")

    async def _read_json(self, url: str, operation: str, *,
                         params: dict | None = None) -> dict[str, Any]:
        await self._ensure()
        response = await self._send("GET", url, params=params)
        if response.status == 401:
            # GET may safely retry after a successful refresh, never POST.
            async with self._lock:
                if not await self.refresh():
                    raise AuthenticationRequired("Sesja Synergii wygasła.")
            response = await self._send("GET", url, params=params)
        if response.status in (401, 403):
            raise HTTPError(response.status, operation)
        if response.status >= 400:
            raise HTTPError(response.status, operation)
        return _json_object(response, operation)

    async def _data(self, path: str, *, params: dict | None = None) -> dict[str, Any]:
        if not path or path.startswith("/") or "://" in path or ".." in path:
            raise ValueError("Nieprawidłowa ścieżka API.")
        return await self._read_json(f"{DATA}/{path}", path, params=params)

    async def me(self) -> dict[str, Any]:
        return await self._data("Me")

    async def _bootstrap_messages(self) -> None:
        await self._ensure()
        response = await self._send("GET", MESSAGE_BOOTSTRAP)
        if response.status in (401, 403):
            raise AuthenticationRequired("Brak aktywnej sesji wiadomości.")
        if response.status >= 400:
            raise HTTPError(response.status, "wiadomości")
        if "brak dostępu" in response.text.casefold():
            raise MessagesUnavailable("Szkoła nie udostępnia modułu Wiadomości.")
        self._messages_ready = True

    async def _message_get(self, path: str, *, params: dict | None = None) -> Any:
        if not path or path.startswith("/") or ".." in path or "://" in path:
            raise ValueError("Nieprawidłowa ścieżka wiadomości.")
        await self._ensure()
        if not self._messages_ready:
            await self._bootstrap_messages()
        url = f"{MESSAGE_API}/{path}"
        response = await self._send("GET", url, params=params)
        if response.status in (401, 403):
            # The messages login is independent from the main Synergia cookie.
            self._messages_ready = False
            await self._bootstrap_messages()
            response = await self._send("GET", url, params=params)
        if response.status == 401:
            raise AuthenticationRequired("Sesja Wiadomości wygasła.")
        if response.status >= 400:
            raise HTTPError(response.status, "wiadomości")
        try:
            result = json.loads(response.text.lstrip("\ufeff"))
        except ValueError as exc:
            raise ProtocolError("Wiadomości: oczekiwano JSON.") from exc
        if not isinstance(result, (dict, list)):
            raise ProtocolError("Wiadomości: nieprawidłowy format danych.")
        return result

    async def unread_counts(self) -> dict[str, int]:
        response = await self._message_get("inbox/unreadMessagesCount")
        data = response.get("data", {}) if isinstance(response, dict) else {}
        if not isinstance(data, dict):
            raise ProtocolError("Wiadomości: brak licznika.")
        result: dict[str, int] = {}
        for key, val in data.items():
            try:
                result[str(key)] = max(0, int(val or 0))
            except (ValueError, TypeError):
                result[str(key)] = 0
        return result

    async def messages(self, mailbox: str = "inbox", limit: int = 30) -> list[MessagePreview]:
        allowed = {"inbox", "outbox", "notes", "alerts", "substitutions", "absences",
                   "justifications", "trash", "archive/inbox", "archive/outbox"}
        if mailbox not in allowed or not 1 <= limit <= 100:
            raise ValueError("Nieprawidłowa skrzynka lub limit.")
        response = await self._message_get(f"{mailbox}/messages", params={"limit": str(limit)})
        items = response if isinstance(response, list) else response.get("data")
        if not isinstance(items, list):
            raise ProtocolError("Nieznany format listy wiadomości.")
        return [MessagePreview(
            id=str(item["messageId"]), mailbox=mailbox,
            sender=str(item.get("senderName") or
                       " ".join(filter(None, [item.get("senderFirstName"),
                                               item.get("senderLastName")])) or ""),
            subject=str(item.get("topic") or ""),
            preview=decode_preview(item.get("content")),
            date=item.get("sendDate"),
            unread=item.get("readDate") is None,
        ) for item in _items(items) if item.get("messageId") is not None]

    async def subjects(self) -> dict[str, str]:
        payload = await self._data("Subjects")
        return reference_names(payload, "Subjects")

    async def grades(self) -> list[Grade]:
        grades_payload, subjects_payload, categories_payload = await asyncio.gather(
            self._data("Grades"), self._data("Subjects"), self._data("Grades/Categories")
        )
        subjects = reference_names(subjects_payload, "Subjects")
        categories = reference_names(categories_payload, "Categories")
        out = []
        for item in _items(grades_payload.get("Grades")):
            subject = item.get("Subject") or {}
            category = item.get("Category") or {}
            if not isinstance(subject, dict) or not isinstance(category, dict):
                raise ProtocolError("Nieznany format referencji oceny.")
            out.append(Grade(
                id=str(item.get("Id", "")),
                value=str(item.get("Grade") or ""),
                subject=subjects.get(str(subject.get("Id")), "Nieznany przedmiot"),
                category=categories.get(str(category.get("Id")), ""),
                date=item.get("AddDate"),
                semester=item.get("Semester"),
            ))
        return out

    async def timetable(self, day_in_week: date | None = None) -> dict[str, list[Lesson]]:
        school_day = datetime.now(ZoneInfo("Europe/Warsaw")).date()
        week = day_in_week or school_day
        week_start = week - timedelta(days=week.weekday())
        plan_payload, subjects_payload, teachers_payload, rooms_payload = await asyncio.gather(
            self._data("Timetables", params={"weekStart": week_start.isoformat()}),
            self._data("Subjects"), self._data("Users"), self._data("Classrooms")
        )
        subjects = reference_names(subjects_payload, "Subjects")
        teachers = reference_names(teachers_payload, "Users", people=True)
        rooms = reference_names(rooms_payload, "Classrooms")
        timetable = plan_payload.get("Timetable")
        if not isinstance(timetable, dict):
            raise ProtocolError("Plan lekcji: nieznany format danych.")
        result: dict[str, list[Lesson]] = {}
        for day, slots in timetable.items():
            try:
                date.fromisoformat(day)
            except (TypeError, ValueError):
                continue
            if not isinstance(slots, list):
                raise ProtocolError("Plan lekcji: dzień nie jest listą godzin.")
            lessons = []
            for slot in slots:
                if not isinstance(slot, list):
                    raise ProtocolError("Plan lekcji: godzina nie jest listą lekcji.")
                for item in _items(slot):
                    original = item.get("OrgSubject")
                    original_id = original.get("Id") if isinstance(original, dict) else None
                    subject = item.get("Subject")
                    teacher = item.get("Teacher")
                    room = item.get("Classroom")
                    def name(ref: Any, mapping: dict[str, str]) -> str:
                        return mapping.get(str(ref.get("Id"))) if isinstance(ref, dict) else ""
                    lessons.append(Lesson(
                        day=day, number=item.get("LessonNo"),
                        start=item.get("HourFrom"), end=item.get("HourTo"),
                        subject=name(subject, subjects), teacher=name(teacher, teachers),
                        room=name(room, rooms), canceled=bool(item.get("IsCanceled")),
                        substitution=bool(item.get("IsSubstitutionClass")),
                        original_subject=subjects.get(str(original_id), "") if original_id is not None else "",
                        note=item.get("SubstitutionNote") or None,
                    ))
            result[day] = lessons
        return result

    async def attendance(self) -> dict[str, Any]:
        """Raw gateway data; parsing of detailed statuses is a future module."""
        return await self._data("Attendances")

    async def agenda(self) -> dict[str, Any]:
        """Raw HomeWorks data, not the separate homework-assignment module."""
        return await self._data("HomeWorks")

    async def notes(self) -> dict[str, Any]:
        return await self._data("Notes")
