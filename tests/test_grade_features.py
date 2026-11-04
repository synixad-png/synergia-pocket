import asyncio
from types import SimpleNamespace as N
import pytest
from grade_features import collect_grades


class AllGrades:
    async def subjects(self): return {1: "Matematyka", 2: "Muzyka"}
    async def grade_categories(self): return {3: N(name="Sprawdzian", weight=3)}
    async def grades(self):
        return [N(id=i, value="5", subject_id=1, category_id=3, add_date="2026-11-04") for i in range(1, 121)]
    async def descriptive_grades(self):
        return [N(id=1, value="6", subject_id=2, skill="Śpiew", comments=["Dobra praca"], add_date="2026-11-04")]
    async def text_grades(self):
        return [N(id=1, value="Bardzo dobrze opanowane umiejętności " * 8, subject_id=2, category="Umiejętności", date="2026-11-03")]
    async def point_grades(self):
        return [N(id=1, value="17", points=17, max_points=20, subject_id=1, category="Test", add_date="2026-11-04")]
    async def partial_grades(self): return []


def test_collects_all_sources_without_limit_or_value_deduplication():
    result = asyncio.run(collect_grades(AllGrades()))
    assert result["count"] == 123
    assert sum(len(group["grades"]) for group in result["subjects"]) == 123
    assert {s["source"]: s["count"] for s in result["sources"]} == {
        "classic": 120, "descriptive": 1, "text": 1, "point": 1, "partial": 0}
    assert len(next(g for g in result["subjects"] if g["name"] == "Matematyka")["grades"]) == 121


def test_auxiliary_source_failure_preserves_classic_and_descriptive():
    class Client(AllGrades):
        async def point_grades(self): raise RuntimeError("PRIVATE CONTENT")
    result = asyncio.run(collect_grades(Client()))
    assert result["count"] == 122
    assert any("Punktowe" in warning for warning in result["warnings"])
    assert "PRIVATE CONTENT" not in str(result)


def test_missing_subject_lookup_preserves_every_record():
    class Client(AllGrades):
        async def subjects(self): raise RuntimeError()
    result = asyncio.run(collect_grades(Client()))
    assert result["count"] == 123
    assert {group["name"] for group in result["subjects"]} == {"Przedmiot #1", "Przedmiot #2"}


def test_same_id_deduplicates_only_within_one_source():
    class Client(AllGrades):
        async def grades(self): return [N(id=1, value="5", subject_id=1)] * 2
    result = asyncio.run(collect_grades(Client()))
    assert result["count"] == 4


def test_auth_failure_is_not_reported_as_no_grades():
    from librus_synergia.exceptions import LibrusAuthError
    class Client(AllGrades):
        async def grades(self): raise LibrusAuthError("Expired")
    with pytest.raises(LibrusAuthError): asyncio.run(collect_grades(Client()))


def test_partial_grades_are_included_with_explicit_completeness_warning():
    class Client(AllGrades):
        async def partial_grades(self): return [N(id="p1", value="0", subject_id=2, skill="Czytanie")]
    result = asyncio.run(collect_grades(Client()))
    assert result["count"] == 124
    row = next(r for g in result["subjects"] for r in g["grades"] if r["source"] == "partial")
    assert row["value"] == "0"
    assert any("pierwszą listę" in warning for warning in result["warnings"])
