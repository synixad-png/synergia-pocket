"""Public fee notices: dates and amounts must come from the same announcement."""
from datetime import date
import base64
from urllib.parse import urlsplit, parse_qs

import pytest
from fastapi.testclient import TestClient

from app import app
import canteen_sp3 as canteen
import canteen_payments as payments

CHECKED="2026-11-04T12:00:00+00:00"
TEXT="""Obiady m-c maj 2026 r. Koszt obiadów dla klas: I - VIII wynosi:
20 obiadów x 13,00 zł = 260,00
Termin płatności: od 04.05.2026 r. do 08.05.2026 r.
Wpłaty wyłącznie na konto szkoły: PKO BP S.A. 04 1020 5226 0000 6502 0416 0479
W tytule przelewu należy podać imię i nazwisko dziecka oraz klasę.
Odpisy za niewykorzystane obiady w danym miesiącu rodzic rozlicza przy następnej płatności.
"""


def notice(text=TEXT,source=payments.NEWS_URL+'#news-317'):
    return payments.parse_notice(text,title='Testowy komunikat o obiadach',source_url=source,checked_at=CHECKED)


def article(text=TEXT,ident='317'):
    return '<li data-sgid="news_ListItem_1-test" data-listitemid="'+ident+'"><div><h2>Obiady</h2><p>'+text+'</p></div></li>'


def test_verified_old_school_format_without_inventing_individual_balance():
    n=notice()
    assert (n['period'],n['amount'],n['unit_price'],n['meal_count'])==('2026-05','260,00 zł','13,00 zł',20)
    assert (n['window_start'],n['deadline'])==('2026-05-04','2026-05-08')
    assert n['scope']=='Klasy I–VIII'
    assert n['complete'] is True
    assert n['bank_account']=='04 1020 5226 0000 6502 0416 0479'
    assert 'klasa' in n['transfer_title']
    r=payments.summarize([n],date(2026,11,4),status='ok',checked_at=CHECKED)
    assert r['current_amount'] is None
    assert r['current_deadline'] is None
    assert r['announcements'][0]['period_status']=='archive'
    assert r['account_balance'] is None


def test_current_month_and_next_month_are_separate_and_deadline_is_not_debt():
    november=notice(TEXT.replace('maj','XI').replace('.05.','.11.'))
    december=notice(TEXT.replace('maj','grudzień').replace('.05.','.12.'),source=payments.NEWS_URL+'#news-400')
    r=payments.summarize([november,december],date(2026,11,10),status='ok',checked_at=CHECKED)
    assert r['verified'] is True
    assert r['current_amount']=='260,00 zł'
    assert r['current_deadline']=='2026-11-08'
    assert {n['period_status'] for n in r['announcements']}=={'current','upcoming'}
    assert next(n for n in r['announcements'] if n['period_status']=='current')['deadline_passed'] is True
    assert 'paid' not in r and r['account_balance'] is None


def test_do_not_join_amounts_and_dates_from_neighbouring_news_articles():
    html=article('Obiady m-c listopad 2026. Kwota do zapłaty: 260,00 zł.','1')+article('Termin płatności: 08.11.2026.','2')
    notices=payments.parse_news(html,payments.NEWS_URL,CHECKED)
    assert len(notices)==1
    assert notices[0]['complete'] is False
    assert notices[0]['deadline'] is None
    assert notices[0]['source_url'].endswith('#news-1')


@pytest.mark.parametrize('text',[
    'Koszt obiadu w 2026 r. wynosi 13,00 zł.',
    'Obiady m-c IX 2026. O wysokości wpłaty poinformujemy 02.09.2026.',
    'Kwota do zapłaty: 260,00 zł. Termin płatności: 08.11.2026.',
    TEXT+' Obiady m-c listopad 2026 r.',
])
def test_do_not_infer_month_or_fee_from_general_information(text):
    assert notice(text) is None


@pytest.mark.parametrize('old,new',[
    ('260,00','999,00'),
    ('20 obiadów x 13,00 zł = 260,00','20 obiadów x 13,00 zł = 260,00. Kwota do zapłaty: 247,00 zł.'),
])
def test_inconsistent_amounts_are_not_marked_verified(old,new):
    n=notice(TEXT.replace(old,new))
    assert n['amount'] is None
    assert n['complete'] is False


def test_dates_require_payment_context_not_registration_deadline():
    n=notice('Karta zapisu do dnia 01.05.2026. '+TEXT)
    assert n['deadline']=='2026-05-08'
    bad=notice(TEXT.replace('08.05.2026','99.05.2026'))
    assert bad['deadline'] is None
    assert bad['complete'] is False


def test_polish_month_and_explicit_fee_without_multiplication():
    n=notice('Obiady w listopadzie 2026. Kwota za obiady: 247,00 zł. Termin płatności: do 8 listopada 2026.')
    assert n['period']=='2026-11'
    assert n['amount']=='247,00 zł'
    assert n['deadline']=='2026-11-08'
    assert n['complete'] is True


def test_bank_account_is_never_guessed_or_returned_with_bad_checksum():
    assert payments.bank_account('04102052260000650204160479')=='04 1020 5226 0000 6502 0416 0479'
    assert payments.bank_account('05 1020 5226 0000 6502 0416 0479') is None


def test_source_refresh_reads_current_and_previous_month_and_replaces_notice(monkeypatch):
    monkeypatch.setattr(payments,'saved_notices',lambda:[])
    calls=[]
    text=TEXT.replace('maj','listopad').replace('.05.','.11.')
    def fetch(url):
        calls.append(url)
        assert url.startswith(payments.NEWS_URL)
        return article(text).encode(),'text/html'
    r=payments.load_payments(fetch,date(2026,11,4))
    assert r['status']=='ok' and r['current_amount']=='260,00 zł'
    assert len(r['announcements'])==1
    months={base64.b64decode(parse_qs(urlsplit(u).query)['eqa'][0]).decode() for u in calls if '?' in u}
    assert months=={'newsarchivef=2026-11','newsarchivef=2026-10'}
    text=text.replace('20 obiadów x 13,00 zł = 260,00','19 obiadów x 13,00 zł = 247,00')
    updated=payments.load_payments(fetch,date(2026,11,4))
    assert updated['current_amount']=='247,00 zł'


def test_repeated_publication_is_deduplicated_but_conflicting_notices_are_visible():
    n=notice(TEXT.replace('maj','listopad').replace('.05.','.11.'))
    copy={**n,'id':'copy','source_url':canteen.SCHOOL_URL}
    r=payments.summarize([n,copy],date(2026,11,4),status='ok',checked_at=CHECKED)
    assert len(r['announcements'])==1
    assert r['verified'] is True
    r=payments.summarize([n,{**copy,'amount':'247,00 zł'}],date(2026,11,4),status='ok',checked_at=CHECKED)
    assert len(r['announcements'])==2 and r['conflict'] is True
    assert r['verified'] is False and r['current_amount'] is None


def test_source_outage_keeps_dated_archive_without_breaking_menu_or_other_modules(monkeypatch):
    def fetch(url,**kwargs):
        if url==canteen.SCHOOL_URL:return '<h1>Stołówka</h1>'.encode(),'text/html'
        raise ConnectionError('offline')
    monkeypatch.setattr(canteen,'_fetch',fetch)
    monkeypatch.setattr(canteen,'_cache',{})
    monkeypatch.setattr(canteen,'_cache_at',0)
    with TestClient(app) as client:
        assert client.get('/api/canteen/sp3').status_code==401
        client.post('/api/demo')
        d=client.get('/api/canteen/sp3').json()
        assert d['status']=='ok'
        assert d['payment']['status']=='unavailable'
        assert any(n['period']=='2026-05' for n in d['payment']['announcements'])
        assert d['payment']['current_amount'] is None
        assert d['calendar']['days']
        assert client.get('/api/section/messages').status_code==200


def test_fee_on_canteen_page_is_read_even_if_news_source_is_offline(monkeypatch):
    text=TEXT.replace('maj','listopad').replace('.05.','.11.')
    def fetch(url,**kwargs):
        if url==canteen.SCHOOL_URL:return ('<h1>Stołówka</h1><p>'+text+'</p>').encode(),'text/html'
        raise ConnectionError('offline')
    monkeypatch.setattr(canteen,'_fetch',fetch)
    monkeypatch.setattr(canteen,'_cache',{})
    monkeypatch.setattr(canteen,'_cache_at',0)
    with TestClient(app) as client:
        client.post('/api/demo')
        d=client.get('/api/canteen/sp3').json()
        n=next(n for n in d['payment']['announcements'] if n['period']=='2026-11')
        assert n['amount']=='260,00 zł' and n['deadline']=='2026-11-08'
