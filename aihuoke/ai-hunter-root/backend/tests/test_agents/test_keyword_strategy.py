from agents.keyword_strategy import keyword_system_prompt


def _settings(**overrides):
    defaults = {"search_backend": "auto", "serper_api_key": ""}
    defaults.update(overrides)
    return type("S", (), defaults)()


class TestKeywordSystemPrompt:
    def test_web_prompt_without_serper(self):
        prompt = keyword_system_prompt(_settings(), n=4, local_language_instruction="English")
        assert "DuckDuckGo" in prompt
        assert "PHYSICAL businesses" not in prompt

    def test_maps_prompt_with_serper(self):
        prompt = keyword_system_prompt(
            _settings(serper_api_key="serper-test-key"),
            n=4,
            local_language_instruction="English",
        )
        assert "Google Maps" in prompt
        assert "DuckDuckGo" not in prompt
