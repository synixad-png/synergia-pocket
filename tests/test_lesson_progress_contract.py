"""Kontrola integracji paska bieżącego czasu w planie lekcji."""
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]

def test_clock_integration():
    js=(ROOT/"static/app.js").read_text(encoding="utf-8")
    script=(ROOT/"static/lesson-progress.js").read_text(encoding="utf-8")
    index=(ROOT/"static/index.html").read_text(encoding="utf-8")
    style=(ROOT/"static/style.css").read_text(encoding="utf-8")
    assert '<script src="/static/lesson-progress.js" defer></script>' in index
    assert index.index("/static/lesson-progress.js") < index.index("/static/app.js")
    for marker in ('data-lesson-date','data-lesson-start','data-lesson-end','data-lesson-canceled','lesson-live','window.SynergiaLessonClock?.update()'):
        assert marker in js
    assert "Europe/Warsaw" in script
    assert "setInterval(update, 15000)" in script
    assert "lesson-ongoing" in style
    assert "lesson-live-marker" in style
