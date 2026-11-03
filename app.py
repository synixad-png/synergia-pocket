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
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel, Field

from demo import demo_section, demo_message

BASE = Path(__file__).parent
LOG = logging.getLogger("synergia_pocket")
@asynccontextmanager
async def lifespan(_app):
    yield
    for token in list(SESSIONS):
        session = SESSIONS.pop(token)
        if session.librus:
            await session.librus.close()


app = FastAPI(title="Synergia Pocket", docs_url=None, redoc_url=None, openapi_url=None, lifespan=lifespan)
app.mount("/static", StaticFiles(directory=BASE / "static"), name="static")
SECTIONS = {"grades", "timetable", "attendance", "agenda", "homework", "messages", "notes", "announcements"}
SESSION_IDLE = 60 * 60
CACHE_SECONDS = 180
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
class Session:
    mode: str
    name: str
    librus: Any = None
    last_seen: float = field(default_factory=time.monotonic)
    cache: dict[str, tuple[float, dict[str, Any]]] = field(default_factory=dict)
    lock: asyncio.Lock = field(default_factory=asyncio.Lock)


SESSIONS: dict[str, Session] = {}


class LoginBody(BaseModel):
    login: str = Field(min_length=1, max_length=128)
    password: str = Field(min_length=1, max_length=256)


def error(status: int, message: str) -> HTTPException:
    return HTTPException(status, message)


def require_origin(request: Request) -> None:
    origin = request.headers.get("origin")
    if origin:
        # Na hostingu TLS jest zwykle terminowany przed aplikacją Python.
        expected_scheme = "https" if FORCE_HTTPS else request.url.scheme
        expected = f"{expected_scheme}://{request.headers.get('host', '')}"
        if origin.rstrip("/") != expected:
            raise error(403, "Nieprawidłowe pochodzenie żądania.")


async def prune_sessions() -> None:
    now = time.monotonic()
    stale = [token for token, session in list(SESSIONS.items()) if now - session.last_seen > SESSION_IDLE]
    for token in stale:
        session = SESSIONS.pop(token, None)
        if session and session.librus:
            try:
                await session.librus.close()
            except Exception:
                LOG.warning("Nie udało się zamknąć wygasłej sesji")


async def get_session(request: Request) -> Session:
    token = request.cookies.get("synergia_session", "")
    session = SESSIONS.get(token)
    if session is None or time.monotonic() - session.last_seen > SESSION_IDLE:
        raise error(401, "Sesja wygasła. Zaloguj się ponownie.")
    session.last_seen = time.monotonic()
    return session


def set_cookie(response: JSONResponse, token: str, request: Request) -> None:
    response.set_cookie("synergia_session", token, httponly=True, secure=FORCE_HTTPS or request.url.scheme == "https",
                        samesite="strict", max_age=SESSION_IDLE, path="/")


def new_session(mode: str, name: str, librus: Any = None) -> tuple[str, Session]:
    token = secrets.token_urlsafe(32)
    session = Session(mode, name, librus)
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
    session = SESSIONS.get(token)
    if not session or time.monotonic() - session.last_seen > SESSION_IDLE:
        return {"authenticated": False}
    session.last_seen = time.monotonic()
    return {"authenticated": True, "mode": session.mode, "name": session.name}


@app.post("/api/demo")
async def start_demo(request: Request):
    require_origin(request)
    await prune_sessions()
    old_token = request.cookies.get("synergia_session", "")
    old = SESSIONS.pop(old_token, None)
    if old and old.librus:
        await old.librus.close()
    token, session = new_session("demo", "Aleksandra Nowak")
    response = JSONResponse({"authenticated": True, "mode": session.mode, "name": session.name})
    set_cookie(response, token, request)
    return response


@app.post("/api/login")
async def login(body: LoginBody, request: Request):
    require_origin(request)
    await prune_sessions()
    throttle_login(body.login)
    try:
        from librus_synergia import Librus
        from librus_synergia.exceptions import LibrusInvalidCredentialsError, LibrusCaptchaRequiredError, LibrusError
    except ImportError:
        raise error(503, "Nie zainstalowano librus-synergia. Uruchom instalator zależności z README.")
    client = Librus(body.login.strip(), body.password, cache_reference_data=True, max_concurrent_requests=3)
    try:
        student = await client.me()
        name = getattr(student, "display_name", "Uczeń") or "Uczeń"
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
        LOG.exception("Niespodziewany problem podczas logowania")
        raise error(502, "Nie udało się połączyć z Synergią.")
    # Nie przechowujemy loginu i hasła w plikach; klient i sesja żyją tylko w pamięci procesu.
    old_token = request.cookies.get("synergia_session", "")
    old = SESSIONS.pop(old_token, None)
    if old and old.librus:
        await old.librus.close()
    token, session = new_session("live", name, client)
    response = JSONResponse({"authenticated": True, "mode": session.mode, "name": session.name})
    set_cookie(response, token, request)
    return response


@app.post("/api/logout")
async def logout(request: Request):
    require_origin(request)
    token = request.cookies.get("synergia_session", "")
    session = SESSIONS.pop(token, None)
    if session and session.librus:
        await session.librus.close()
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
        old = session.cache.get(key)
        if old and time.monotonic() - old[0] < CACHE_SECONDS and not refresh:
            return old[1]
        try:
            result = demo_section(section, day, mailbox) if session.mode == "demo" else \
                await live_section(session.librus, section, day, mailbox)
        except HTTPException:
            raise
        except Exception as exc:
            LOG.warning("Błąd pobierania %s: %s", section, type(exc).__name__)
            raise error(502, f"Nie udało się pobrać sekcji {section} z Synergii ({type(exc).__name__}).")
        session.cache[key] = (time.monotonic(), result)
        return result


@app.get("/api/message/{message_id}")
async def get_message(message_id: str, request: Request, mailbox: str = "inbox"):
    session = await get_session(request)
    if mailbox not in ("inbox", "outbox") or not 1 <= len(message_id) <= 160:
        raise error(400, "Nieprawidłowe parametry wiadomości")
    if session.mode == "demo":
        result = demo_message(message_id, mailbox)
    else:
        try:
            msg = await session.librus.message(message_id, mailbox)
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
    session.cache.pop(f"messages::{mailbox}", None)
    return result




if __name__ == "__main__":
    import uvicorn
    # Domyślnie WYŁĄCZNIE ten komputer — brak wystawienia haseł do sieci LAN.
    uvicorn.run(app, host=os.getenv("SYNERGIA_HOST", "127.0.0.1"), port=int(os.getenv("PORT", os.getenv("SYNERGIA_PORT", "8765"))))
