"""Publiczna stołówka SP3 Wrocław — moduł niezależny od Librusa.

Źródło jest stałe; nie przekazujemy haseł ani ciasteczek Synergii do EduPage.
Informacje nieaktualne lub niepewne NIE są oznaczane jako bieżące płatności.
"""
from __future__ import annotations

import asyncio
from datetime import date, datetime, timedelta, timezone
from hashlib import sha256
from html import unescape
from html.parser import HTMLParser
import logging
import re
import time
from typing import Any
from urllib.parse import urljoin, urlsplit
from urllib.request import Request, build_opener, HTTPRedirectHandler
from zoneinfo import ZoneInfo

from canteen_calendar import allowed_menu_url, load_calendar, saved_calendar, period_bounds

from fastapi import HTTPException, Request as FastRequest
from fastapi.responses import Response

LOG = logging.getLogger("synergia_pocket.sp3_canteen")
SCHOOL_DOMAIN = "sp3wroc.edupage.org"
SCHOOL_URL = "https://sp3wroc.edupage.org/a/stolowka"
NEWS_URL = "https://sp3wroc.edupage.org/news/"
CACHE_TTL = 30 * 60
MAX_HTML_BYTES = 1_500_000
MAX_IMAGE_BYTES = 6_000_000
MONTH_NAMES = (
    "styczeń", "luty", "marzec", "kwiecień", "maj", "czerwiec",
    "lipiec", "sierpień", "wrzesień", "październik", "listopad", "grudzień"
)
_cache: dict[str, Any] = {}
_cache_at = 0.0
_cache_lock = asyncio.Lock()
_media_index: dict[str, str] = {}


def _safe_url(url: str, *, image=False, menu=False) -> bool:
    if menu:
        return allowed_menu_url(url)
    try:
        u = urlsplit(url)
        host = (u.hostname or "").lower()
        if u.scheme != "https" or u.port not in (None, 443) or u.username or u.password:
            return False
        if image:
            return host == SCHOOL_DOMAIN or re.fullmatch(r"cloud[a-z0-9-]*\.edupage\.org", host) is not None
        return host == SCHOOL_DOMAIN
    except ValueError:
        return False


class _SafeRedirect(HTTPRedirectHandler):
    def __init__(self, *, image=False, menu=False):
        super().__init__()
        self.image = image
        self.menu = menu

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        if not _safe_url(newurl, image=self.image, menu=self.menu):
            raise ValueError("Odrzucono przekierowanie poza dozwoloną domenę szkoły.")
        return super().redirect_request(req, fp, code, msg, headers, newurl)


def _fetch(url: str, *, image=False, menu=False) -> tuple[bytes, str]:
    if not _safe_url(url, image=image, menu=menu):
        raise ValueError("Niedozwolony adres źródła.")
    limit = MAX_IMAGE_BYTES if image else MAX_HTML_BYTES
    opener = build_opener(_SafeRedirect(image=image, menu=menu))
    req = Request(url, headers={
        "User-Agent": "Mozilla/5.0",
        "Referer": "https://sp3wroc.edupage.org/",
        "Accept": "image/avif,image/webp,image/png,image/jpeg,image/gif;q=0.8" if image else "text/html,application/xhtml+xml",
    })
    with opener.open(req, timeout=8) as resp:
        if not _safe_url(resp.url, image=image, menu=menu):
            raise ValueError("Otrzymano odpowiedź z niezaufanej domeny.")
        typ = resp.headers.get("Content-Type", "").split(";", 1)[0].strip().lower()
        if image:
            if typ not in ("image/jpeg", "image/png", "image/webp", "image/gif"):
                raise ValueError("Źródło nie zwróciło obrazu jadłospisu.")
        elif typ not in ("text/html", "application/xhtml+xml", ""):
            raise ValueError("Źródło nie zwróciło strony HTML.")
        declared_size = resp.headers.get("Content-Length")
        if declared_size and declared_size.isdigit() and int(declared_size) > limit:
            raise ValueError("Plik ze strony szkoły przekracza limit.")
        payload = resp.read(limit + 1)
        if len(payload) > limit:
            raise ValueError("Odpowiedź szkoły przekracza limit.")
        return payload, typ


def _clean_text(value: str) -> str:
    return re.sub(r"\s+", " ", unescape(value)).strip()


class _Page(HTMLParser):
    """Pozycje tekstu i zdjęć, bez wykonywania skryptów z obcej strony."""
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.texts: list[tuple[int, str]] = []
        self.images: list[tuple[int, dict]] = []
        self.links: list[tuple[str, str]] = []
        self.counter = 0
        self._link: dict | None = None
        self._omit = 0

    def handle_starttag(self, tag, attrs):
        self.counter += 1
        d = dict(attrs)
        if tag in ("script", "style", "noscript"):
            self._omit += 1
        if tag == "a":
            self._link = {"href": d.get("href", ""), "text": []}
        if tag in ("img", "source"):
            src = d.get("data-src") or d.get("data-original") or d.get("src") or ""
            self.images.append((self.counter, {
                "src": str(src),
                "alt": str(d.get("alt") or d.get("title") or ""),
                "width": str(d.get("width") or ""),
                "height": str(d.get("height") or ""),
            }))

    def handle_endtag(self, tag):
        self.counter += 1
        if tag in ("script", "style", "noscript"):
            self._omit = max(0, self._omit - 1)
        if tag == "a" and self._link:
            label = _clean_text(" ".join(self._link["text"]))
            self.links.append((label, self._link["href"]))
            self._link = None

    def handle_data(self, data):
        if self._omit:
            return
        value = _clean_text(data)
        if value:
            self.counter += 1
            self.texts.append((self.counter, value[:700]))
            if self._link is not None:
                self._link["text"].append(value)


_PERIOD_END = re.compile(r"(?<!\d)(\d{1,2})\s*[.]\s*(\d{1,2})\s*[.]\s*(20\d{2})(?!\d)")
_DAY_RANGE = re.compile(r"(?:\d{1,2}\s*[.]\s*\d{1,2}|\d{1,2})\s*[-–—]\s*\d{1,2}\s*[.]\s*\d{1,2}\s*[.]\s*20\d{2}")


def _parse_period(text: str) -> date | None:
    """Konserwatywnie: koniec zakresu może być datą, ale nie datą publikacji."""
    if not _DAY_RANGE.search(text):
        return None
    match = list(_PERIOD_END.finditer(text))
    if not match:
        return None
    dd, mm, yyyy = match[-1].groups()
    try:
        return date(int(yyyy), int(mm), int(dd))
    except ValueError:
        return None


def _image_link(src: str) -> str | None:
    if not src or src.startswith("data:") or src.startswith("blob:"):
        return None
    url = urljoin(SCHOOL_URL, unescape(src).strip())
    return url if _safe_url(url, image=True) else None


def _find_menu(page: _Page, today: date) -> dict | None:
    candidates = []
    for at, text in page.texts:
        label = text[:120]
        end = _parse_period(label)
        if end is None:
            continue
        # Nie pokazuj jadłospisu sprzed kilku lat, gdy strona go nie aktualizuje.
        if end < today - timedelta(days=180) or end > today + timedelta(days=40):
            continue
        candidates.append((end, at, label))
    if not candidates:
        return None
    end, pos, label = max(candidates, key=lambda x: (x[0], x[1]))
    # Zdjęcie ma następować po nagłówku terminu, bez przypadkowego logo szkoły.
    next_heading = min((at for _, at, _ in candidates if at > pos), default=pos+180)
    pictures = []
    for at, image in page.images:
        if not (pos < at < min(pos + 180, next_heading)):
            continue
        if re.search(r"logo|ikona|avatar|facebook|instagram|banner", image["alt"], re.I):
            continue
        url = _image_link(image["src"])
        if url:
            pictures.append(url)
    image_url = pictures[0] if pictures else None
    return {
        "period": label,
        "period_start": period_bounds(label)[0].isoformat() if period_bounds(label) else None,
        "period_end": end.isoformat(),
        "is_current": bool(period_bounds(label) and period_bounds(label)[0] <= today <= end),
        "image_available": bool(image_url),
        "_image_url": image_url,
    }


def _notice_text(page: _Page) -> list[str]:
    whole = " ".join(text for _, text in page.texts)
    notes = []
    if re.search(r"kart[ayę]\s+zapisu", whole, re.I):
        notes.append("Przed korzystaniem ze stołówki szkoła wymaga złożenia karty zapisu.")
    if re.search(r"wpłat\w*\s+bez\s+złożon\w*\s+kart", whole, re.I):
        notes.append("Sama wpłata bez złożonej karty zapisu nie upoważnia do otrzymania obiadu.")
    return notes


def _find_registration_link(page: _Page) -> str | None:
    for title, href in page.links:
        if ("kart" in title.lower() and ("obiad" in title.lower() or "zapis" in title.lower())):
            url = urljoin(SCHOOL_URL, unescape(href).strip())
            if _safe_url(url):
                return url
    return None


def _parse_page(html: str, today: date) -> dict:
    page = _Page()
    page.feed(html)
    menu = _find_menu(page, today)
    picture = menu.pop("_image_url", None) if menu else None
    image_id = None
    if picture:
        image_id = sha256(picture.encode("utf-8")).hexdigest()[:24]
    if menu:
        menu["image_id"] = image_id
    return {
        "menu": menu,
        "notices": _notice_text(page),
        "registration_url": _find_registration_link(page),
        "_image_url": picture,
    }


def _empty_state() -> dict:
    return {
        "school": "Szkoła Podstawowa nr 3 im. Mariusza Zaruskiego we Wrocławiu",
        "source_url": SCHOOL_URL,
        "news_url": NEWS_URL,
        "status": "unavailable",
        "checked_at": datetime.now(timezone.utc).isoformat(),
        "menu": None,
        "notices": [],
        "registration_url": None,
        "calendar": saved_calendar(),
        "payment": {
            "current_amount": None,
            "current_deadline": None,
            "account_balance": None,
            "verified": False,
            "message": "Brak potwierdzonej kwoty i terminu płatności na bieżący miesiąc. Sprawdź komunikat szkoły; Synergia Pocket nie ma dostępu do indywidualnych rozliczeń.",
        },
    }


def _get_public_data() -> dict:
    result = _empty_state()
    try:
        raw, typ = _fetch(SCHOOL_URL)
        html = raw.decode("utf-8", "replace")
        if not re.search(r"stołówka|stolowka|jadłospis|obiady", html, re.I):
            raise ValueError("Strona szkoły nie zawiera rozpoznawalnej treści stołówki.")
        today = datetime.now(ZoneInfo("Europe/Warsaw")).date()
        parsed = _parse_page(html, today)
        result["status"] = "ok"
        result["menu"] = parsed["menu"]
        result["notices"] = parsed["notices"]
        result["registration_url"] = parsed["registration_url"]
        if parsed["_image_url"] and parsed["menu"]["image_id"]:
            result["_image_url"] = parsed["_image_url"]
        calendar = load_calendar(html, today, _fetch)
        if calendar["days"]:
            result["calendar"] = calendar
    except Exception as exc:
        LOG.warning("Publiczna strona stołówki SP3 chwilowo niedostępna: %s", type(exc).__name__)
    return result


async def _current():
    global _cache_at, _cache, _media_index
    if _cache and time.monotonic() - _cache_at < CACHE_TTL:
        return _cache
    async with _cache_lock:
        if _cache and time.monotonic() - _cache_at < CACHE_TTL:
            return _cache
        info = await asyncio.to_thread(_get_public_data)
        previous = _cache.get("calendar", {})
        if (info.get("calendar", {}).get("status") == "saved"
                and previous.get("days") and previous.get("status") in ("ok", "partial", "stale")):
            info["calendar"] = {**previous, "status": "stale"}
        img = info.pop("_image_url", None)
        _media_index = {info["menu"]["image_id"]: img} if img and info["menu"] else {}
        _cache = info
        _cache_at = time.monotonic()
        return info


def install_canteen_routes(app, get_session, assert_session_current):
    @app.get("/api/canteen/sp3")
    async def sp3_canteen(request: FastRequest, refresh: bool = False):
        session = await get_session(request)
        async with session.lock:
            assert_session_current(request, session)
        # Publiczny moduł nie pobiera profilu, haseł ani żadnych danych z Librusa.
        # refresh nie omija pamięci 30 min — chroni stronę szkoły przed floodem.
        return await _current()

    @app.get("/api/canteen/sp3/menu-image/{image_id}")
    async def sp3_menu_image(image_id: str, request: FastRequest):
        session = await get_session(request)
        async with session.lock:
            assert_session_current(request, session)
        await _current()
        if not re.fullmatch(r"[0-9a-f]{24}", image_id):
            raise HTTPException(400, "Nieprawidłowy identyfikator zdjęcia.")
        url = _media_index.get(image_id)
        if not url:
            raise HTTPException(404, "Nie znaleziono zdjęcia jadłospisu.")
        try:
            data, media_type = await asyncio.to_thread(_fetch, url, image=True)
        except Exception as exc:
            LOG.warning("Nie udało się pobrać zdjęcia jadłospisu SP3: %s", type(exc).__name__)
            raise HTTPException(502, "Nie udało się pobrać zdjęcia ze strony szkoły.") from exc
        if not (data.startswith((b"\xff\xd8\xff", b"\x89PNG\r\n\x1a\n", b"GIF87a", b"GIF89a",
                                 b"RIFF"))):
            raise HTTPException(502, "Źródło nie zwróciło poprawnego obrazu.")
        return Response(data, media_type=media_type, headers={
            "Cache-Control": "private, max-age=300",
            "X-Content-Type-Options": "nosniff",
        })

