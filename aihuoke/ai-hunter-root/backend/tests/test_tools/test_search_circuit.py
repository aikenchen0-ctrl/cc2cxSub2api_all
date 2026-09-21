"""Tests for tools/search_circuit.py."""

from tools.search_circuit import SearchCircuit


def test_starts_open():
    circuit = SearchCircuit()
    assert circuit.tripped() is False
    assert circuit.error == ""


def test_trip_keeps_first_error():
    circuit = SearchCircuit()
    circuit.trip("ddg_html: timeout")
    circuit.trip("ddg_lite: 502")
    circuit.trip("   ")
    assert circuit.tripped() is True
    assert circuit.error == "ddg_html: timeout"


def test_blank_trip_is_ignored():
    circuit = SearchCircuit()
    circuit.trip("")
    circuit.trip("   ")
    assert circuit.tripped() is False
    assert circuit.error == ""
