"""Read dated meals from the public Intendent Pro page linked by SP3.

No OCR, authenticated API, provider cookies or pupil data are involved.
Only the explicitly published /jadlospis/<id> links are fetched.
"""
from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor
from datetime import date, datetime, timedelta, timezone
from html.parser import HTMLParser
import json
from pathlib import Path
import re
from urllib.parse import urlsplit

SNAPSHOT = Path(__file__).parent / "data" / "sp3-calendar.json"
MAX_MENUS = 8


def allowed_menu_url(url):
    try:
        u = urlsplit(url)
        return (u.scheme == "https" and u.hostname == "app.intendentpro.pl"
                and u.port in (None, 443) and not u.username and not u.password
                and not u.query and not u.fragment
                and re.fullmatch(r"/jadlospis/[0-9]{1,10}", u.path) is not None)
    except (ValueError, TypeError):
        return False


def period_bounds(text):
    m = re.search(r"(?<!\d)(\d{1,2})(?:\.(\d{1,2})(?:\.(20\d{2}))?)?\s*[-–—]\s*(\d{1,2})\.(\d{1,2})\.(20\d{2})(?!\d)", text)
    if not m:
        return None
    sd, sm, sy, ed, em, ey = m.groups()
    try:
        end = date(int(ey), int(em), int(ed))
        month = int(sm or em)
        year = int(sy or ey) - (1 if not sy and month > int(em) else 0)
        start = date(year, month, int(sd))
        return (start, end) if 0 <= (end-start).days <= 31 else None
    except ValueError:
        return None


class _SchoolLinks(HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.heading = None
        self.heading_parts = []
        self.period = None
        self.links = []

    def handle_starttag(self, tag, attrs):
        if tag in ("h1", "h2", "h3", "h4"):
            self.heading = tag
            self.heading_parts = []
        if tag == "a" and self.period:
            url = dict(attrs).get("href", "")
            if allowed_menu_url(url):
                self.links.append({"url": url, "start": self.period[0].isoformat(),
                                   "end": self.period[1].isoformat()})

    def handle_data(self, value):
        if self.heading:
            self.heading_parts.append(value)

    def handle_endtag(self, tag):
        if tag == self.heading:
            # A new unrelated heading also terminates the previous date scope.
            self.period = period_bounds(" ".join(self.heading_parts))
            self.heading = None


def discover_menus(html, today):
    parser = _SchoolLinks()
    parser.feed(html)
    found = {x["url"]: x for x in parser.links
             if today-timedelta(days=180) <= date.fromisoformat(x["end"]) <= today+timedelta(days=40)}
    return sorted(found.values(), key=lambda x: x["start"], reverse=True)[:MAX_MENUS]


class _PublicPage(HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.page = None

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if tag == "div" and attrs.get("id") == "app" and "data-page" in attrs:
            self.page = json.loads(attrs["data-page"])


class _Text(HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.parts = []
    def handle_data(self, value):
        self.parts.append(value)


def plain(value):
    p = _Text()
    p.feed(str(value))
    return re.sub(r"\s+", " ", "".join(p.parts)).strip()[:500]


def parse_menu(html, source):
    """Fail closed if publication ID, sharing or date bounds do not match."""
    parser = _PublicPage()
    parser.feed(html)
    page = parser.page
    if not isinstance(page, dict) or page.get("component") != "Dashboard/Menus/SharedPage":
        raise ValueError("Nie rozpoznano publicznego jadłospisu.")
    props = page.get("props", {})
    menu = props.get("menu", {})
    expected_id = source["url"].rsplit("/", 1)[-1]
    if (str(menu.get("id")) != expected_id or menu.get("shared") is not True
            or props.get("selected_diet") != "default"
            or menu.get("start") != source["start"] or menu.get("end") != source["end"]):
        raise ValueError("Dane nie odpowiadają publikacji szkoły.")
    start, end = date.fromisoformat(source["start"]), date.fromisoformat(source["end"])
    days = []
    seen = set()
    for day in menu.get("days", [])[:32]:
        when = date.fromisoformat(day.get("day", ""))
        if not start <= when <= end or when in seen:
            raise ValueError("Nieprawidłowe lub powtórzone daty posiłków.")
        seen.add(when)
        meals = []
        for meal in day.get("meals", [])[:10]:
            # This calendar shows the ordinary lunch, not possible diet swaps,
            # ingredients or other meals that may be included by the provider.
            if meal.get("type") != "DINNER":
                continue
            items = meal.get("items", {})
            if isinstance(items, dict):
                items = list(items.values())
            if not isinstance(items, list):
                raise ValueError("Nieprawidłowe dania.")
            ordered = sorted(items, key=lambda x: int(x.get("position", 0)))
            names = [plain(x["name"]) for x in ordered[:30] if x.get("name")]
            meals.extend(x for x in names if x)
        free = day.get("is_free_day") is True
        days.append({"date": when.isoformat(), "items": [] if free else meals,
                     "status": "no_meals" if free else "published" if meals else "unavailable",
                     "source_url": source["url"]})
    if not days:
        raise ValueError("Publikacja nie zawiera dni jadłospisu.")
    return sorted(days, key=lambda x: x["date"])


def load_calendar(html, today, fetch):
    sources = discover_menus(html, today)
    days = {}
    failed = False
    def read(source):
        raw, _ = fetch(source["url"], menu=True)
        return parse_menu(raw.decode("utf-8", "replace"), source)
    if sources:
        with ThreadPoolExecutor(max_workers=3) as pool:
            jobs = [(s, pool.submit(read, s)) for s in sources]
            for source, job in jobs:
                try:
                    for entry in job.result():
                        # Newest school publication wins if date ranges overlap.
                        days.setdefault(entry["date"], entry)
                except Exception:
                    failed = True
    return {"days": sorted(days.values(), key=lambda x: x["date"]),
            "status": "partial" if days and failed else "ok" if days else "unavailable",
            "checked_at": datetime.now(timezone.utc).isoformat()}


def saved_calendar():
    """A dated, verified public snapshot, never shifted to another week."""
    try:
        data = json.loads(SNAPSHOT.read_text(encoding="utf-8"))
        return {"days": data["days"], "checked_at": data["checked_at"], "status": "saved"}
    except (OSError, ValueError, KeyError):
        return {"days": [], "checked_at": None, "status": "unavailable"}
