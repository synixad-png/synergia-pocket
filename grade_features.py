"""Complete grade-view sources. Independent from other functional modules.

The SDK's grades() is only /Grades, not every grade shown by Synergia.
Keep IDs scoped to their source: identical numeric IDs in different grade
systems are distinct records. Never deduplicate by value/date/subject.
"""
import asyncio
from librus_synergia.exceptions import LibrusAuthError

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
            return await asyncio.wait_for(fetch(), 25), None
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
        if source == "partial" and count:
            warnings.append("Nowe oceny opisowe: odebrano pierwszą listę; kompletność tego źródła wymaga sprawdzenia.")
    if not successes:
        raise RuntimeError("Nie udało się odczytać żadnego źródła ocen.")
    entries = sorted(grouped.values(), key=lambda group: group["name"].casefold())
    for group in entries:
        group["grades"].sort(key=lambda row: row["date"] or "", reverse=True)
    return {"subjects": entries, "count": sum(len(group["grades"]) for group in entries),
            "sources": sources, "warnings": warnings}
