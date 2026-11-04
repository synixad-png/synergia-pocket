"""Dated publication contracts; no student accounts or external requests."""
from copy import deepcopy
from datetime import date
from html import escape
import json

import pytest
from fastapi.testclient import TestClient

from app import app
import canteen_sp3 as canteen
import canteen_calendar as calendar

SOURCE = {"url": "https://app.intendentpro.pl/jadlospis/123", "start": "2026-11-02", "end": "2026-11-06"}
SCHOOL = '<h2>02-06.11.2026</h2><a href="'+SOURCE["url"]+'"><img alt="Jadłospis"></a>'
PAGE = {"component": "Dashboard/Menus/SharedPage", "props": {
    "selected_diet": "default", "menu": {"id": 123, "shared": True,
    "start": "2026-11-02", "end": "2026-11-06", "days": [
        {"day": "2026-11-04", "is_free_day": False, "meals": [{"type": "DINNER", "items": {
            "b": {"name": "Naleśniki z <em>twarogiem</em>", "position": 1,
                  "items": [{"name": "Składnik"}], "alternativeDishes": [{"name": "Zamiennik"}]},
            "a": {"name": "Zupa ogórkowa", "position": 0}}},
            {"type": "BREAKFAST", "items": {"a": {"name": "Śniadanie"}}}]},
        {"day": "2026-11-05", "is_free_day": True, "meals": []},
        {"day": "2026-11-06", "is_free_day": False, "meals": []}]}}}


def html(page=PAGE):
    return '<div id="app" data-page="'+escape(json.dumps(page), quote=True)+'"></div>'


def test_dishes_keep_explicit_dates_order_and_only_standard_lunch():
    days = calendar.parse_menu(html(), SOURCE)
    assert days[0]["date"] == "2026-11-04"
    assert days[0]["items"] == ["Zupa ogórkowa", "Naleśniki z twarogiem"]
    assert days[0]["status"] == "published"
    assert days[1]["status"] == "no_meals"
    assert days[2]["status"] == "unavailable"
    assert len(days) == 3  # Do not fill unpublished dates or shift a week.


@pytest.mark.parametrize("key,value", [("id",999),("shared",False),
    ("start","2026-10-26"),("end","2026-11-13")])
def test_reject_publication_that_does_not_match_school_link(key,value):
    page=deepcopy(PAGE)
    page["props"]["menu"][key]=value
    with pytest.raises(ValueError):
        calendar.parse_menu(html(page),SOURCE)


def test_reject_duplicate_dates_diet_and_out_of_period_days():
    for change in ("duplicate","diet","range"):
        p=deepcopy(PAGE)
        if change=="duplicate":p["props"]["menu"]["days"].append(p["props"]["menu"]["days"][0])
        if change=="diet":p["props"]["selected_diet"]="vegetarian"
        if change=="range":p["props"]["menu"]["days"][0]["day"]="2026-11-11"
        with pytest.raises(ValueError):calendar.parse_menu(html(p),SOURCE)


def test_only_official_linked_menu_pages_are_discovered():
    assert calendar.discover_menus(SCHOOL,date(2026,11,4)) == [SOURCE]
    assert calendar.discover_menus('<a href="'+SOURCE["url"]+'">menu</a>',date(2026,11,4)) == []
    assert calendar.discover_menus(SCHOOL,date(2027,11,4)) == []
    for url in ['http://app.intendentpro.pl/jadlospis/123',
        'https://app.intendentpro.pl.evil.test/jadlospis/123',
        'https://app.intendentpro.pl/api/v2/menus/123/details',
        'https://app.intendentpro.pl/jadlospis/123?url=http://localhost',
        'https://user:pass@app.intendentpro.pl/jadlospis/123',
        'https://app.intendentpro.pl:444/jadlospis/123']:
        assert not calendar.allowed_menu_url(url)
    assert not canteen._safe_url(SOURCE['url'])
    assert canteen._safe_url(SOURCE['url'],menu=True)


def test_periods_cross_month_and_year_and_future_is_not_current():
    assert calendar.period_bounds('30.11-04.12.2026') == (date(2026,11,30),date(2026,12,4))
    assert calendar.period_bounds('30.12-03.01.2027') == (date(2026,12,30),date(2027,1,3))
    assert calendar.period_bounds('99-06.11.2026') is None
    future=canteen._parse_page('<h2>09-13.11.2026</h2>',date(2026,11,4))
    assert future['menu']['is_current'] is False


def test_partial_failure_retains_readable_week():
    school=SCHOOL+'<h2>09-13.11.2026</h2><a href="https://app.intendentpro.pl/jadlospis/456">menu</a>'
    calls=[]
    def fetch(url,**kwargs):
        assert kwargs=={'menu':True}
        calls.append(url)
        if url.endswith('/456'):raise ConnectionError('offline')
        return html().encode(), 'text/html'
    result=calendar.load_calendar(school,date(2026,11,4),fetch)
    assert result['status']=='partial'
    assert len(result['days'])==3
    assert len(calls)==2


def test_route_displays_real_calendar_and_is_independent_of_librus(monkeypatch):
    def fetch(url,**kwargs):
        if url==canteen.SCHOOL_URL:return ('<h1>Stołówka</h1>'+SCHOOL).encode(),'text/html'
        assert url==SOURCE['url']
        return html().encode(),'text/html'
    monkeypatch.setattr(canteen,'_fetch',fetch)
    monkeypatch.setattr(canteen,'_cache',{})
    monkeypatch.setattr(canteen,'_cache_at',0)
    with TestClient(app) as client:
        assert client.get('/api/canteen/sp3').status_code==401
        client.post('/api/demo')
        result=client.get('/api/canteen/sp3').json()
        assert result['calendar']['days'][0]['items'][1]=='Naleśniki z twarogiem'
        assert result['calendar']['status']=='ok'
        assert client.get('/api/section/grades').status_code==200


def test_verified_snapshot_has_only_original_dates_and_menu_fields():
    saved=calendar.saved_calendar()
    assert saved['status']=='saved'
    days={d['date']:d for d in saved['days']}
    assert days['2026-11-04']['items']==['Zupa ogórkowa z ziemniakami','Naleśniki z twarogiem',
        'Polewa jogurtowo - owocowa','Woda z cytryną i miętą','Owoc']
    assert len(days)==10
    assert not any('csrf' in str(d) for d in days.values())
