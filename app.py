"""Synergia Pocket WEB — przeglądarkowy klient Synergia z backendem HTTPS."""
from __future__ import annotations

import asyncio
from contextlib import asynccontextmanager
import logging
import hashlib
import os
import re
import secrets
import time
from dataclasses import dataclass, field
from datetime import date, timedelta
from pathlib import Path
from typing import Any

from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import FileResponse, JSONResponse
from session_store import configured_store
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel, Field

from demo import demo_section, demo_message

BASE = Path(__file__).parent
LOG = logging.getLogger("synergia_pocket")
STORE = configured_store()
RESTORE_LOCK = asyncio.Lock()
@asynccontextmanager
async def lifespan(_app):
    yield
    for token in list(SESSIONS):
        session = SESSIONS.pop(token)
        await close_session_clients(session)
    if STORE is not None:
        await STORE.close()


app = FastAPI(title="Synergia Pocket", docs_url=None, redoc_url=None, openapi_url=None, lifespan=lifespan)
app.mount("/static", StaticFiles(directory=BASE / "static"), name="static")
SECTIONS = {"grades", "timetable", "attendance", "agenda", "homework", "messages", "notes", "announcements"}
SESSION_IDLE = 7 * 24 * 60 * 60  # 7 dni, odnawiane podczas aktywnego korzystania
CACHE_SECONDS = 180
MAX_PROFILES = 2
# Ustaw SYNERGIA_FORCE_HTTPS=1 na hostingu za terminatorem TLS.
FORCE_HTTPS = os.getenv("SYNERGIA_FORCE_HTTPS", "0") == "1"
LOGIN_WINDOW = 600
LOGIN_MAX_ATTEMPTS = 8
LOGIN_TRIES: dict[str, list[float]] = {}


def throttle_login(login: str) -> None:
    """Ogranicz liczbę prób na identyfikator bez przechowywania loginu w logach."""
    key = hashlib.sha256(login.strip().casefold().encode("utf-8")).hexdigest()
    now = time.monotonic()
    past = [ts for ts in LOGIN_TRIES.get(key, []) if now - ts < LOGIN_WINDOW]
    if len(past) >= LOGIN_MAX_ATTEMPTS:
        raise error(429, "Za wiele prób logowania. Spróbuj ponownie później.")
    past.append(now)
    LOGIN_TRIES[key] = past
    if len(LOGIN_TRIES) > 4000:
        for k in list(LOGIN_TRIES):
            LOGIN_TRIES[k] = [t for t in LOGIN_TRIES[k] if now - t < LOGIN_WINDOW]
            if not LOGIN_TRIES[k]:
                del LOGIN_TRIES[k]


@dataclass
class Profile:
    id: str
    name: str
    mode: str
    librus: Any = None
    login_fingerprint: str | None = None
    username: str | None = None  # wyłącznie dla bezhasłowego wznowienia sesji
    cache: dict[str, tuple[float, dict[str, Any]]] = field(default_factory=dict)


@dataclass
class Session:
    profiles: dict[str, Profile] = field(default_factory=dict)
    active_profile_id: str = ""
    last_seen: float = field(default_factory=time.monotonic)
    lock: asyncio.Lock = field(default_factory=asyncio.Lock)

    @property
    def active(self) -> Profile:
        return self.profiles[self.active_profile_id]

    @property
    def mode(self) -> str:
        return self.active.mode

    @property
    def name(self) -> str:
        return self.active.name

    @property
    def librus(self) -> Any:
        return self.active.librus

    @property
    def cache(self) -> dict:
        return self.active.cache


SESSIONS: dict[str, Session] = {}


class LoginBody(BaseModel):
    login: str = Field(min_length=1, max_length=128)
    password: str = Field(min_length=1, max_length=256)
    label: str | None = Field(default=None, max_length=40)


class ProfileIdBody(BaseModel):
    profile_id: str = Field(min_length=1, max_length=80)


class RenameProfileBody(ProfileIdBody):
    label: str = Field(min_length=1, max_length=40)


def login_fingerprint(login: str) -> str:
    return hashlib.sha256(login.strip().casefold().encode("utf-8")).hexdigest()


def clean_label(label: str | None, default: str) -> str:
    value = re.sub(r"[\x00-\x1f\x7f]", " ", label or str(default or "Uczeń")).strip()
    return value[:40] or "Uczeń"


def profile_payload(session: Session) -> dict[str, Any]:
    return {
        "authenticated": True,
        "mode": session.mode,
        "name": session.name,
        "active_profile_id": session.active_profile_id,
        "profiles": [{"id": profile.id, "name": profile.name, "mode": profile.mode}
                     for profile in session.profiles.values()],
    }


async def close_session_clients(session: Session) -> None:
    async with session.lock:
        for profile in list(session.profiles.values()):
            if profile.librus is not None:
                try:
                    await profile.librus.close()
                except Exception:
                    LOG.warning("Problem podczas zamykania sesji Synergia")
        session.profiles.clear()
        session.active_profile_id = ""


def make_profile(mode: str, name: str, librus: Any = None,
                 fingerprint: str | None = None, username: str | None = None) -> Profile:
    return Profile(id=secrets.token_urlsafe(16), name=clean_label(name, "Uczeń"),
                   mode=mode, librus=librus, login_fingerprint=fingerprint, username=username)


def error(status: int, message: str) -> HTTPException:
    return HTTPException(status, message)


def require_origin(request: Request) -> None:
    origin = request.headers.get("origin")
    if request.headers.get("sec-fetch-site") == "cross-site":
        raise error(403, "Nieprawidłowe pochodzenie żądania.")
    if origin:
        # Na hostingu TLS jest zwykle terminowany przed aplikacją Python.
        expected_scheme = "https" if FORCE_HTTPS else request.url.scheme
        expected = f"{expected_scheme}://{request.headers.get('host', '')}"
        if origin.rstrip("/") != expected:
            raise error(403, "Nieprawidłowe pochodzenie żądania.")



def session_snapshot(session: Session) -> dict[str, Any] | None:
    """Serwerowy zapis *aktywnych* ciasteczek API (nigdy haseł, nigdy demo)."""
    if not session.profiles or any(p.mode != "live" or not p.username for p in session.profiles.values()):
        return None
    profiles = []
    for p in session.profiles.values():
        data = getattr(p.librus, "session_data", None)
        if data is None or not getattr(data, "cookies", None):
            return None
        profiles.append({
            "id": p.id, "name": p.name, "mode": p.mode,
            "username": p.username, "login_fingerprint": p.login_fingerprint,
            "session_data": {"cookies": data.cookies, "logged_in_at": data.logged_in_at},
        })
    return {"version": 1, "profiles": profiles, "active_profile_id": session.active_profile_id}


def restored_session(record: dict[str, Any]) -> Session | None:
    """Odtwarza wyłącznie ciasteczka Librusa; hasło pozostaje puste."""
    from librus_synergia import Librus, LibrusSessionData
    entries = record.get("profiles")
    if not isinstance(entries, list) or not 1 <= len(entries) <= MAX_PROFILES:
        return None
    restored = Session()
    for item in entries:
        if not isinstance(item, dict) or item.get("mode") != "live":
            return None
        username = item.get("username")
        profile_id = item.get("id")
        data = item.get("session_data")
        if (not isinstance(username, str) or not username or len(username) > 128
                or not isinstance(profile_id, str) or not 1 <= len(profile_id) <= 80
                or not isinstance(data, dict) or not isinstance(data.get("cookies"), list)
                or not isinstance(data.get("logged_in_at"), (int, float))
                or not data["cookies"] or profile_id in restored.profiles):
            return None
        client = Librus(username, "", session_data=LibrusSessionData(
            cookies=data["cookies"], logged_in_at=data["logged_in_at"]),
            cache_reference_data=True, max_concurrent_requests=3)
        restored.profiles[profile_id] = Profile(
            id=profile_id, name=clean_label(item.get("name"), "Uczeń"),
            mode="live", librus=client, username=username,
            login_fingerprint=login_fingerprint(username))
    active = record.get("active_profile_id")
    if active not in restored.profiles:
        return None
    restored.active_profile_id = active
    return restored


async def save_session(token: str, session: Session) -> None:
    if STORE is None:
        return
    record = session_snapshot(session)
    if record is None:
        return
    try:
        await STORE.save(token, record, SESSION_IDLE)
    except Exception as exc:
        LOG.warning("Nie udało się zapisać sesji: %s", type(exc).__name__)


async def delete_saved_session(token: str) -> None:
    if STORE is None or not token:
        return
    try:
        await STORE.delete(token)
    except Exception as exc:
        LOG.warning("Nie udało się usunąć zapisu sesji: %s", type(exc).__name__)


async def restore_cookie_session(token: str) -> Session | None:
    if STORE is None or not token:
        return None
    async with RESTORE_LOCK:
        if token in SESSIONS:
            return SESSIONS[token]
        try:
            record = await STORE.get(token)
        except Exception as exc:
            LOG.warning("Błąd magazynu sesji: %s", type(exc).__name__)
            raise error(503, "Magazyn sesji chwilowo niedostępny.")
        if record is None:
            return None
        session = restored_session(record)
        if session is None:
            await delete_saved_session(token)
            return None
        # Nie pokazujemy danych profilu, dopóki Librus nie potwierdzi sesji.
        try:
            await session.active.librus.me()
        except Exception as exc:
            await close_session_clients(session)
            from librus_synergia.exceptions import LibrusAuthError
            if isinstance(exc, LibrusAuthError):
                await delete_saved_session(token)
                return None
            LOG.warning("Nie udało się zweryfikować wznawianej sesji: %s", type(exc).__name__)
            raise error(503, "Nie można teraz potwierdzić sesji w Synergii.")
        SESSIONS[token] = session
        return session


async def prune_sessions() -> None:
    now = time.monotonic()
    stale = [token for token, session in list(SESSIONS.items()) if now - session.last_seen > SESSION_IDLE]
    for token in stale:
        session = SESSIONS.pop(token, None)
        if session:
            await close_session_clients(session)
            await delete_saved_session(token)


async def get_session(request: Request) -> Session:
    token = request.cookies.get("synergia_session", "")
    session = SESSIONS.get(token)
    if session is None:
        session = await restore_cookie_session(token)
    if session is None:
        raise error(401, "Sesja wygasła. Zaloguj się ponownie.")
    if time.monotonic() - session.last_seen > SESSION_IDLE:
        SESSIONS.pop(token, None)
        await close_session_clients(session)
        await delete_saved_session(token)
        raise error(401, "Sesja wygasła. Zaloguj się ponownie.")
    session.last_seen = time.monotonic()
    return session


def assert_session_current(request: Request, session: Session) -> None:
    # Chroni przed użyciem sesji zamkniętej w trakcie równoległego żądania.
    if SESSIONS.get(request.cookies.get("synergia_session", "")) is not session or not session.profiles:
        raise error(401, "Sesja wygasła. Zaloguj się ponownie.")


def set_cookie(response: JSONResponse, token: str, request: Request) -> None:
    response.set_cookie("synergia_session", token, httponly=True, secure=FORCE_HTTPS or request.url.scheme == "https",
                        samesite="strict", max_age=SESSION_IDLE, path="/")


def new_session(mode: str, name: str, librus: Any = None,
                username: str | None = None) -> tuple[str, Session]:
    token = secrets.token_urlsafe(32)
    session = Session()
    profile = make_profile(mode, name, librus, username=username)
    session.profiles[profile.id] = profile
    session.active_profile_id = profile.id
    SESSIONS[token] = session
    return token, session


@app.middleware("http")
async def safety_headers(request: Request, call_next):
    response = await call_next(request)
    response.headers["Cache-Control"] = "no-store"
    response.headers["X-Content-Type-Options"] = "nosniff"
    response.headers["X-Permitted-Cross-Domain-Policies"] = "none"
    response.headers["Referrer-Policy"] = "no-referrer"
    response.headers["X-Frame-Options"] = "DENY"
    if FORCE_HTTPS:
        response.headers["Strict-Transport-Security"] = "max-age=31536000; includeSubDomains"
    response.headers["Content-Security-Policy"] = (
        "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; "
        "connect-src 'self'; frame-ancestors 'none'; object-src 'none'; base-uri 'none'; form-action 'self'"
    )
    token = request.cookies.get("synergia_session", "")
    session = SESSIONS.get(token)
    if request.url.path.startswith("/api/") and response.status_code < 400 and session is not None:
        # Sliding cookie: także po odświeżeniu strony, bez dotykania haseł.
        if time.monotonic() - session.last_seen < SESSION_IDLE:
            set_cookie(response, token, request)
            await save_session(token, session)
    return response


@app.get("/api/health")
async def health():
    return {"status": "ok", "service": "synergia-pocket"}


@app.get("/")
async def index():
    return FileResponse(BASE / "static" / "index.html")


@app.get("/api/session")
async def session_info(request: Request):
    token = request.cookies.get("synergia_session", "")
    try:
        session = await get_session(request)
    except HTTPException as exc:
        if exc.status_code == 401:
            return {"authenticated": False}
        raise
    # Biblioteka sama odświeża ciasteczko oauth_token, gdy jest to potrzebne.
    async with session.lock:
        login = getattr(session.active.librus, "login", None) if session.mode == "live" else None
        if callable(login):
            try:
                await login()
            except Exception as exc:
                from librus_synergia.exceptions import LibrusAuthError
                if isinstance(exc, LibrusAuthError):
                    return {"authenticated": False}
                LOG.warning("Błąd odnawiania sesji przy otwarciu: %s", type(exc).__name__)
                # Nie niszczymy ważnego widoku przy chwilowej awarii połączenia.
    return profile_payload(session)


@app.post("/api/demo")
async def start_demo(request: Request):
    require_origin(request)
    await prune_sessions()
    old_token = request.cookies.get("synergia_session", "")
    old = SESSIONS.pop(old_token, None)
    if old:
        await close_session_clients(old)
    await delete_saved_session(old_token)
    token, session = new_session("demo", "Aleksandra Nowak")
    response = JSONResponse(profile_payload(session))
    set_cookie(response, token, request)
    return response


async def connect_librus(login: str, password: str) -> tuple[Any, str]:
    """Tworzy klienta. Hasło nie jest zapisywane na dysku ani wysyłane do frontendowych ustawień."""
    try:
        from librus_synergia import Librus
        from librus_synergia.exceptions import LibrusInvalidCredentialsError, LibrusCaptchaRequiredError, LibrusError
    except ImportError:
        raise error(503, "Nie zainstalowano biblioteki librus-synergia.")
    client = Librus(login.strip(), password, cache_reference_data=True, max_concurrent_requests=3)
    try:
        student = await client.me()
        return client, str(getattr(student, "display_name", "Uczeń") or "Uczeń")
    except LibrusInvalidCredentialsError:
        await client.close()
        raise error(401, "Nieprawidłowy login lub hasło Synergia.")
    except LibrusCaptchaRequiredError:
        await client.close()
        raise error(403, "Synergia wymaga CAPTCHA. Zaloguj się w oficjalnej przeglądarce.")
    except LibrusError as exc:
        await client.close()
        LOG.warning("Problem z połączeniem Synergia: %s", type(exc).__name__)
        raise error(502, f"Błąd Synergia ({type(exc).__name__}). Spróbuj później.")
    except Exception:
        await client.close()
        LOG.warning("Niespodziewany problem podczas logowania do Synergii")
        raise error(502, "Nie udało się połączyć z Synergią.")


@app.post("/api/login")
async def login(body: LoginBody, request: Request):
    require_origin(request)
    await prune_sessions()
    throttle_login(body.login)
    client, name = await connect_librus(body.login, body.password)
    old_token = request.cookies.get("synergia_session", "")
    old = SESSIONS.pop(old_token, None)
    if old:
        await close_session_clients(old)
    await delete_saved_session(old_token)
    token, session = new_session("live", clean_label(body.label, name), client, username=body.login.strip())
    session.active.login_fingerprint = login_fingerprint(body.login)
    response = JSONResponse(profile_payload(session))
    set_cookie(response, token, request)
    await save_session(token, session)
    return response


@app.get("/api/profiles")
async def list_profiles(request: Request):
    return profile_payload(await get_session(request))


@app.post("/api/profiles/add")
async def add_profile(body: LoginBody, request: Request):
    require_origin(request)
    session = await get_session(request)
    async with session.lock:
        assert_session_current(request, session)
        if session.mode == "demo":
            raise error(409, "Wyjdź z trybu demo, aby dodać prawdziwe konto.")
        if len(session.profiles) >= MAX_PROFILES:
            raise error(409, "Można dodać najwyżej dwa profile.")
        fingerprint = login_fingerprint(body.login)
        if any(x.login_fingerprint == fingerprint for x in session.profiles.values()):
            raise error(409, "To konto jest już dodane.")
        throttle_login(body.login)
        client, name = await connect_librus(body.login, body.password)
        profile = make_profile("live", clean_label(body.label, name), client, fingerprint,
                               username=body.login.strip())
        session.profiles[profile.id] = profile
        session.active_profile_id = profile.id
        return profile_payload(session)


@app.post("/api/profiles/switch")
async def switch_profile(body: ProfileIdBody, request: Request):
    require_origin(request)
    session = await get_session(request)
    async with session.lock:
        assert_session_current(request, session)
        if body.profile_id not in session.profiles:
            raise error(404, "Nie znaleziono profilu w tej sesji.")
        session.active_profile_id = body.profile_id
        return profile_payload(session)


@app.post("/api/profiles/remove")
async def remove_profile(body: ProfileIdBody, request: Request):
    require_origin(request)
    session = await get_session(request)
    async with session.lock:
        assert_session_current(request, session)
        if body.profile_id not in session.profiles:
            raise error(404, "Nie znaleziono profilu w tej sesji.")
        if len(session.profiles) <= 1:
            raise error(409, "Ostatni profil można zakończyć przez wylogowanie.")
        profile = session.profiles.pop(body.profile_id)
        if session.active_profile_id == body.profile_id:
            session.active_profile_id = next(iter(session.profiles))
        if profile.librus is not None:
            try:
                await profile.librus.close()
            except Exception:
                LOG.warning("Problem z zamknięciem usuwanego profilu")
        return profile_payload(session)


@app.post("/api/profiles/rename")
async def rename_profile(body: RenameProfileBody, request: Request):
    require_origin(request)
    session = await get_session(request)
    label = body.label.strip()
    if not label:
        raise error(400, "Podaj nazwę profilu.")
    async with session.lock:
        assert_session_current(request, session)
        if body.profile_id not in session.profiles:
            raise error(404, "Nie znaleziono profilu w tej sesji.")
        session.profiles[body.profile_id].name = label
        return profile_payload(session)


@app.post("/api/logout")
async def logout(request: Request):
    require_origin(request)
    token = request.cookies.get("synergia_session", "")
    session = SESSIONS.pop(token, None)
    if session:
        await close_session_clients(session)
    await delete_saved_session(token)
    response = JSONResponse({"ok": True})
    response.delete_cookie("synergia_session", path="/")
    return response


def val(obj: Any, field: str, default: Any = None) -> Any:
    return getattr(obj, field, default)


def lookup(items: dict[Any, Any], key: Any, fallback: str = "") -> str:
    if key is None:
        return fallback
    item = items.get(key, items.get(str(key)))
    if item is None:
        return fallback or f"#{key}"
    return str(getattr(item, "name", item))


async def optional(fetch, default):
    try:
        return await fetch()
    except Exception as exc:
        LOG.warning("Nie udało się pobrać danych pomocniczych: %s", type(exc).__name__)
        return default


async def live_section(client: Any, section: str, week: date | None, mailbox: str) -> dict[str, Any]:
    if section == "grades":
        grades, subjects, categories = await asyncio.gather(
            client.grades(), optional(client.subjects, {}), optional(client.grade_categories, {}))
        grouped: dict[str, list[dict[str, Any]]] = {}
        for g in grades:
            cat = categories.get(val(g, "category_id"))
            name = lookup(subjects, val(g, "subject_id"), "Inny przedmiot")
            grouped.setdefault(name, []).append({
                "value": str(val(g, "value", "")), "date": val(g, "add_date"),
                "category": val(cat, "name", "Ocena"), "weight": val(cat, "weight", None),
                "comments": val(g, "comments", []), "semester": val(g, "semester"),
                "kind": "semestralna" if val(g, "is_semester", False) else
                        "roczna" if val(g, "is_final", False) else
                        "proponowana" if val(g, "is_semester_proposition", False) or val(g, "is_final_proposition", False) else "cząstkowa"
            })
        return {"subjects": [{"name": name, "grades": sorted(items, key=lambda x: x["date"] or "", reverse=True)}
                             for name, items in sorted(grouped.items())], "count": len(grades)}

    if section == "timetable":
        entries, subjects, rooms, teachers = await asyncio.gather(
            client.timetable(week), optional(client.subjects, {}), optional(client.classrooms, {}), optional(client.teachers, {}))
        start = (week or date.today()) - timedelta(days=(week or date.today()).weekday())
        days = []
        for i in range(5):
            day = start + timedelta(days=i)
            lessons = []
            for lesson in entries.get(day, []):
                original = val(lesson, "original")
                lessons.append({
                    "num": val(lesson, "lesson_no"), "start": val(lesson, "hour_from"),
                    "end": val(lesson, "hour_to"),
                    "subject": lookup(subjects, val(lesson, "subject_id"), "Zajęcia"),
                    "teacher": lookup(teachers, val(lesson, "teacher_id"), ""),
                    "room": lookup(rooms, val(lesson, "classroom_id"), ""),
                    "canceled": val(lesson, "is_canceled", False),
                    "substitution": val(lesson, "is_substitution", False),
                    "note": val(lesson, "substitution_note"),
                    "original_subject": lookup(subjects, val(original, "subject_id"), "")
                })
            days.append({"date": day.isoformat(), "lessons": lessons})
        return {"week_start": start.isoformat(), "days": days}

    if section == "attendance":
        entries, types = await asyncio.gather(client.attendances(), optional(client.attendance_types, {}))
        stats = {"present": 0, "absent": 0, "excused": 0, "late": 0, "total": len(entries)}
        records = []
        for a in entries:
            typ = types.get(val(a, "type_id")) or types.get(str(val(a, "type_id")))
            label = val(typ, "name", "Nieznany status")
            is_present = val(typ, "is_presence_kind", None)
            is_excused = val(typ, "is_excused_absence", False)
            if is_present is True:
                status = "late" if "spóź" in label.lower() else "present"
            elif is_excused:
                status = "excused"
            elif is_present is False:
                status = "absent"
            else:
                status = "unknown"
            if status in stats:
                stats[status] += 1
            records.append({"date": val(a, "date"), "lesson": val(a, "lesson_no"), "label": label, "status": status})
        records.sort(key=lambda x: x["date"] or "", reverse=True)
        return {"stats": stats, "records": records[:120]}

    if section == "agenda":
        entries, subjects, categories = await asyncio.gather(
            client.agenda(), optional(client.subjects, {}), optional(client.agenda_categories, {}))
        items = [{"date": val(x, "date"), "title": lookup(categories, val(x, "category_id"), "Wydarzenie"),
                  "description": val(x, "content", ""),
                  "subject": lookup(subjects, val(x, "subject_id"), ""), "id": val(x, "id")}
                 for x in entries]
        return {"items": sorted(items, key=lambda x: x["date"] or "")}

    if section == "homework":
        entries, teachers = await asyncio.gather(client.homework(), optional(client.teachers, {}))
        return {"items": sorted([{"id": val(x, "id"), "topic": val(x, "topic", "Zadanie"),
                   "text": val(x, "text", ""), "date": val(x, "date"), "due_date": val(x, "due_date"),
                   "teacher": lookup(teachers, val(x, "teacher_id"), ""),
                   "attachments": len(val(x, "attachments", []))} for x in entries],
                key=lambda x: x["due_date"] or "9999")}

    if section == "messages":
        entries, unread = await asyncio.gather(client.messages(mailbox, limit=30), optional(client.unread_messages, {}))
        return {"items": [{"id": val(x, "id"), "sender": val(x, "sender_name", ""),
                           "recipient": val(x, "receiver_name", ""), "subject": val(x, "topic", ""),
                           "preview": val(x, "content", ""), "date": val(x, "send_date"),
                           "unread": val(x, "read_date") is None and mailbox == "inbox",
                           "mailbox": mailbox} for x in entries],
                "unread": unread.get("inbox", 0), "mailbox": mailbox}

    if section == "notes":
        entries, teachers, cats = await asyncio.gather(
            client.notes(), optional(client.teachers, {}), optional(client.note_categories, {}))
        return {"items": sorted([{"date": val(x, "date"), "text": val(x, "text"),
               "teacher": lookup(teachers, val(x, "teacher_id"), ""),
               "category": lookup(cats, val(x, "category_id"), "Uwaga"),
               "sentiment": val(x, "sentiment", "neutral")} for x in entries],
               key=lambda x: x["date"] or "", reverse=True)}

    if section == "announcements":
        entries = await client.announcements()
        return {"items": sorted([{"date": val(x, "creation_date"), "title": val(x, "subject"),
                            "text": val(x, "content"), "unread": not val(x, "was_read", True)} for x in entries],
                            key=lambda x: x["date"] or "", reverse=True)}
    raise error(404, "Nieznana zakładka")


@app.get("/api/section/{section}")
async def get_section(section: str, request: Request, week: str | None = None, mailbox: str = "inbox", refresh: bool = False):
    session = await get_session(request)
    if section not in SECTIONS:
        raise error(404, "Nieznana zakładka")
    if mailbox not in ("inbox", "outbox"):
        raise error(400, "Nieznana skrzynka")
    day = None
    if week:
        if not re.fullmatch(r"\d{4}-\d{2}-\d{2}", week):
            raise error(400, "Nieprawidłowa data")
        try:
            day = date.fromisoformat(week)
        except ValueError:
            raise error(400, "Nieprawidłowa data")
        if abs((day - date.today()).days) > 730:
            raise error(400, "Data poza dozwolonym zakresem")
    key = f"{section}:{week or ''}:{mailbox}"
    async with session.lock:
        assert_session_current(request, session)
        # Cache należy do konkretnego profilu, nie do sesji wspólnej.
        profile = session.active
        old = profile.cache.get(key)
        if old and time.monotonic() - old[0] < CACHE_SECONDS and not refresh:
            return old[1]
        try:
            result = demo_section(section, day, mailbox) if profile.mode == "demo" else \
                await live_section(profile.librus, section, day, mailbox)
        except HTTPException:
            raise
        except Exception as exc:
            LOG.warning("Błąd pobierania %s: %s", section, type(exc).__name__)
            raise error(502, f"Nie udało się pobrać sekcji {section} z Synergii ({type(exc).__name__}).")
        profile.cache[key] = (time.monotonic(), result)
        return result


@app.get("/api/message/{message_id}")
async def get_message(message_id: str, request: Request, mailbox: str = "inbox"):
    session = await get_session(request)
    if mailbox not in ("inbox", "outbox") or not 1 <= len(message_id) <= 160:
        raise error(400, "Nieprawidłowe parametry wiadomości")
    async with session.lock:
        assert_session_current(request, session)
        profile = session.active
        if profile.mode == "demo":
            result = demo_message(message_id, mailbox)
        else:
            try:
                msg = await profile.librus.message(message_id, mailbox)
            except Exception as exc:
                LOG.warning("Błąd odczytu wiadomości: %s", type(exc).__name__)
                raise error(502, "Nie udało się pobrać treści wiadomości")
            if msg is None:
                raise error(404, "Nie znaleziono wiadomości")
            result = {"id": val(msg, "id"), "subject": val(msg, "topic"),
                      "sender": val(msg, "sender_name"), "date": val(msg, "send_date"),
                      "body": val(msg, "content"), "attachments": [val(a, "filename") for a in val(msg, "attachments", [])]}
        if not result:
            raise error(404, "Nie znaleziono wiadomości")
        profile.cache.pop(f"messages::{mailbox}", None)
        return result




if __name__ == "__main__":
    import uvicorn
    # Domyślnie WYŁĄCZNIE ten komputer — brak wystawienia haseł do sieci LAN.
    uvicorn.run(app, host=os.getenv("SYNERGIA_HOST", "127.0.0.1"), port=int(os.getenv("PORT", os.getenv("SYNERGIA_PORT", "8765"))))
