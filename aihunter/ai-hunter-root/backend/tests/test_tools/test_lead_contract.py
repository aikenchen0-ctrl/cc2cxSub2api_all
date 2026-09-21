"""Tests for tools/lead_contract.py — reason, identity, Instantly CSV."""

from tools.lead_contract import (
    CSV_COLUMNS,
    compose_lead_reason,
    finalize_lead_contract,
    has_usable_contact,
    lead_to_csv_rows,
    leads_to_csv,
    should_spend_verify_credits,
)


def test_compose_reason_prefers_fit_reasons():
    lead = {
        "company_name": "Acme",
        "fit_reasons": ["Sells hardware to distributors.", "Active in SE Asia."],
    }
    assert compose_lead_reason(lead).startswith("Sells hardware")


def test_finalize_lead_contract_fills_identity():
    lead = finalize_lead_contract({
        "company_name": "Acme Tools",
        "website": "https://acme.example",
        "decision_makers": [{"name": "Jane Doe", "title": "Purchasing Manager"}],
        "fit_reasons": ["Matches the distributor profile."],
    })
    assert lead["first_name"] == "jane"
    assert lead["last_name"] == "doe"
    assert lead["title"] == "Purchasing Manager"
    assert lead["lead_id"].startswith("lead_")
    assert "distributor" in lead["reason"]
    assert lead["qualified_at"]


def test_csv_columns_align_with_instantly():
    assert CSV_COLUMNS == [
        "email",
        "first_name",
        "last_name",
        "company",
        "title",
        "website",
        "linkedin_url",
        "reason",
        "lead_id",
        "qualified_at",
    ]
    lead = {
        "company_name": "Acme Tools",
        "website": "https://acme.example",
        "emails": ["jane.doe@acme.example"],
        "decision_makers": [{"name": "Jane Doe", "title": "Buyer", "email": "jane.doe@acme.example"}],
        "reason": "Distributor of industrial hardware.",
    }
    rows = lead_to_csv_rows(lead)
    assert rows[0]["email"] == "jane.doe@acme.example"
    assert rows[0]["company"] == "Acme Tools"
    csv_body = leads_to_csv([lead])
    assert csv_body.splitlines()[0] == ",".join(CSV_COLUMNS)
    assert "Distributor of industrial hardware" in csv_body


def test_has_usable_contact_skips_role_inbox():
    assert has_usable_contact({"emails": ["info@acme.com"]}) is False
    assert has_usable_contact({"emails": ["jane.doe@acme.com"]}) is True
    assert has_usable_contact({
        "emails": ["info@acme.com"],
        "decision_makers": [{"email": "jane.doe@acme.com"}],
    }) is True


def test_should_spend_verify_credits_gate():
    settings = type("S", (), {"email_verify_min_fit_score": 0.6})()
    low = {"fit_score": 0.2, "reason": "weak"}
    high = {"fit_score": 0.8, "reason": "Distributor of industrial hardware."}
    assert should_spend_verify_credits(low, settings) is False
    assert should_spend_verify_credits(high, settings) is True
