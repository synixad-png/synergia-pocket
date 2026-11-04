"""Independent HTTP protocol tests. No upstream connections or real accounts."""
from __future__ import annotations

import base64
import json
from collections import defaultdict
from datetime import date

import pytest

from synergia_independent import (
    SynergiaClient, AuthenticationRequired, CaptchaRequired,
    MessagesUnavailable, HTTPError, ProtocolError,
)
from synergia_independent.client import PORTAL, OAUTH, REFRESH, DATA, MESSAGE_API, MESSAGE_BOOTSTRAP
from synergia_independent.models import decode_preview, reference_names


class FakeJar:
    def __init__(self):
        self.has_token = False

    def filter_cookies(self, url):
        return {"oauth_token": "fake"} if self.has_token else {}


class FakeResponse:
    def __init__(self, method, url, status, body, *, location=None, cookie=False):
        self.status = status
        self.url = url
        self.headers = {"Location": location} if location else {}
        self.cookies = {"oauth_token": "new"} if cookie else {}
        self.body = body
        self._cookie = cookie

    async def read(self):
        return self.body if isinstance(self.body, bytes) else str(self.body).encode("utf-8")


class FakeRequestContext:
    def __init__(self, session, response):
        self.session = session
        self.response = response

    async def __aenter__(self):
        if self.response._cookie:
            self.session.cookie_jar.has_token = True
        return self.response

    async def __aexit__(self, *args):
        return False


class FakeSession:
    """Only responses explicitly registered here are possible; no network."""
    def __init__(self):
        self.cookie_jar = FakeJar()
        self.replies = defaultdict(list)
        self.calls = []
        self.closed = False

    def add(self, method, url, status, body="", **kwargs):
        self.replies[(method, url)].append(
            FakeResponse(method, url, status, body, **kwargs))

    def request(self, method, url, **kwargs):
        self.calls.append((method, url, kwargs))
        responses = self.replies[(method, url)]
        assert responses, f"Unplanned request: {method} {url}"
        return FakeRequestContext(self, responses.pop(0))


def login_script(session):
    authorization = "https://api.librus.pl/OAuth/Authorization?client_id=46&response_type=code"
    session.add("GET", PORTAL, 302, "", location=authorization)
    session.add("GET", authorization, 200, "")
    session.add("POST", OAUTH, 200, '{"status":"ok","goTo":"/OAuth/Authorization/2FA?client_id=46"}')
    session.add("GET", "https://api.librus.pl/OAuth/Authorization/2FA?client_id=46",
                302, "", location="https://synergia.librus.pl/sso/landing")
    session.add("GET", "https://synergia.librus.pl/sso/landing", 200, "", cookie=True)


@pytest.mark.asyncio
async def test_login_follows_confirmed_flow_without_retaining_password():
    session = FakeSession()
    login_script(session)
    client = SynergiaClient("child_user", session=session)
    await client.login("synthetic-secret-42")
    assert client._has_cookie()
    assert "synthetic-secret-42" not in repr(client.__dict__)
    assert len(session.calls) == 5
    request = session.calls[2]
    assert request[0] == "POST"
    assert request[2]["data"]["login"] == "child_user"
    assert request[2]["data"]["pass"] == "synthetic-secret-42"
    assert request[2]["allow_redirects"] is False


@pytest.mark.asyncio
async def test_foreign_oauth_redirect_is_never_requested():
    session = FakeSession()
    session.add("GET", PORTAL, 302, "", location="https://example.com/login")
    client = SynergiaClient("abc", session=session)
    with pytest.raises(ProtocolError):
        await client.login("only-a-fake-password")
    assert len(session.calls) == 1


@pytest.mark.asyncio
async def test_authentication_fails_without_cookie_and_without_password_storage():
    session = FakeSession()
    authorization = "https://api.librus.pl/OAuth/Authorization?client_id=46"
    session.add("GET", PORTAL, 302, "", location=authorization)
    session.add("GET", authorization, 200)
    session.add("POST", OAUTH, 200, '{"goTo":"/OAuth/Authorization/2FA?client_id=46"}')
    session.add("GET", "https://api.librus.pl/OAuth/Authorization/2FA?client_id=46", 200)
    client = SynergiaClient("abc", session=session)
    with pytest.raises(AuthenticationRequired):
        await client.login("fake-pass")
    assert client._authenticated_at == 0.0


@pytest.mark.asyncio
async def test_captcha_marker_is_not_silently_bypassed():
    session = FakeSession()
    auth = "https://api.librus.pl/OAuth/Authorization?client_id=46"
    session.add("GET", PORTAL, 302, location=auth, body="")
    session.add("GET", auth, 200)
    session.add("POST", OAUTH, 200, '{"goTo":"/2FA","captcha":true}')
    client = SynergiaClient("abc", session=session)
    with pytest.raises(CaptchaRequired):
        await client.login("fake-pass")


@pytest.mark.asyncio
async def test_refresh_requires_new_cookie_from_response():
    session = FakeSession()
    client = SynergiaClient("abc", session=session)
    client._authenticated_at = 1.0
    session.cookie_jar.has_token = True
    session.add("GET", REFRESH, 200, "", cookie=False)
    assert await client.refresh() is False
    session.add("GET", REFRESH, 200, "", cookie=True)
    assert await client.refresh() is True


@pytest.mark.asyncio
async def test_read_grades_with_subject_names_categories():
    session = FakeSession()
    client = SynergiaClient("abc", session=session)
    client._authenticated_at = __import__("time").monotonic()
    session.cookie_jar.has_token = True
    session.add("GET", DATA+"/Grades", 200, json.dumps({
        "Grades": [{"Id":101,"Grade":"5+","AddDate":"2026-11-04",
                    "Subject":{"Id":51},"Category":{"Id":9},"Semester":1}]
    }))
    session.add("GET", DATA+"/Subjects", 200,
                json.dumps({"Subjects":[{"Id":51,"Name":"Matematyka"}]}))
    session.add("GET", DATA+"/Grades/Categories", 200,
                json.dumps({"Categories":[{"Id":9,"Name":"Sprawdzian"}]}))
    items = await client.grades()
    assert len(items)==1
    assert items[0].value=="5+" and items[0].subject=="Matematyka"
    assert items[0].category=="Sprawdzian"
    assert items[0].date=="2026-11-04"


@pytest.mark.asyncio
async def test_timetable_nested_periods_parallel_classes_and_substitutions():
    session=FakeSession()
    client=SynergiaClient("abc",session=session)
    client._authenticated_at=__import__("time").monotonic()
    session.cookie_jar.has_token=True
    body={"Timetable":{"2026-11-04":[
        [
            {"LessonNo":1,"HourFrom":"08:00","HourTo":"08:45",
             "Subject":{"Id":1},"Teacher":{"Id":22},"Classroom":{"Id":8},
             "IsCanceled":False,"IsSubstitutionClass":True,
             "OrgSubject":{"Id":2},"SubstitutionNote":"Inny nauczyciel"},
            {"LessonNo":1,"HourFrom":"08:00","HourTo":"08:45",
             "Subject":{"Id":2},"IsCanceled":False}
        ], [], [
            {"LessonNo":3,"HourFrom":"09:50","HourTo":"10:35",
             "Subject":{"Id":1},"IsCanceled":True}
        ]]}}
    session.add("GET",DATA+"/Timetables",200,json.dumps(body))
    session.add("GET",DATA+"/Subjects",200,json.dumps({"Subjects":[
        {"Id":1,"Name":"Matematyka"},{"Id":2,"Name":"Angielski"}]}))
    session.add("GET",DATA+"/Users",200,json.dumps({"Users":[
        {"Id":22,"FirstName":"Anna","LastName":"Nauczyciel"}]}))
    session.add("GET",DATA+"/Classrooms",200,
                json.dumps({"Classrooms":[{"Id":8,"Name":"204"}]}))
    days=await client.timetable(date(2026,11,4))
    assert len(days["2026-11-04"])==3
    first=days["2026-11-04"][0]
    assert first.substitution and first.original_subject=="Angielski"
    assert first.teacher=="Anna Nauczyciel" and first.room=="204"
    assert days["2026-11-04"][-1].canceled
    assert next(c for c in session.calls if c[1].endswith("/Timetables"))[2]["params"]=={"weekStart":"2026-11-02"}


@pytest.mark.asyncio
async def test_messages_read_only_and_independent_bootstrap_recovery():
    session=FakeSession()
    session.cookie_jar.has_token=True
    client=SynergiaClient("abc",session=session)
    client._authenticated_at=__import__("time").monotonic()
    session.add("GET",MESSAGE_BOOTSTRAP,200,"Welcome")
    url=MESSAGE_API+"/inbox/messages"
    session.add("GET",url,401,"Not authorized")
    session.add("GET",MESSAGE_BOOTSTRAP,200,"Welcome again")
    raw="<Message><Content><![CDATA[Witaj!]]></Content></Message>"
    encoded=base64.b64encode(raw.encode()).decode()
    session.add("GET",url,200,json.dumps({
        "data":[{"messageId":52,"senderFirstName":"Jan","senderLastName":"Kowalski",
                 "topic":"Spotkanie","content":encoded,"sendDate":"2026-11-04","readDate":None}]
    }))
    rows=await client.messages(limit=15)
    assert len(rows)==1 and rows[0].unread
    assert rows[0].preview=="Witaj!"
    assert rows[0].sender=="Jan Kowalski"
    assert [x[1] for x in session.calls].count(MESSAGE_BOOTSTRAP)==2
    assert all("/messages/52" not in c[1] for c in session.calls)


@pytest.mark.asyncio
async def test_access_denied_messages_not_treated_as_empty_inbox():
    session=FakeSession()
    session.cookie_jar.has_token=True
    client=SynergiaClient("abc",session=session)
    client._authenticated_at=__import__("time").monotonic()
    session.add("GET",MESSAGE_BOOTSTRAP,200,"Brak dostępu do modułu")
    with pytest.raises(MessagesUnavailable):
        await client.messages()
    assert len(session.calls)==1


@pytest.mark.asyncio
async def test_no_relogin_for_timtable_forbidden():
    session=FakeSession()
    session.cookie_jar.has_token=True
    client=SynergiaClient("abc",session=session)
    client._authenticated_at=__import__("time").monotonic()
    session.add("GET",DATA+"/Timetables",403,"No plan")
    session.add("GET",DATA+"/Subjects",200,'{"Subjects":[]}')
    session.add("GET",DATA+"/Users",200,'{"Users":[]}')
    session.add("GET",DATA+"/Classrooms",200,'{"Classrooms":[]}')
    with pytest.raises(HTTPError) as err:
        await client.timetable(date(2026,11,4))
    assert err.value.status == 403
    assert len([x for x in session.calls if x[1]==PORTAL])==0


@pytest.mark.asyncio
async def test_one_account_one_session_identity():
    session_a,session_b=FakeSession(),FakeSession()
    a,b=SynergiaClient("A",session=session_a),SynergiaClient("B",session=session_b)
    session_a.cookie_jar.has_token=True
    assert a._has_cookie() and not b._has_cookie()
    assert a._session is not b._session
    assert a.username=="A" and b.username=="B"


def test_decode_truncated_and_reference_names():
    raw="Zażółć"
    broken=base64.b64encode(raw.encode()[:-1]).decode()
    assert decode_preview(broken).startswith("Zażół")
    assert reference_names({"Users":[{"Id":"23","FirstName":"Ola","LastName":"N."}]},
                           "Users",people=True)["23"]=="Ola N."
