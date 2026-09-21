from tools.lead_source_filter import is_usable_discovery_link


class TestIsUsableDiscoveryLink:
    def test_company_site(self):
        assert is_usable_discovery_link("https://energiedist.de/about") is True

    def test_drops_wikipedia(self):
        assert is_usable_discovery_link("https://en.wikipedia.org/wiki/Solar_inverter") is False
        assert is_usable_discovery_link("https://de.wikipedia.org/wiki/Photovoltaik") is False

    def test_drops_empty(self):
        assert is_usable_discovery_link("") is False

    def test_drops_dictionary_and_cn_portals(self):
        assert is_usable_discovery_link("https://baike.baidu.com/item/solar/1") is False
        assert is_usable_discovery_link("https://www.iciba.com/word?w=solar") is False
        assert is_usable_discovery_link("https://dictionary.cambridge.org/zhs/solar") is False
