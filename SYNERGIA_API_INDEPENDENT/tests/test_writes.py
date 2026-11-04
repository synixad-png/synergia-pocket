"""Independent outbound-message safety tests — mock network only."""
import base64
import json
import time
import pytest

from synergia_independent import SynergiaClient, MessageWriteUncertain
from synergia_independent.client import MESSAGE_API
from test_protocol import FakeSession


def service():
    session=FakeSession()
    session.cookie_jar.has_token=True
    client=SynergiaClient("fake-child",session=session)
    client._authenticated_at=time.monotonic()
    client._messages_ready=True
    return client,session


def contacts_responses(session):
    session.add("GET", MESSAGE_API + "/receivers/types", 200,
                json.dumps({"data":{"list":[{"id":"teachers","name":"Nauczyciele"}]}}))
    session.add("GET", MESSAGE_API + "/receivers/student-subjects", 200,
                json.dumps([{"teacherIdentifier":"1999","subject":"Język polski"}]))
    session.add("GET", MESSAGE_API + "/receivers/groups/school-employees", 200,
                json.dumps({"receivers":[{"accountId":"1999","label":"Anna Nauczyciel",
                           "availabilityStatus":{"isAvailable":True}}]}))


@pytest.mark.asyncio
async def test_contacts_group_subjects_and_availability():
    client,session=service()
    contacts_responses(session)
    contacts=await client.contacts()
    assert len(contacts)==1
    assert contacts[0].account_id=="1999"
    assert contacts[0].name=="Anna Nauczyciel"
    assert contacts[0].subjects==("Język polski",)
    assert contacts[0].available is True
    assert session.calls[-1][2]["params"]=={"receiverType":"teachers"}


@pytest.mark.asyncio
async def test_explicit_confirmation_required_before_any_network():
    client,session=service()
    with pytest.raises(ValueError):
        await client.send_message(["1999"],"Witaj","Treść")
    with pytest.raises(ValueError):
        await client.reply_to_message("100","Treść")
    with pytest.raises(ValueError):
        await client.full_message("100")
    assert not session.calls


@pytest.mark.asyncio
async def test_compose_exactly_one_post_plain_text_escaped_and_confirmed():
    client,session=service()
    contacts_responses(session)
    session.add("GET", MESSAGE_API+"/me",200,'{"data":{}}')
    session.add("POST",MESSAGE_API+"/messages",201,'{"data":{"messageId":876,"status":"sent"}}')
    output=await client.send_message(["1999"],"Ważny temat","Wiadomość <test>\nKoniec",confirm=True)
    assert output.accepted and output.message_id=="876"
    posts=[x for x in session.calls if x[0]=="POST"]
    assert len(posts)==1
    payload=posts[0][2]["json"]
    assert payload["receivers"]=={"schoolReceivers":[{"accountId":"1999"}]}
    assert base64.b64decode(payload["topic"]).decode()=="Ważny temat"
    body=base64.b64decode(payload["content"]).decode()
    assert body=="Wiadomość &lt;test&gt;\nKoniec"


@pytest.mark.asyncio
async def test_recipient_not_offered_by_school_is_blocked():
    client,session=service()
    contacts_responses(session)
    with pytest.raises(ValueError):
        await client.send_message(["1234567"],"Ważne","Treść",confirm=True)
    assert not any(x[0]=="POST" for x in session.calls)


@pytest.mark.asyncio
async def test_reply_reads_one_message_and_posts_once_without_auto_retry():
    client,session=service()
    session.add("GET",MESSAGE_API+"/inbox/messages/52",200,
                '{"data":{"senderId":"1999","topic":"Temat","noReply":0,"Message":""}}')
    session.add("GET",MESSAGE_API+"/me",200,'{"data":{}}')
    session.add("POST",MESSAGE_API+"/messages/reply",200,'')
    sent=await client.reply_to_message("52","Dziękuję",confirm=True)
    assert sent.accepted and sent.message_id is None
    calls=[x for x in session.calls if x[0]=="POST"]
    assert len(calls)==1
    payload=calls[0][2]["json"]
    assert payload["originalMessageId"]==52
    assert payload["receivers"]["schoolReceivers"][0]["accountId"]=="1999"
    assert base64.b64decode(payload["topic"]).decode()=="Re: Temat"


@pytest.mark.asyncio
async def test_no_reply_flag_prevents_post():
    client,session=service()
    session.add("GET",MESSAGE_API+"/inbox/messages/52",200,
                '{"data":{"senderId":"1999","topic":"Temat","noReply":1}}')
    with pytest.raises(ValueError):
        await client.reply_to_message("52","Dziękuję",confirm=True)
    assert not any(x[0]=="POST" for x in session.calls)


@pytest.mark.asyncio
@pytest.mark.parametrize("status,sent",[(400,False),(503,None),(302,None)])
async def test_http_failure_is_not_automatically_retried(status,sent):
    client,session=service()
    session.add("GET",MESSAGE_API+"/me",200,'{"data":{}}')
    session.add("POST",MESSAGE_API+"/messages",status,'{"errors":["bad"]}')
    with pytest.raises(MessageWriteUncertain) as exc:
        await client._send_once(["1999"],"Temat","Treść")
    assert exc.value.sent is sent
    assert len([x for x in session.calls if x[0]=="POST"])==1


@pytest.mark.asyncio
async def test_response_with_errors_is_ambiguous():
    client,session=service()
    session.add("GET",MESSAGE_API+"/me",200,'{"data":{}}')
    session.add("POST",MESSAGE_API+"/messages",201,'{"errors":["unexpected"]}')
    with pytest.raises(MessageWriteUncertain) as exc:
        await client._send_once(["1999"],"Temat","Treść")
    assert exc.value.sent is None


def test_no_production_integration_in_project():
    from pathlib import Path
    root=Path(__file__).resolve().parents[1]
    s=(root/"src/synergia_independent/client.py").read_text()
    assert "librus_synergia" not in s
    assert "synergia-pocket.onrender.com" not in s
    assert "MessageWriteUncertain" in s
