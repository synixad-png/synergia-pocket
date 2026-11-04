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


def test_verified_partial_pagination_collects_later_grades():
    from grade_features import collect_partial_pages
    calls = []
    async def fetch(page, limit):
        calls.append((page, limit))
        n = page or 1
        return {"data": [{"gradeId": n, "scaleValue": {"value": "5"}}],
                "pagination": {"page": n, "limit": 1, "total": 3}}
    result = asyncio.run(collect_partial_pages(fetch))
    assert [row["gradeId"] for row in result] == [1, 2, 3]
    assert calls == [(None, None), (2, 1), (3, 1)]
    assert result.warning == ""


def test_ignored_pagination_is_not_counted_twice_or_claimed_complete():
    from grade_features import collect_partial_pages
    async def fetch(page, limit):
        return {"data": [{"gradeId": 1}], "pagination": {"page": 1, "limit": 1, "total": 3}}
    result = asyncio.run(collect_partial_pages(fetch))
    assert len(result) == 1 and "nie potwierdził" in result.warning


def test_later_page_failure_keeps_already_collected_grades():
    from grade_features import collect_partial_pages
    async def fetch(page, limit):
        if page: raise RuntimeError("PRIVATE")
        return {"data": [{"gradeId": 1}], "pagination": {"page": 1, "limit": 1, "total": 3}}
    result = asyncio.run(collect_partial_pages(fetch))
    assert len(result) == 1 and "1 z 3" in result.warning and "PRIVATE" not in result.warning


def test_sdk_adapter_keeps_pagination_metadata_and_current_child():
    from urllib.parse import urlsplit, parse_qs
    calls = []
    class Low:
        async def _async_request_url(self, url, method, json_body):
            calls.append((url, method, json_body))
            n = int(parse_qs(urlsplit(url).query).get("page", [1])[0])
            return {"data": [{"gradeId": n, "subjectId": 1, "scaleValue": {"value": 0}, "area": {"name": "Czytanie"}}],
                    "pagination": {"page": n, "limit": 1, "total": 2}}
    class Client(AllGrades):
        client = Low()
        async def student_identifier(self): return "LID-AUTH-USER-child"
        async def _call(self, fetch): return await fetch()
        async def _subjects_by_lid(self): return {}
    result = asyncio.run(collect_grades(Client()))
    assert result["count"] == 125 and not result["warnings"]
    assert len(calls) == 2
    assert all(urlsplit(url).path.endswith("/Student/LID-AUTH-USER-child") and method == "POST" and body == {}
               for url, method, body in calls)
