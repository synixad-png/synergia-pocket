"""Testy kontraktu mobilnego: właściwy viewport i brak blokady powiększania strony."""
from pathlib import Path
from html.parser import HTMLParser

ROOT = Path(__file__).resolve().parents[1]
HTML = (ROOT / "static" / "index.html").read_text(encoding="utf-8")
CSS = (ROOT / "static" / "style.css").read_text(encoding="utf-8")

class ViewportParser(HTMLParser):
    def __init__(self):
        super().__init__()
        self.viewport = ""
    def handle_starttag(self, tag, attrs):
        if tag == "meta" and dict(attrs).get("name") == "viewport":
            self.viewport = dict(attrs).get("content", "")

def test_mobile_viewport_meta_keeps_zoom_accessible():
    parser = ViewportParser()
    parser.feed(HTML)
    assert "width=device-width" in parser.viewport
    assert "initial-scale=1" in parser.viewport
    assert "user-scalable=no" not in parser.viewport
    assert "maximum-scale=1" not in parser.viewport

def test_mobile_layout_and_ios_form_zoom_protection():
    assert "Synergia Pocket v4.3" in CSS
    assert "overflow-x:clip" in CSS
    assert ".mobile-wordmark { flex:1 1 auto; min-width:0" in CSS
    assert ".bottom-link { flex:1 1 0; min-width:0" in CSS
    assert "font-size:16px" in CSS
    assert "overflow-wrap:anywhere" in CSS
