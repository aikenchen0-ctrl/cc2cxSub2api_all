"""Tests for emailing/guards.py — freemail, role inbox, send window."""

from datetime import datetime, timezone

from emailing.guards import (
    is_freemail_domain,
    is_role_or_generic_email,
    looks_like_personal_email,
    screen_recipient,
    screen_send_window,
)


def test_role_and_generic_inboxes():
    assert is_role_or_generic_email("info@acme.com") is True
    assert is_role_or_generic_email("sales@acme.com") is True
    assert is_role_or_generic_email("ceo@acme.com") is True
    assert is_role_or_generic_email("jane.doe@acme.com") is False


def test_personal_email_shape():
    assert looks_like_personal_email("jane.doe@acme.com") is True
    assert looks_like_personal_email("info@acme.com") is False


def test_freemail_domains_include_cn_providers():
    assert is_freemail_domain("gmail.com") is True
    assert is_freemail_domain("qq.com") is True
    assert is_freemail_domain("163.com") is True
    assert is_freemail_domain("acme.com") is False


def test_screen_recipient_blocks_freemail_and_role():
    settings = type("S", (), {"email_block_freemail": True, "email_allow_generic_company_email": False, "email_blocked_countries": []})()
    blocked_role = screen_recipient("info@acme.com", settings=settings, sender_email="sales@example.com")
    blocked_free = screen_recipient("jane.doe@gmail.com", settings=settings, sender_email="sales@example.com")
    ok = screen_recipient("jane.doe@acme.com", settings=settings, sender_email="sales@example.com")
    assert blocked_role.ok is False
    assert blocked_free.ok is False
    assert ok.ok is True


def test_screen_recipient_blocks_suppressed():
    settings = type("S", (), {"email_block_freemail": True, "email_allow_generic_company_email": False, "email_blocked_countries": []})()
    store = type("Store", (), {"is_suppressed": staticmethod(lambda email: email == "jane.doe@acme.com")})()
    blocked = screen_recipient(
        "jane.doe@acme.com",
        settings=settings,
        sender_email="sales@example.com",
        store=store,
    )
    ok = screen_recipient(
        "pat.lee@acme.com",
        settings=settings,
        sender_email="sales@example.com",
        store=store,
    )
    assert blocked.ok is False
    assert "unsubscribed" in blocked.reason
    assert ok.ok is True


def test_screen_send_window_daily_cap_and_hours():
    weekday_morning = datetime(2026, 3, 9, 10, 0, tzinfo=timezone.utc)
    cap = screen_send_window(
        sent_today=50,
        daily_cap=50,
        now=weekday_morning,
        timezone_name="UTC",
        weekdays_only=True,
    )
    assert cap.ok is False
    assert "daily cap" in cap.reason

    outside = screen_send_window(
        sent_today=0,
        daily_cap=50,
        now=datetime(2026, 3, 9, 20, 0, tzinfo=timezone.utc),
        timezone_name="UTC",
        weekdays_only=True,
    )
    assert outside.ok is False

    inside = screen_send_window(
        sent_today=0,
        daily_cap=50,
        now=weekday_morning,
        timezone_name="UTC",
        weekdays_only=True,
    )
    assert inside.ok is True
