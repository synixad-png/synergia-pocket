"""Read public SP3 lunch fees; keep each notice and its billing month separate."""
from __future__ import annotations

import base64
from concurrent.futures import ThreadPoolExecutor
from datetime import date, datetime, timedelta, timezone
from decimal import Decimal, InvalidOperation
from hashlib import sha256
from html.parser import HTMLParser
import json
from pathlib import Path
import re
import unicodedata
from urllib.parse import quote

NEWS_URL = "https://sp3wroc.edupage.org/news/"
SNAPSHOT = Path(__file__).parent / "data" / "sp3-payments.json"
MONTH_NAMES = ("styczeń", "luty", "marzec", "kwiecień", "maj", "czerwiec",
               "lipiec", "sierpień", "wrzesień", "październik", "listopad", "grudzień")
MONTHS = [r"stycz(?:en|nia|niu)", r"lut(?:y|ego|ym)", r"mar(?:zec|ca|cu)",
          r"kwie(?:cien|tnia|tniu)", r"maj(?:a|u)?", r"czerw(?:iec|ca|cu)",
          r"lip(?:iec|ca|cu)", r"sierp(?:ien|nia|niu)", r"wrzes(?:ien|nia|niu)",
          r"pazdziernik(?:a|u)?", r"listopad(?:a|zie)?", r"grud(?:zien|nia|niu)"]
ROMAN = ("I", "II", "III", "IV", "V", "VI", "VII", "VIII", "IX", "X", "XI", "XII")
MONTH_TOKEN = "(?:" + "|".join(MONTHS) + r"|XII|XI|IX|VIII|VII|VI|IV|III|II|X|V|I|0?[1-9]|1[0-2])"
PERIOD = re.compile(r"\bobiad(?:y|ow)\s*(?:(?:za|w)\s+)?(?:(?:miesiac|mies[.]|m\s*[-–.]\s*c?)\s*)?("+
                    MONTH_TOKEN+r")\s*(?:[./-]\s*)?(20\d{2})\b", re.I)
NUMBER = r"\d{1,5}(?:[.,]\d{1,2})?"
MULTIPLICATION = re.compile(r"(?<!\d)(\d{1,2})\s*obiad(?:ow|y)?\s*[x×*]\s*("+NUMBER+
                            r")\s*(?:zl|pln)\s*=\s*("+NUMBER+r")(?!\d|[.,]\d)", re.I)
TOTAL = re.compile(r"(?:kwota\s+(?:do\s+zaplaty|za\s+obiady)|oplata\s+za\s+obiady)\s*(?:wynosi)?\s*[:=]?\s*("+
                   NUMBER+r")\s*(?:zl|pln)\b", re.I)
UNIT = re.compile(r"(?:koszt|cena)\s+(?:jednego\s+)?obiadu\s*(?:wynosi)?\s*[:=]?\s*("+NUMBER+r")\s*(?:zl|pln)\b",re.I)
DATE_TOKEN = r"(?:\d{1,2}[.]\d{1,2}[.]20\d{2}|20\d{2}-\d{2}-\d{2}|\d{1,2}\s+(?:"+"|".join(MONTHS)+r")\s+20\d{2})"
DEADLINE = re.compile(r"(?:termin\s+(?:platnosci|wplaty)|wplat[aye]\s+do|platne\s+do)\s*[:–-]?\s*(?:od\s+|do\s+)?("+
                      DATE_TOKEN+r")(?:\s*r?[.]?\s*(?:do|[-–])\s*("+DATE_TOKEN+r"))?",re.I)
VOID = {"area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr"}


def normalize(text):
    text = text.replace("ł", "l").replace("Ł", "L")
    return "".join(c for c in unicodedata.normalize("NFKD", text) if not unicodedata.combining(c)).lower()


def clean(text):
    return re.sub(r"\s+", " ", text).strip()


def month_number(text):
    for i, pattern in enumerate(MONTHS, 1):
        if re.fullmatch(pattern, normalize(text)):
            return i
    if text.upper() in ROMAN:
        return ROMAN.index(text.upper())+1
    return int(text)


def parse_date(text):
    if re.fullmatch(r"20\d{2}-\d{2}-\d{2}",text):
        return date.fromisoformat(text)
    if "." in text:
        dd, mm, yyyy = text.split(".")
        return date(int(yyyy), int(mm), int(dd))
    dd, month, yyyy = text.split()
    return date(int(yyyy), month_number(month), int(dd))


def money(value):
    d = Decimal(value.replace(",", "."))
    if not Decimal("0") < d <= Decimal("10000"):
        raise ValueError("Nieprawidłowa kwota")
    return d.quantize(Decimal("0.01"))


def amount_label(value):
    return str(value).replace(".", ",")+" zł" if value is not None else None


def bank_account(text):
    matches = re.findall(r"(?<!\d)(\d{26}|\d{2}(?:[ \u00a0]+\d{4}){6})(?!\d)", text)
    valid=[]
    for match in matches:
        digits=re.sub(r"\s", "", match)
        if int(digits[2:]+"2521"+digits[:2]) % 97 == 1:
            valid.append(digits[:2]+" "+" ".join(digits[i:i+4] for i in range(2,26,4)))
    return valid[0] if len(set(valid)) == 1 else None


class NewsArticles(HTMLParser):
    """Never combine an amount in one news item with dates in another."""
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.stack=[]
        self.current=None
        self.articles=[]

    def handle_starttag(self, tag, attrs):
        attrs=dict(attrs)
        root=(tag=="li" and attrs.get("data-sgid", "").startswith("news_ListItem_1-")) or tag=="article"
        if root and self.current is None:
            self.current={"parts":[],"heading":[],"id":attrs.get("data-listitemid", ""),"depth":len(self.stack)}
        if tag not in VOID:
            self.stack.append(tag)
        elif tag=="br" and self.current:
            self.current["parts"].append(" ")

    def handle_endtag(self, tag):
        if tag in VOID:
            return
        if tag in self.stack:
            while self.stack:
                if self.stack.pop()==tag:
                    break
        if self.current is not None and len(self.stack)==self.current["depth"]:
            item=self.current
            self.articles.append({"title":clean(" ".join(item["heading"]))[:200],
                                  "text":clean(" ".join(item["parts"]))[:30000], "id":item["id"]})
            self.current=None

    def handle_data(self, value):
        if self.current is None or any(t in self.stack for t in ("script","style","noscript")):
            return
        self.current["parts"].append(value)
        if "h2" in self.stack or "h1" in self.stack:
            self.current["heading"].append(value)


def parse_notice(text, *, title, source_url, checked_at):
    text=clean(text)
    lowered=normalize(text)
    periods={(int(m.group(2)),month_number(m.group(1))) for m in PERIOD.finditer(lowered)}
    if len(periods)!=1:
        return None
    year,month=periods.pop()
    multiplication=list(MULTIPLICATION.finditer(lowered))
    totals=[]
    unit=None
    count=None
    inconsistent=False
    try:
        for m in multiplication:
            n,price,total=int(m.group(1)),money(m.group(2)),money(m.group(3))
            if n>31 or price*n!=total:
                inconsistent=True
                continue
            totals.append(total)
            count,unit=n,price
        totals.extend(money(m.group(1)) for m in TOTAL.finditer(lowered))
        amount=totals[0] if len(set(totals))==1 and not inconsistent else None
        if len({(int(m.group(1)),money(m.group(2))) for m in multiplication})>1:
            amount,unit,count=None,None,None
        if unit is None:
            units={money(m.group(1)) for m in UNIT.finditer(lowered)}
            unit=units.pop() if len(units)==1 else None
    except (InvalidOperation,ValueError):
        amount,unit,count=None,None,None
    deadlines=[]
    for m in DEADLINE.finditer(lowered):
        try:
            first=parse_date(m.group(1))
            last=parse_date(m.group(2)) if m.group(2) else first
            if first<=last and abs((last-date(year,month,1)).days)<=62:
                deadlines.append((first,last))
        except ValueError:
            pass
    window,deadline=deadlines[0] if len(set(deadlines))==1 else (None,None)
    # A promise to announce an amount later is not an actual fee notice.
    if not multiplication and not totals and not deadlines:
        return None
    scope_match=re.search(r"dla\s+klas\s*[:]?\s*([ivx1-8]+\s*[-–]\s*[ivx1-8]+)",lowered)
    scope="Klasy "+re.sub(r"\s*[-–]\s*","–",scope_match.group(1).upper()) if scope_match else "Zakres niepodany w komunikacie"
    period=f"{year:04d}-{month:02d}"
    return {"id":sha256((source_url+period+scope).encode()).hexdigest()[:20],
            "period":period,"period_label":f"{MONTH_NAMES[month-1]} {year}",
            "title":title[:200],"scope":scope,"amount":amount_label(amount),
            "unit_price":amount_label(unit),"meal_count":count,
            "deadline":deadline.isoformat() if deadline else None,
            "window_start":window.isoformat() if window and window!=deadline else None,
            "complete":bool(amount and deadline),"bank_account":bank_account(text),
            "transfer_title":"Imię i nazwisko dziecka oraz klasa" if re.search(r"tytul\w* przelewu.{0,100}imie.{0,100}klas",lowered) else None,
            "deductions":"Odpisy za niewykorzystane obiady rozliczane przy następnej płatności." if re.search(r"odpisy.{0,100}nastepnej platnosci",lowered) else None,
            "source_url":source_url,"checked_at":checked_at,"source_status":"live"}


def parse_news(html, source_url, checked_at):
    parser=NewsArticles()
    parser.feed(html)
    notices=[]
    for article in parser.articles[:100]:
        if not re.search(r"obiad",normalize(article["title"]+article["text"])):
            continue
        source=NEWS_URL+"#news-"+article["id"] if re.fullmatch(r"[0-9]{1,10}",article["id"]) else source_url
        notice=parse_notice(article["text"],title=article["title"],source_url=source,checked_at=checked_at)
        if notice:
            notices.append(notice)
    return notices


def saved_notices():
    try:
        records=json.loads(SNAPSHOT.read_text(encoding="utf-8"))["announcements"]
        return [{**r,"source_status":"saved"} for r in records]
    except (OSError,ValueError,KeyError):
        return []


def summarize(records, today, *, status, checked_at):
    period=today.strftime("%Y-%m")
    enriched=[]
    # The same notice can be repeated in News and on the canteen page.
    unique={}
    for notice in records:
        key=tuple(notice.get(k) for k in ("period","scope","amount","unit_price","meal_count","deadline","bank_account"))
        if key not in unique or notice["source_status"]=="live":
            unique[key]=notice
    for notice in unique.values():
        relation="current" if notice["period"]==period else "archive" if notice["period"]<period else "upcoming"
        enriched.append({**notice,"period_status":relation,
                         "deadline_passed":bool(notice.get("deadline") and notice["deadline"]<today.isoformat())})
    enriched.sort(key=lambda r:r["period"],reverse=True)
    current=[r for r in enriched if r["period_status"]=="current"]
    conflict=any(sum(n["scope"]==r["scope"] for n in current)>1 for r in current)
    verified=len(current)==1 and current[0]["complete"] and current[0]["source_status"]=="live" and status=="ok"
    return {"status":status,"checked_at":checked_at,"billing_month":period,
            "billing_month_label":f"{MONTH_NAMES[today.month-1]} {today.year}",
            "announcements":enriched[:30],"verified":bool(verified),"conflict":conflict,
            "current_amount":current[0]["amount"] if verified else None,
            "current_deadline":current[0]["deadline"] if verified else None,
            "account_balance":None,
            "message":"Kwota z ogłoszenia szkoły przed indywidualnymi odpisami." if current else
                      f"Brak odczytanego komunikatu o opłacie za {MONTH_NAMES[today.month-1]} {today.year}."}


def load_payments(fetch, today):
    previous=(today.replace(day=1)-timedelta(days=1)).strftime("%Y-%m")
    periods=[today.strftime("%Y-%m"),previous]
    urls=[NEWS_URL]+[NEWS_URL+"?eqa="+quote(base64.b64encode(("newsarchivef="+p).encode()).decode(),safe="") for p in periods]
    checked_at=datetime.now(timezone.utc).isoformat()
    by_id={n["id"]:n for n in saved_notices()}
    success=0
    def read(url):
        raw,_=fetch(url)
        html=raw.decode("utf-8","replace")
        if not re.search(r"news_ListItem_1-|<article\b|Archiwum Aktualności",html,re.I):
            raise ValueError("Nie rozpoznano strony aktualności")
        return parse_news(html,url,checked_at)
    with ThreadPoolExecutor(max_workers=3) as pool:
        jobs=[pool.submit(read,u) for u in urls]
        for job in jobs:
            try:
                for notice in job.result():
                    by_id[notice["id"]]=notice
                success+=1
            except Exception:
                pass
    status="ok" if success==len(urls) else "partial" if success else "unavailable"
    return summarize(list(by_id.values()),today,status=status,checked_at=checked_at)
