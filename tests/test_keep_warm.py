"""Kontrakt bezpiecznego podtrzymania serwera: tylko health, bez kont dzieci."""
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
WORKFLOW=ROOT/".github"/"workflows"/"keep-warm.yml"

def test_keep_warm_is_school_hours_only_and_private_data_free():
    text=WORKFLOW.read_text(encoding="utf-8")
    assert "timezone: Europe/Warsaw" in text
    assert "3,13,23,33,43,53 7-17 * * 1-5" in text
    assert "workflow_dispatch:" in text
    assert "/api/health" in text
    assert "/api/session" not in text
    assert "/api/section/" not in text
    assert "SYNERGIA_REDIS_URL" not in text
    assert "password" not in text.lower()
    assert "secrets." not in text
    assert "contents: read" in text
