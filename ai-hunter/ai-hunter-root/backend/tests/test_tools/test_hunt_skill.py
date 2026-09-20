"""Guardrails for the self-written hunt-leads skill (thought source only)."""

from pathlib import Path


SKILL = Path(__file__).resolve().parents[3] / "skills" / "hunt-leads" / "SKILL.md"


def test_hunt_leads_skill_exists_and_keeps_the_contract():
    text = SKILL.read_text(encoding="utf-8")
    assert SKILL.is_file()
    assert "name: hunt-leads" in text
    assert "email, first_name, last_name, company, title, website, linkedin_url, reason, lead_id, qualified_at" in text
    assert "reason is the point" in text or "**`reason` is the point.**" in text
    assert "no score column" in text
    assert "Never paste it into a message to the lead" in text
    assert "CSV is the integration" in text
    assert "never run unasked" in text.lower() or "Never do that unless the user asked" in text
    assert "email_dry_run" in text
    assert "refs/OpenOutreach" in text
    assert "openoutreach find" not in text.lower()
    assert "licensed-finder/search" in text
    assert "enrich_emails" in text
