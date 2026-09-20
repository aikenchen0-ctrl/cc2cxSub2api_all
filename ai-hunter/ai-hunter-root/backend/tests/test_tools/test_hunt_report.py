"""Tests for tools/hunt_report.py — operator report, no per-lead reason."""

from tools.hunt_report import render_hunt_report


def _sample_result() -> dict:
    return {
        "insight": {
            "company_name": "SolarTech",
            "summary": "Inverters for distributors",
            "products": ["solar inverter"],
            "industries": ["Energy"],
            "target_customer_profile": "EU distributors",
        },
        "used_keywords": ["solar inverter distributor DE", "solar inverter distributor DE"],
        "hunt_round": 2,
        "search_result_count": 40,
        "keyword_search_stats": {
            "solar inverter distributor DE": {"result_count": 40, "leads_found": 4},
        },
        "round_feedback": {
            "round": 2,
            "new_leads_this_round": 4,
            "total_leads": 4,
            "target": 10,
            "best_keywords": ["solar inverter distributor DE"],
            "worst_keywords": ["cheap solar"],
            "keyword_performance": [
                {
                    "keyword": "solar inverter distributor DE",
                    "search_results": 40,
                    "leads_found": 4,
                    "precision": 0.1,
                    "avg_match_score": 0.6,
                    "effectiveness": "high",
                }
            ],
            "industry_distribution": {"Energy": 4},
            "region_distribution": {"DE": 3, "NL": 1},
            "search_backend_error": "ddg_html: 502 Bad Gateway",
        },
        "leads": [
            {
                "company_name": "Acme",
                "emails": ["jane@acme.example"],
                "reason": "Operator-only reason must not appear",
            }
        ],
    }


def test_report_includes_insight_search_and_effectiveness():
    body = render_hunt_report("hunt-abc12345", _sample_result(), exported_at="2026-09-17T11:00:00Z")
    assert "Hunt ID : hunt-abc12345" in body
    assert "SolarTech" in body
    assert "solar inverter distributor DE" in body
    assert "SEARCH COUNTS (raw)" in body
    assert "40 | 4" in body
    assert "KEYWORD EFFECTIVENESS" in body
    assert "10.0%" in body
    assert "高" in body
    assert "Energy 4" in body
    assert "DE 3" in body
    assert "Search blocked: ddg_html: 502 Bad Gateway" in body


def test_report_excludes_lead_reason_and_emails():
    body = render_hunt_report("hunt-abc12345", _sample_result(), exported_at="fixed")
    assert "Operator-only reason must not appear" not in body
    assert "jane@acme.example" not in body


def test_report_includes_per_keyword_search_error():
    result = _sample_result()
    result["keyword_search_stats"] = {
        "solar inverter distributor DE": {
            "result_count": 0,
            "leads_found": 0,
            "error": "ddg_lite: timeout",
        },
    }
    body = render_hunt_report("hunt-abc12345", result, exported_at="fixed")
    assert "error: ddg_lite: timeout" in body


def test_empty_result_still_has_header():
    body = render_hunt_report("hunt-empty", {}, exported_at="fixed")
    assert "INSIGHT / KEYWORD / EFFECTIVENESS REPORT" in body
    assert "Hunt ID : hunt-empty" in body
    assert "KEYWORD EFFECTIVENESS" not in body
