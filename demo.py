"""Fikcyjne dane wyłącznie do demonstracji interfejsu — nigdy dane uczniów."""
from __future__ import annotations

from datetime import date, timedelta

SUBJECTS = ["Matematyka", "Język polski", "Język angielski", "Biologia", "Historia", "Geografia", "Informatyka", "Wychowanie fizyczne"]
TIMES = [("08:00", "08:45"), ("08:55", "09:40"), ("09:50", "10:35"),
         ("10:50", "11:35"), ("11:45", "12:30"), ("12:45", "13:30")]


def demo_section(section: str, week: date | None = None, mailbox: str = "inbox") -> dict:
    today = date.today()
    start = (week or today) - timedelta(days=(week or today).weekday())
    d = lambda offset: (today + timedelta(days=offset)).isoformat()
    if section == "grades":
        rows = [
            ("Matematyka", [("5", -4, "Sprawdzian", 3), ("4+", -14, "Kartkówka", 2), ("5", -21, "Odpowiedź", 1)]),
            ("Język polski", [("5", -2, "Wypracowanie", 2), ("4", -17, "Aktywność", 1)]),
            ("Język angielski", [("6", -3, "Słówka", 2), ("5-", -11, "Test", 3)]),
            ("Biologia", [("4", -8, "Sprawdzian", 3), ("5", -16, "Projekt", 2)]),
            ("Historia", [("5", -6, "Odpowiedź", 2)]),
            ("Geografia", [("4+", -5, "Kartkówka", 2), ("5", -25, "Mapa", 1)]),
            ("Informatyka", [("6", -1, "Projekt", 3)]),
        ]
        return {"subjects": [{"name": name, "grades": [
            {"value": value, "date": d(offset), "category": kind, "weight": weight, "comments": [],
             "semester": 1, "kind": "cząstkowa"} for value, offset, kind, weight in values]}
            for name, values in rows], "count": sum(len(values) for _, values in rows)}
    if section == "timetable":
        days = []
        for i in range(5):
            day = start + timedelta(days=i)
            subjects = [SUBJECTS[(i * 3 + k) % len(SUBJECTS)] for k in range(6)]
            lessons = [{"num": k + 1, "start": TIMES[k][0], "end": TIMES[k][1], "subject": name,
                        "teacher": ["A. Wójcik", "M. Wiśniewska", "P. Kamiński"][k % 3],
                        "room": str(101 + ((i + k) % 12)), "canceled": False,
                        "substitution": False, "note": None, "original_subject": ""}
                       for k, name in enumerate(subjects)]
            if i == 2:
                lessons[4]["substitution"] = True
                lessons[4]["original_subject"] = lessons[4]["subject"]
                lessons[4]["teacher"] = "K. Zielińska"
            days.append({"date": day.isoformat(), "lessons": lessons})
        return {"week_start": start.isoformat(), "days": days}
    if section == "attendance":
        labels = [("Obecność", "present"), ("Obecność", "present"), ("Obecność", "present"),
                  ("Spóźnienie", "late"), ("Nieobecność usprawiedliwiona", "excused"),
                  ("Obecność", "present"), ("Nieobecność", "absent")]
        records = [{"date": d(-(i // 6)), "lesson": 1 + i % 6,
                    "label": labels[i % len(labels)][0], "status": labels[i % len(labels)][1]} for i in range(42)]
        stats = {"present": 0, "absent": 0, "excused": 0, "late": 0, "total": len(records)}
        for item in records:
            stats[item["status"]] += 1
        return {"stats": stats, "records": records}
    if section == "agenda":
        items = [
            {"id": 1, "date": d(1), "title": "Kartkówka", "description": "Dział: ułamki dziesiętne", "subject": "Matematyka"},
            {"id": 2, "date": d(3), "title": "Sprawdzian", "description": "Wydarzenia XIX wieku", "subject": "Historia"},
            {"id": 3, "date": d(4), "title": "Wydarzenie", "description": "Wyjście do muzeum", "subject": ""},
            {"id": 4, "date": d(8), "title": "Sprawdzian", "description": "Rozdział 4 — słownictwo i gramatyka", "subject": "Język angielski"},
            {"id": 5, "date": d(-2), "title": "Kartkówka", "description": "Budowa komórki", "subject": "Biologia"},
        ]
        return {"items": sorted(items, key=lambda x: x["date"])}
    if section == "homework":
        return {"items": [
            {"id": 1, "topic": "Zadania 3–7 ze strony 62", "text": "Rozwiąż zadania w zeszycie.",
             "date": d(-1), "due_date": d(1), "teacher": "A. Wójcik", "attachments": 0},
            {"id": 2, "topic": "Przeczytaj rozdział 5", "text": "Przygotuj trzy pytania do tekstu.",
             "date": d(-1), "due_date": d(2), "teacher": "M. Wiśniewska", "attachments": 0},
            {"id": 3, "topic": "Prezentacja o planetach", "text": "Przygotuj krótką prezentację z ilustracjami.",
             "date": d(-3), "due_date": d(7), "teacher": "P. Kamiński", "attachments": 1},
        ]}
    if section == "messages":
        if mailbox == "outbox":
            return {"unread": 2, "mailbox": "outbox", "items": [{"id": "demo-out-1", "sender": "",
                "recipient": "Wychowawca", "subject": "Potwierdzenie udziału", "preview": "Dziękuję za informację.",
                "date": d(-5), "unread": False, "mailbox": "outbox"}]}
        return {"unread": 2, "mailbox": "inbox", "items": [
            {"id": "demo-1", "sender": "Anna Wójcik", "subject": "Informacja o wycieczce",
             "preview": "Szanowni Państwo, przekazuję szczegóły piątkowej wycieczki...",
             "date": d(-1), "unread": True, "mailbox": "inbox"},
            {"id": "demo-2", "sender": "Sekretariat", "subject": "Zebranie z rodzicami",
             "preview": "Przypominamy o zebraniu w przyszłym tygodniu...", "date": d(-2),
             "unread": True, "mailbox": "inbox"},
            {"id": "demo-3", "sender": "Marek Kamiński", "subject": "Projekt z informatyki",
             "preview": "Prace projektowe można przesyłać do końca tygodnia.", "date": d(-4),
             "unread": False, "mailbox": "inbox"},
        ]}
    if section == "notes":
        return {"items": [
            {"date": d(-6), "text": "Aktywny udział w projekcie zespołowym.",
             "teacher": "M. Wiśniewska", "category": "Pochwała", "sentiment": "positive"},
            {"date": d(-16), "text": "Brak wymaganych materiałów na zajęciach.",
             "teacher": "A. Wójcik", "category": "Uwaga", "sentiment": "negative"},
        ]}
    if section == "announcements":
        return {"items": [
            {"date": d(-1), "title": "Zmiana organizacji zajęć", "text": "W piątek część zajęć odbędzie się według zmienionego planu.", "unread": True},
            {"date": d(-4), "title": "Konsultacje dla rodziców", "text": "Najbliższe konsultacje odbędą się w przyszłym tygodniu.", "unread": False},
        ]}
    raise ValueError(section)


def demo_message(message_id: str, mailbox: str) -> dict | None:
    item = next((x for x in demo_section("messages", mailbox=mailbox)["items"] if x["id"] == message_id), None)
    if not item:
        return None
    return {"id": item["id"], "subject": item["subject"], "sender": item["sender"] or item["recipient"],
            "date": item["date"], "body": item["preview"] + "\n\nTo jest fikcyjna wiadomość demonstracyjna.",
            "attachments": []}
