"""Plan lekcji: integracja pomarańczowych zakończonych zajęć, zastępstw i ocen."""
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
def test_timetable_integration():
    html=(ROOT/"static/index.html").read_text(encoding="utf-8")
    js=(ROOT/"static/app.js").read_text(encoding="utf-8")
    extras=(ROOT/"static/timetable-extras.js").read_text(encoding="utf-8")
    css=(ROOT/"static/timetable-extras.css").read_text(encoding="utf-8")
    app=(ROOT/"app.py").read_text(encoding="utf-8")
    assert "timetable-extras.js" in html and "timetable-extras.css" in html
    assert "Promise.all([section('timetable'" in js
    assert "SynergiaTimetable?.enhance" in js
    assert "Europe/Warsaw" in extras
    assert "Zakończona według planu" in extras
    assert "Oceny wpisane tego dnia" in extras
    assert "original_teacher" in app and "original_room" in app
    assert 'html[data-theme="dark"] .lesson-card.lesson-completed' in css
