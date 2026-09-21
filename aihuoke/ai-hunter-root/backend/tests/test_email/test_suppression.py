from pathlib import Path

from emailing.store import EmailStore
from emailing.suppression import (
    apply_unsubscribe,
    looks_like_unsubscribe_request,
    unsubscribe_headers,
    unsubscribe_token,
    unsubscribe_url,
    verify_unsubscribe_token,
    with_unsubscribe_footer,
)


def test_token_roundtrip_and_url():
    secret = "test-secret"
    email = "buyer@acme.com"
    token = unsubscribe_token(email, secret=secret)
    assert token
    assert verify_unsubscribe_token(email, token, secret=secret) is True
    assert verify_unsubscribe_token(email, "nope", secret=secret) is False
    assert verify_unsubscribe_token("other@acme.com", token, secret=secret) is False

    settings = type(
        "S",
        (),
        {
            "email_unsubscribe_secret": secret,
            "email_public_base_url": "https://hunter.example",
            "api_host": "0.0.0.0",
            "api_port": 8000,
        },
    )()
    url = unsubscribe_url(email, settings=settings)
    assert url.startswith("https://hunter.example/api/v1/unsubscribe?")
    assert "email=buyer%40acme.com" in url
    assert f"token={token}" in url


def test_headers_include_mailto_and_one_click():
    settings = type(
        "S",
        (),
        {
            "email_unsubscribe_secret": "test-secret",
            "email_public_base_url": "https://hunter.example",
        },
    )()
    headers = unsubscribe_headers(
        "buyer@acme.com",
        settings=settings,
        sender_email="sales@example.com",
    )
    assert "mailto:sales@example.com?subject=unsubscribe" in headers["List-Unsubscribe"]
    assert "https://hunter.example/api/v1/unsubscribe" in headers["List-Unsubscribe"]
    assert headers["List-Unsubscribe-Post"] == "List-Unsubscribe=One-Click"


def test_footer_appended_once():
    body = "Hello Jane,\n\nThere may be a fit."
    with_footer = with_unsubscribe_footer(body, "https://hunter.example/api/v1/unsubscribe?email=x")
    assert "would rather not receive these emails" in with_footer
    again = with_unsubscribe_footer(with_footer, "https://hunter.example/api/v1/unsubscribe?email=x")
    assert again == with_footer


def test_unsubscribe_markers():
    assert looks_like_unsubscribe_request("Please unsubscribe me") is True
    assert looks_like_unsubscribe_request("退订") is True
    assert looks_like_unsubscribe_request("Interested, let's talk.") is False
    quoted = (
        "Thanks, let's talk next week.\n"
        "> If you would rather not receive these emails, reply with UNSUBSCRIBE\n"
        "> or use: https://hunter.example/api/v1/unsubscribe?email=x"
    )
    assert looks_like_unsubscribe_request(quoted) is False


def test_store_suppression_roundtrip_and_stop_sequences(tmp_path: Path):
    store = EmailStore(str(tmp_path / "email.db"))
    store.init_db()
    store.create_campaign({
        "id": "cmp_1",
        "hunt_id": "hunt_1",
        "email_account_id": "acct_1",
        "name": "Test",
        "status": "active",
        "language_mode": "auto_by_region",
        "default_language": "en",
        "fallback_language": "en",
        "tone": "professional",
        "step1_delay_days": 0,
        "step2_delay_days": 3,
        "step3_delay_days": 3,
        "min_fit_score": 0.6,
        "min_contactability_score": 0.45,
        "created_at": "2026-03-09T00:00:00Z",
        "updated_at": "2026-03-09T00:00:00Z",
    })
    store.create_sequence({
        "id": "seq_1",
        "campaign_id": "cmp_1",
        "hunt_id": "hunt_1",
        "lead_key": "w:acme.com",
        "lead_email": "buyer@acme.com",
        "lead_name": "Acme",
        "decision_maker_name": "Jane",
        "decision_maker_title": "Purchasing Manager",
        "locale": "en",
        "status": "scheduled",
        "current_step": 0,
        "stop_reason": "",
        "replied_at": "",
        "last_sent_at": "",
        "next_scheduled_at": "2026-03-09T00:00:00Z",
        "created_at": "2026-03-09T00:00:00Z",
        "updated_at": "2026-03-09T00:00:00Z",
    })
    store.create_message({
        "id": "msg_1",
        "sequence_id": "seq_1",
        "step_number": 1,
        "goal": "intro",
        "locale": "en",
        "subject": "Hello",
        "body_text": "Body",
        "status": "pending",
        "scheduled_at": "2026-03-09T00:00:00Z",
        "sent_at": "",
        "provider_message_id": "",
        "thread_key": "",
        "failure_reason": "",
        "created_at": "2026-03-09T00:00:00Z",
        "updated_at": "2026-03-09T00:00:00Z",
    })
    assert store.is_suppressed("buyer@acme.com") is False
    record = apply_unsubscribe(
        store,
        "Buyer@Acme.com",
        reason="unsubscribed",
        source="recipient",
        created_at="2026-03-10T00:00:00Z",
    )
    assert record["email"] == "buyer@acme.com"
    assert record["sequences_stopped"] == 1
    assert store.is_suppressed("buyer@acme.com") is True
    seq = store.get_sequence("seq_1")
    assert seq is not None
    assert seq["status"] == "stopped"
    assert seq["stop_reason"] == "unsubscribed"
    assert store.get_message("msg_1")["status"] == "cancelled"
    secret = store.unsubscribe_secret()
    assert len(secret) >= 32
    assert store.unsubscribe_secret() == secret
