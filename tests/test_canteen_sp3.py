"""Regresje izolowanego modułu publicznej Stołówki SP3.

Brak prawdziwych loginów, połączeń z kontami szkolnymi i fikcyjnych płatności.
"""
import asyncio
from datetime import date, timedelta
from types import SimpleNamespace

import pytest
from fastapi.testclient import TestClient

from app import app, SESSIONS, new_session
import canteen_sp3 as canteen


def _page(ending, image="/cloud/sp3-jadlospis.png"):
    period = f"01-{ending.day:02d}.{ending.month:02d}.{ending.year}"
    return f"""
    <html><body><h1>Stołówka</h1>
    <div><h2>{period}</h2>
    <p><img src="{image}" alt="jadłospis szkolny"></p></div>
    <p>Warunkiem korzystania jest złożenie Karty zapisu na obiady.</p>
    <p>Wpłata bez złożonej karty zapisu nie upoważnia do otrzymania obiadu.</p>
    <a href="/a/stolowka?subpage=1">Pobierz kartę zapisu na obiady</a>
    </body></html>
    """


def test_menu_current_and_archived_period_handling():
    today = date(2026, 11, 4)
    fresh = canteen._parse_page(_page(date(2026, 11, 6)), today)
    assert fresh["menu"]["is_current"] is True
    assert fresh["menu"]["period_end"] == "2026-11-06"
    assert fresh["menu"]["image_available"] is True
    assert fresh["menu"]["image_id"]
    assert fresh["registration_url"].startswith(canteen.SCHOOL_URL)
    assert len(fresh["notices"]) >= 1
    old = canteen._parse_page(_page(date(2026, 9, 4)), today)
    assert old["menu"]["is_current"] is False
    assert old["menu"]["period_end"] == "2026-09-04"


@pytest.mark.parametrize("url,allowed", [
    ("https://sp3wroc.edupage.org/a/stolowka", True),
    ("https://sp3wroc.edupage.org/news/", True),
    ("http://sp3wroc.edupage.org/a/stolowka", False),
    ("https://sp3wroc.edupage.org.evil.test/a/stolowka", False),
    ("https://sp3wroc.edupage.org@evil.test/a/stolowka", False),
    ("https://sp3wroc.edupage.org:444/a/stolowka", False),
    ("https://evil.test/", False),
])
def test_whitelisted_school_urls(url, allowed):
    assert canteen._safe_url(url) is allowed


@pytest.mark.parametrize("url,allowed", [
    ("https://cloud-a.edupage.org/cloud/menu.png", True),
    ("https://cloud6.edupage.org/cloud/menu.png", True),
    ("https://sp3wroc.edupage.org/file/menu.jpg", True),
    ("https://cloud.evil.org/menu.png", False),
    ("https://other-school.edupage.org/menu.png", False),
    ("file:///etc/passwd", False),
    ("http://cloud-a.edupage.org/cloud/menu.png", False),
])
def test_images_restricted_to_school_or_edupage_cloud(url, allowed):
    assert canteen._safe_url(url, image=True) is allowed


def test_untrusted_image_in_html_is_skipped():
    page = _page(date(2026, 11, 6), "https://evil.test/img.png")
    parsed = canteen._parse_page(page, date(2026,11,4))
    assert parsed["menu"]["image_id"] is None
    assert parsed["_image_url"] is None


def test_no_menu_is_not_fabricated():
    parsed = canteen._parse_page("<h2>Stołówka</h2><p>Oplaty za obiady</p>",date(2026, 11, 4))
    assert parsed["menu"] is None


def test_unknown_monthly_payment_is_never_invented():
    result = canteen._empty_state()
    assert result["payment"]["current_amount"] is None
    assert result["payment"]["current_deadline"] is None
    assert result["payment"]["account_balance"] is None
    assert result["payment"]["verified"] is False


def test_source_failure_returns_clear_fallback_without_affecting_other_modules(monkeypatch):
    def unavailable(_url, **kwargs):
        raise ConnectionError("offline")
    monkeypatch.setattr(canteen,"_fetch",unavailable)
    monkeypatch.setattr(canteen,"_cache",{})
    monkeypatch.setattr(canteen,"_cache_at",0.0)
    with TestClient(app) as client:
        assert client.get("/api/canteen/sp3").status_code == 401
        assert client.post("/api/demo").status_code == 200
        r = client.get("/api/canteen/sp3")
        assert r.status_code == 200, r.text
        d = r.json()
        assert d["status"] == "unavailable"
        assert d["menu"] is None
        assert d["payment"]["current_amount"] is None
        assert d["source_url"] == canteen.SCHOOL_URL
        assert client.get("/api/section/grades").status_code == 200
        assert client.get("/api/section/messages").status_code == 200
        assert client.get("/api/canteen/sp3/menu-image/abcd").status_code == 400


def test_public_menu_does_not_call_librus(monkeypatch):
    today = date.today()
    end = today + timedelta(days=2)
    calls = []
    def school_get(url, *, image=False):
        calls.append((url,image))
        if url.startswith(canteen.NEWS_URL):
            return '<html>Archiwum Aktualności</html>'.encode(), 'text/html'
        assert url == canteen.SCHOOL_URL
        return _page(end).encode("utf-8"), "text/html"
    monkeypatch.setattr(canteen,"_fetch",school_get)
    monkeypatch.setattr(canteen,"_cache",{})
    monkeypatch.setattr(canteen,"_cache_at",0.0)
    class Forbidden:
        def __getattr__(self, key):
            raise AssertionError("Moduł stołówki nie może korzystać z Librusa: " + key)
    with TestClient(app) as client:
        token, session = new_session("live", "Profil testowy", Forbidden())
        client.cookies.set("synergia_session",token)
        try:
            result=client.get("/api/canteen/sp3")
            assert result.status_code == 200
            assert result.json()["status"] == "ok"
            assert len(calls) == 4
            assert all(url.startswith('https://sp3wroc.edupage.org/') for url,_ in calls)
            # Cache dla odczytu publicznego, również w drugim zapytaniu.
            assert client.get("/api/canteen/sp3").status_code == 200
            assert len(calls) == 4
        finally:
            SESSIONS.pop(token,None)


def test_image_proxy_only_lists_observed_image_and_checks_mime(monkeypatch):
    today = date.today()
    end = today + timedelta(days=2)
    def fetch(url, *, image=False):
        if not image:
            return _page(end).encode("utf-8"), "text/html"
        assert url == "https://sp3wroc.edupage.org/cloud/sp3-jadlospis.png"
        return b"\x89PNG\r\n\x1a\nFAKE_IMAGE_BYTES", "image/png"
    monkeypatch.setattr(canteen,"_fetch",fetch)
    monkeypatch.setattr(canteen,"_cache",{})
    monkeypatch.setattr(canteen,"_cache_at",0.0)
    with TestClient(app) as client:
        client.post("/api/demo")
        data=client.get("/api/canteen/sp3").json()
        key=data["menu"]["image_id"]
        assert key and len(key)==24
        img=client.get("/api/canteen/sp3/menu-image/"+key)
        assert img.status_code==200
        assert img.content.startswith(b"\x89PNG")
        assert img.headers["content-type"].startswith("image/png")
        assert client.get("/api/canteen/sp3/menu-image/"+"0"*24).status_code==404


def test_source_path_metadata_does_not_leak_into_json(monkeypatch):
    def fake(_url, *, image=False):
        return _page(date.today()+timedelta(days=2)).encode(), "text/html"
    monkeypatch.setattr(canteen,"_fetch",fake)
    monkeypatch.setattr(canteen,"_cache",{})
    monkeypatch.setattr(canteen,"_cache_at",0.0)
    with TestClient(app) as client:
        client.post("/api/demo")
        data=client.get("/api/canteen/sp3").json()
        assert "_image_url" not in data
        assert "password" not in str(data)
        assert data["payment"]["account_balance"] is None


def test_dockerfile_contains_module():
    from pathlib import Path
    text=(Path(__file__).resolve().parents[1]/"Dockerfile").read_text()
    assert "canteen_sp3.py" in text

