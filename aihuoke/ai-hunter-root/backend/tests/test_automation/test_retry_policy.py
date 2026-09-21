from automation.retry_policy import is_permanent_failure, should_give_up


class TestRetryPolicy:
    def test_401_is_permanent(self):
        assert is_permanent_failure("AuthenticationError: Error code: 401") is True
        assert should_give_up(attempt_count=1, error_message="unauthorized_error", max_attempts=3) is True

    def test_503_retries_until_cap(self):
        assert should_give_up(attempt_count=1, error_message="Service temporarily unavailable", max_attempts=3) is False
        assert should_give_up(attempt_count=3, error_message="Service temporarily unavailable", max_attempts=3) is True

    def test_empty_is_not_permanent(self):
        assert is_permanent_failure("") is False
