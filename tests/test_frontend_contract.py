import re
from html.parser import HTMLParser
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]

class IDs(HTMLParser):
    def __init__(self):
        super().__init__(); self.ids=set();self.page_names=set()
    def handle_starttag(self,tag,attrs):
        d=dict(attrs)
        if d.get('id'):self.ids.add(d['id'])
        if d.get('data-page'):self.page_names.add(d['data-page'])

def test_js_dom_references_exist():
    js=(ROOT/'static/app.js').read_text()
    html=(ROOT/'static/index.html').read_text()
    parsed=IDs();parsed.feed(html)
    ids=set(re.findall(r"\$\('([a-zA-Z0-9_-]+)'\)",js))
    assert ids.issubset(parsed.ids), sorted(ids-parsed.ids)
    expected={'dashboard','timetable','grades','attendance','agenda','homework','messages','notes','announcements'}
    assert parsed.page_names==expected


def test_offline_preview_contains_embedded_styles_and_script():
    preview=ROOT/'SYNERGIA_POCKET_WEB_v3_DEMO.html'
    assert preview.exists()
    text=preview.read_text()
    assert 'OFFLINE_DEMO' in text
    assert '<style>' in text
    assert '<script>' in text
    assert 'http://preview.local' in text
