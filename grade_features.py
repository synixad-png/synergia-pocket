"""Complete grade-view sources. Independent from other functional modules.

The SDK's grades() is only /Grades, not every grade shown by Synergia.
Keep IDs scoped to their source: identical numeric IDs in different grade
systems are distinct records. Never deduplicate by value/date/subject.
"""
import asyncio
import time
from urllib.parse import urlencode
from librus_synergia.exceptions import LibrusAuthError


class GradePage(list):
    warning = ""


async def collect_partial_pages(fetch):
    """Only accept a continuation when the server echoes the requested page.

    Unknown/ignored pagination never masquerades as a complete grade list.
    fetch(None, None) performs the known first read with its original body {}.
    """
    result, seen, expected, limit, total = GradePage(), set(), None, None, None
    started = time.monotonic()
    for _ in range(100):
        if expected is not None and time.monotonic() - started > 18:
            result.warning = f"Nowe opisowe: odczytano {len(result)} z {total} wpisów; przekroczono czas dalszego odczytu."
            return result
        try:
            payload = await fetch(expected, limit)
        except LibrusAuthError:
            raise
        except Exception:
            if total is None:
                raise
            result.warning = f"Nowe opisowe: odczytano {len(result)} z {total} wpisów; nie udało się pobrać dalszej strony."
            return result
        if not isinstance(payload, dict) or not isinstance(payload.get("data"), list):
            if total is not None:
                result.warning = f"Nowe opisowe: odczytano {len(result)} z {total} wpisów; dalsza strona ma nieznany format."
                return result
            raise ValueError("Nieznany format nowych ocen opisowych")
        meta = payload.get("pagination")
        if not isinstance(meta, dict):
            meta = {}
        number, size, count = meta.get("page"), meta.get("limit"), meta.get("total")
        valid = all(type(v) is int for v in (number, size, count)) and number >= 0 and 0 < size <= 5000 and len(payload["data"]) <= count <= 20000
        if expected is not None and (not valid or number != expected or count != total):
            result.warning = f"Nowe opisowe: odczytano {len(result)} z {total} wpisów; serwer nie potwierdził dalszej strony."
            return result
        before = len(result)
        for row in payload["data"]:
            if not isinstance(row, dict) or row.get("gradeId") is None:
                raise ValueError("Nieznany wpis nowych ocen opisowych")
            identity = str(row["gradeId"])
            if identity not in seen:
                seen.add(identity)
                result.append(row)
        if not valid:
            if result:
                result.warning = "Nowe opisowe: serwer nie udostępnił informacji o kompletności listy."
            return result
        total, limit = count, size
        if len(result) >= total:
            return result
        if len(result) == before:
            result.warning = f"Nowe opisowe: odczytano {len(result)} z {total} wpisów; dalsza strona nie zawiera nowych danych."
            return result
        expected = number + 1
    result.warning = f"Nowe opisowe: odczytano {len(result)} z {total} wpisów; przerwano nadmiernie długie stronicowanie."
    return result


async def partial_grades(client):
    # Use the SDK's authenticated request/refresh limiter, but retain pagination
    # metadata which its convenience partial_grades() method discards.
    low = getattr(client, "client", None)
    if not (callable(getattr(client, "student_identifier", None)) and
            callable(getattr(low, "_async_request_url", None)) and callable(getattr(client, "_call", None))):
        return await client.partial_grades()
    from librus_synergia.const import DATA_BASE_URL, ENDPOINT_PARTIAL_GRADES
    from librus_synergia.parsers import parse_partial_grades
    student = await client.student_identifier()
    if not isinstance(student, str) or not 1 <= len(student) <= 160 or not all(c.isascii() and (c.isalnum() or c in "-_") for c in student):
        raise ValueError("Nie udało się ustalić ucznia dla nowych ocen opisowych")
    target = f"{DATA_BASE_URL}/{ENDPOINT_PARTIAL_GRADES.format(student=student)}"
    async def fetch(page, limit):
        query = "" if page is None else "?" + urlencode({"page": page, "limit": limit})
        return await asyncio.wait_for(client._call(lambda: low._async_request_url(target + query, method="POST", json_body={})), 6)
    raw = await collect_partial_pages(fetch)
    subjects = {}
    if raw:
        try:
            subjects = await asyncio.wait_for(client._subjects_by_lid(), 3)
        except LibrusAuthError:
            raise
        except Exception:
            pass
    result = GradePage(parse_partial_grades({"data": raw}, subjects))
    result.warning = raw.warning
    return result

SOURCES = (
    ("classic", "Klasyczne", "grades"),
    ("descriptive", "Opisowe", "descriptive_grades"),
    ("text", "Tekstowe", "text_grades"),
    ("point", "Punktowe", "point_grades"),
    ("partial", "Nowe opisowe", "partial_grades"),
)


async def collect_grades(client):
    async def read(method):
        fetch = getattr(client, method, None)
        if not callable(fetch):
            return None, "unsupported"
        try:
            call = partial_grades(client) if method == "partial_grades" else fetch()
            return await asyncio.wait_for(call, 25), None
        except LibrusAuthError:
            raise
        except Exception as exc:
            status = getattr(exc, "status_code", None)
            return None, "unavailable" if status in (403, 404, 405) else "failed"

    results = await asyncio.gather(*(read(method) for _, _, method in SOURCES),
                                   read("subjects"), read("grade_categories"))
    subjects = results[-2][0] if isinstance(results[-2][0], dict) else {}
    categories = results[-1][0] if isinstance(results[-1][0], dict) else {}
    warnings, sources, grouped, seen = [], [], {}, set()
    if results[-2][1]:
        warnings.append("Nie udało się odczytać nazw przedmiotów. Oceny zachowano pod identyfikatorami.")
    successes = 0
    for (source, label, _), (grades, failure) in zip(SOURCES, results):
        if failure:
            sources.append({"source": source, "label": label, "state": failure, "count": 0})
            if failure != "unavailable":
                warnings.append(f"{label}: nie udało się pobrać tej części ocen.")
            continue
        if not isinstance(grades, list):
            warnings.append(f"{label}: nieznany format listy ocen.")
            sources.append({"source": source, "label": label, "state": "failed", "count": 0})
            continue
        successes += 1
        count = 0
        for index, grade in enumerate(grades):
            get = lambda field, default=None: getattr(grade, field, default)
            raw_id = get("id")
            identity = (source, str(raw_id)) if raw_id is not None else (source, "row", index)
            if identity in seen:
                continue
            seen.add(identity)
            subject_id = get("subject_id")
            subject = subjects.get(subject_id, subjects.get(str(subject_id)))
            name = str(getattr(subject, "name", subject) or
                       (f"Przedmiot #{subject_id}" if subject_id is not None else "Przedmiot nieokreślony"))
            category_id = get("category_id")
            category = categories.get(category_id, categories.get(str(category_id)))
            kind = "semestralna" if get("is_semester", False) else "roczna" if get("is_final", False) else \
                "proponowana" if get("is_semester_proposition", False) or get("is_final_proposition", False) else "cząstkowa"
            row = {
                "id": str(raw_id) if raw_id is not None else None,
                "source": source, "source_label": label,
                "value": str(get("value", "") if get("value", "") is not None else ""),
                "date": get("add_date") or get("date"), "semester": get("semester"),
                "category": (get("skill") or "Ocena opisowa") if source in ("descriptive", "partial") else
                            get("category") or (getattr(category, "name", "Ocena") if source == "classic" else label),
                "weight": getattr(category, "weight", None) if source == "classic" else get("weight"),
                "comments": get("comments", []), "kind": kind,
                "points": get("points"), "max_points": get("max_points"),
                "requirements": get("requirements", []),
            }
            key = str(subject_id) if subject_id is not None else "unknown:" + source
            grouped.setdefault(key, {"name": name, "grades": []})["grades"].append(row)
            count += 1
        sources.append({"source": source, "label": label, "state": "ok" if count else "empty", "count": count})
        # The installed SDK exposes only the first page of this newer system.
        # Do not falsely claim its completeness when the account actually uses it.
        if source == "partial":
            if isinstance(grades, GradePage):
                if grades.warning:
                    warnings.append(grades.warning)
                    sources[-1]["state"] = "partial"
            elif count:
                warnings.append("Nowe oceny opisowe: odebrano pierwszą listę; kompletność tego źródła wymaga sprawdzenia.")
    if not successes:
        raise RuntimeError("Nie udało się odczytać żadnego źródła ocen.")
    entries = sorted(grouped.values(), key=lambda group: group["name"].casefold())
    for group in entries:
        group["grades"].sort(key=lambda row: row["date"] or "", reverse=True)
    return {"subjects": entries, "count": sum(len(group["grades"]) for group in entries),
            "sources": sources, "warnings": warnings}
