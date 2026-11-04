"""Independent, minimal Synergia HTTP client.

No dependency on librus-synergia. All protocol URLs in this file reflect
the documented public observation of the unofficial gateway as at 2026-11-04.
No real account was accessed during development.
"""
from __future__ import annotations

import asyncio
import base64
import html
import re
from dataclasses import dataclass
from datetime import date, timedelta, datetime
import json
import time
from typing import Any
from urllib.parse import urljoin, urlsplit
from zoneinfo import ZoneInfo

import aiohttp
from yarl import URL

from .models import Contact, SentMessage, Grade, Lesson, MessagePreview, decode_preview, reference_names

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


class MessageWriteUncertain(SynergiaError):
    def __init__(self, sent: bool | None):
        self.sent = sent
        msg = ('Librus odrzucił wysyłkę.' if sent is False else
               'Brak potwierdzenia: wiadomość mogła zostać wysłana. Sprawdź Wysłane.')
        super().__init__(msg)


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
                    json_body: dict | None = None,
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
        if json_body is not None:
            opts["json"] = json_body
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
            async with self._lock:
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
            unread=mailbox not in ("outbox", "archive/outbox") and item.get("readDate") is None,
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


    async def contacts(self) -> list[Contact]:
        """School employees actually offered to this account."""
        payload = await self._message_get("receivers/types",
                                          params={"includeClass": "true"})
        data = payload.get("data") if isinstance(payload, dict) else None
        groups = data.get("list") if isinstance(data, dict) else None
        if not isinstance(groups, list):
            raise ProtocolError("Nie rozpoznano listy grup odbiorców.")
        offered = []
        for group in groups:
            group_id = group.get("id") if isinstance(group, dict) else None
            if isinstance(group_id, str) and re.fullmatch(r"[A-Za-z][A-Za-z0-9]*", group_id):
                offered.append((group_id, str(group.get("name") or group_id)))
        if len(offered) > 40:
            raise ProtocolError("Niespodziewanie długa lista grup pracowników.")
        subjects_by_id: dict[str, list[str]] = {}
        try:
            subjects = await self._message_get("receivers/student-subjects")
            items = subjects if isinstance(subjects, list) else subjects.get("data", [])
            for item in _items(items):
                key = item.get("teacherIdentifier")
                subject = item.get("subject")
                if key is not None and isinstance(subject, str) and subject:
                    subjects_by_id.setdefault(str(key), []).append(subject)
        except HTTPError as exc:
            if exc.status not in (403, 404):
                raise
        contacts = {}
        for group_id, role in offered:
            try:
                group_payload = await self._message_get(
                    "receivers/groups/school-employees",
                    params={"receiverType": group_id})
            except HTTPError as exc:
                if exc.status in (403, 404):
                    continue
                raise
            entries = group_payload.get("receivers") if isinstance(group_payload, dict) else []
            for item in _items(entries):
                key = item.get("accountId")
                if key is None or not str(key).isdigit():
                    continue
                availability = item.get("availabilityStatus")
                available = (availability.get("isAvailable") if isinstance(availability, dict)
                             else None)
                contact = Contact(
                    account_id=str(key), name=str(item.get("label") or ""),
                    role=role, subjects=tuple(dict.fromkeys(subjects_by_id.get(str(key), []))),
                    available=available if isinstance(available, bool) else None)
                if contact.account_id not in contacts:
                    contacts[contact.account_id] = contact
        return list(contacts.values())

    async def full_message(self, message_id: str, *, confirm_mark_read: bool = False) -> dict:
        """Explicit read action. Upstream may mark this message as read."""
        if not confirm_mark_read:
            raise ValueError("Otworzenie pełnej wiadomości może oznaczyć ją jako przeczytaną.")
        if not str(message_id).isdigit():
            raise ValueError("Nieprawidłowy identyfikator wiadomości.")
        result = await self._message_get(f"inbox/messages/{message_id}")
        value = result.get("data") if isinstance(result, dict) else None
        if not isinstance(value, dict):
            raise ProtocolError("Nieznany format pełnej wiadomości.")
        detail = dict(value)
        detail["decoded_content"] = decode_preview(value.get("Message"))
        return detail

    async def _send_once(self, recipient_ids: list[str], topic: str, body: str,
                         *, original_id: str | None = None) -> SentMessage:
        """One POST only, even after a timeout / ambiguous failure."""
        if not recipient_ids or len(recipient_ids) > 50:
            raise ValueError("Wymagany jest od 1 do 50 odbiorców.")
        if len(set(recipient_ids)) != len(recipient_ids) or any(
                not str(x).isdigit() for x in recipient_ids):
            raise ValueError("Odbiorcy muszą mieć unikatowe numery kont.")
        if not 1 <= len(topic.strip()) <= 200 or not 1 <= len(body.strip()) <= 10000:
            raise ValueError("Temat (1–200) i treść (1–10000) są wymagane.")
        # Confirm that the messages session is valid before an irreversible POST.
        await self._message_get("me")
        encoded = {
            "receivers": {"schoolReceivers": [
                {"accountId": str(value)} for value in recipient_ids]},
            "topic": base64.b64encode(topic.strip().encode("utf-8")).decode("ascii"),
            "content": base64.b64encode(
                html.escape(body.strip().replace("\r\n", "\n"), quote=False)
                .encode("utf-8")).decode("ascii"),
            "storageId": None, "category": "normal",
        }
        path = "messages"
        if original_id is not None:
            if not original_id.isdigit():
                raise ValueError("Nieprawidłowy identyfikator oryginalnej wiadomości.")
            encoded["originalMessageId"] = int(original_id)
            path = "messages/reply"
        try:
            reply = await self._send("POST", MESSAGE_API + "/" + path,
                                     json_body=encoded)
        except NetworkError as exc:
            raise MessageWriteUncertain(sent=None) from exc
        if 400 <= reply.status < 500:
            raise MessageWriteUncertain(sent=False)
        if reply.status >= 500:
            raise MessageWriteUncertain(sent=None)
        if reply.status < 200:
            raise MessageWriteUncertain(sent=None)
        try:
            parsed = json.loads(reply.text) if reply.text.strip() else {}
        except (TypeError, ValueError):
            parsed = {}  # A successful reply can return no JSON body.
        if not isinstance(parsed, dict):
            parsed = {}
        inner = parsed.get("data")
        inner = inner if isinstance(inner, dict) else {}
        status = inner.get("status") or parsed.get("status")
        if parsed.get("errors") or (isinstance(status, str) and
                                    status.casefold() in {"failed", "error", "rejected"}):
            raise MessageWriteUncertain(sent=None)
        message_id = inner.get("messageId", parsed.get("messageId"))
        return SentMessage(message_id=str(message_id) if message_id is not None else None,
                           accepted=True)

    async def send_message(self, recipient_ids: list[str], topic: str, text: str,
                           *, confirm: bool = False) -> SentMessage:
        """Explicit opt-in only. Never retry a POST automatically."""
        if not confirm:
            raise ValueError("Ustaw confirm=True po świadomym zatwierdzeniu wiadomości.")
        # Ensure all recipients are people actually offered by the school.
        allowed = {contact.account_id for contact in await self.contacts()}
        ids = [str(x) for x in recipient_ids]
        if not set(ids).issubset(allowed) or not ids:
            raise ValueError("Odbiorca nie występuje na liście kontaktów szkoły.")
        return await self._send_once(ids, topic, text)

    async def reply_to_message(self, message_id: str, text: str,
                               *, confirm: bool = False) -> SentMessage:
        """User opt-in: opening original marks it as read, then one POST."""
        if not confirm:
            raise ValueError("Odpowiedź wymaga confirm=True.")
        original = await self.full_message(message_id, confirm_mark_read=True)
        if bool(original.get("noReply")):
            raise ValueError("Nie można odpowiedzieć na tę wiadomość.")
        sender_id = str(original.get("senderId") or "")
        if not sender_id.isdigit():
            raise ProtocolError("Nie znamy numeru konta nadawcy.")
        subject = str(original.get("topic") or "Wiadomość")
        if not subject.casefold().startswith("re:"):
            subject = "Re: " + subject
        return await self._send_once([sender_id], subject[:200], text,
                                     original_id=str(message_id))

    async def attendance(self) -> dict[str, Any]:
        """Raw gateway data; parsing of detailed statuses is a future module."""
        return await self._data("Attendances")

    async def agenda(self) -> dict[str, Any]:
        """Raw HomeWorks data, not the separate homework-assignment module."""
        return await self._data("HomeWorks")

    async def notes(self) -> dict[str, Any]:
        return await self._data("Notes")
