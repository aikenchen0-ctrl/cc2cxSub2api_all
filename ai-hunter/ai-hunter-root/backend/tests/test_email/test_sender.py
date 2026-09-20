from unittest.mock import MagicMock, patch

import pytest

from emailing.email_sender import send_email
from emailing.store import EmailStore


def _live_smtp_settings(tmp_path, **kwargs):
    values = {
        "email_dry_run": False,
        "email_block_freemail": True,
        "email_allow_generic_company_email": False,
        "email_blocked_countries": [],
        "email_db_path": str(tmp_path / "email.db"),
        "email_unsubscribe_secret": "test-secret",
        "email_public_base_url": "https://hunter.example",
        "api_host": "127.0.0.1",
        "api_port": 8000,
    }
    values.update(kwargs)
    return type("S", (), values)()


def _smtp_account():
    return {
        "provider_type": "smtp",
        "from_name": "B2Binsights",
        "from_email": "sales@example.com",
        "reply_to": "sales@example.com",
        "smtp_host": "smtp.example.com",
        "smtp_port": 587,
        "smtp_username": "sales@example.com",
        "smtp_secret_encrypted": "secret",
        "use_tls": 1,
    }


@pytest.mark.asyncio
async def test_send_email_missing_recipient():
    result = await send_email({}, to_email="", subject="Hi", body_text="Hello")
    assert result["ok"] is False
    assert result["error_type"] == "invalid_recipient"


@pytest.mark.asyncio
async def test_send_email_smtp_success(tmp_path):
    smtp = MagicMock()
    smtp.__enter__.return_value = smtp
    smtp.__exit__.return_value = False
    with (
        patch("emailing.email_sender.get_settings", return_value=_live_smtp_settings(tmp_path)),
        patch("emailing.email_sender.smtplib.SMTP", return_value=smtp),
    ):
        result = await send_email(_smtp_account(), to_email="buyer@acme.com", subject="Hi", body_text="Hello")
    assert result["ok"] is True
    assert result["provider"] == "smtp"
    assert result["provider_message_id"].startswith("<")
    sent_message = smtp.send_message.call_args.args[0]
    assert "List-Unsubscribe" in sent_message
    assert "List-Unsubscribe-Post" in sent_message
    assert "would rather not receive these emails" in sent_message.get_content()


@pytest.mark.asyncio
async def test_send_email_formats_plaintext_body_before_sending(tmp_path):
    smtp = MagicMock()
    smtp.__enter__.return_value = smtp
    smtp.__exit__.return_value = False
    with (
        patch("emailing.email_sender.get_settings", return_value=_live_smtp_settings(tmp_path)),
        patch("emailing.email_sender.smtplib.SMTP", return_value=smtp),
    ):
        await send_email(
            _smtp_account(),
            to_email="buyer@acme.com",
            subject="Hi",
            body_text=(
                "Dear Sir/Madam, We manufacture industrial switches for control systems. "
                "Given your product mix, there may be a fit. Kind regards,"
            ),
        )

    sent_message = smtp.send_message.call_args.args[0]
    assert "\n\n" in sent_message.get_content()
    assert "would rather not receive these emails" in sent_message.get_content()


@pytest.mark.asyncio
async def test_send_email_dry_run_does_not_touch_smtp(tmp_path):
    with (
        patch("emailing.email_sender.get_settings", return_value=_live_smtp_settings(tmp_path, email_dry_run=True)),
        patch("emailing.email_sender.smtplib.SMTP") as smtp,
    ):
        result = await send_email(
            {"provider_type": "smtp", "from_email": "sales@example.com"},
            to_email="buyer@acme.com",
            subject="Hi",
            body_text="Hello",
        )
    smtp.assert_not_called()
    assert result["ok"] is True
    assert result["provider"] == "dry_run"
    assert result["dry_run"] is True


@pytest.mark.asyncio
async def test_send_email_blocks_role_inbox(tmp_path):
    with patch("emailing.email_sender.get_settings", return_value=_live_smtp_settings(tmp_path)):
        result = await send_email(
            {"provider_type": "smtp", "from_email": "sales@example.com"},
            to_email="info@acme.com",
            subject="Hi",
            body_text="Hello",
        )
    assert result["ok"] is False
    assert result["error_type"] == "blocked_recipient"


@pytest.mark.asyncio
async def test_send_email_blocks_suppressed_recipient(tmp_path):
    settings = _live_smtp_settings(tmp_path)
    store = EmailStore(settings.email_db_path)
    store.init_db()
    store.add_suppression(
        "buyer@acme.com",
        reason="unsubscribed",
        source="recipient",
        created_at="2026-03-10T00:00:00Z",
    )
    with patch("emailing.email_sender.get_settings", return_value=settings):
        result = await send_email(
            {"provider_type": "smtp", "from_email": "sales@example.com"},
            to_email="buyer@acme.com",
            subject="Hi",
            body_text="Hello",
        )
    assert result["ok"] is False
    assert result["error_type"] == "blocked_recipient"
    assert "unsubscribed" in result["error"]
