import json
import tempfile
import time
import unittest
from pathlib import Path

from service.manager import MonitorManager


class FakeStorage:
    def get_all(self):
        return []


class FakeCore:
    def __init__(self, user_id, config, data_dir, log_callback):
        self.user_id = user_id
        self.config = config
        self.data_dir = Path(data_dir)
        self.storage = FakeStorage()
        self.log_callback = log_callback

    def run_once(self, stop_event=None):
        self.log_callback("run-complete")
        return {"new_count": 1, "failed_sites": [], "total_crawlers": 0}


class ManagerTests(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.created = []

        def factory(user_id, config, data_dir, log_callback):
            self.created.append((user_id, Path(data_dir)))
            return FakeCore(user_id, config, data_dir, log_callback)

        self.manager = MonitorManager(self.root, core_factory=factory)

    def tearDown(self):
        self.manager.close()

    def test_user_states_are_isolated(self):
        self.manager.update_config("1", {"keywords": ["alpha"]})
        self.manager.update_config("2", {"keywords": ["beta"]})

        first = self.manager.status("1")
        second = self.manager.status("2")
        self.assertNotEqual(first["data_dir"], second["data_dir"])
        self.assertEqual(first["user_id"], "1")
        self.assertEqual(second["user_id"], "2")

    def test_run_once_finishes_and_keeps_result(self):
        self.assertTrue(self.manager.run_once("1"))
        self.assertTrue(self.manager.wait_for_idle("1", timeout=2))
        status = self.manager.status("1")
        self.assertFalse(status["current_task_running"])
        self.assertEqual(status["last_result"]["new_count"], 1)
        self.assertEqual(self.created[0][0], "1")

    def test_start_and_stop_update_lifecycle(self):
        self.assertTrue(self.manager.start("1"))
        self.assertTrue(self.manager.wait_for_idle("1", timeout=2))
        self.assertTrue(self.manager.status("1")["is_running"])
        self.assertTrue(self.manager.stop("1"))
        self.assertFalse(self.manager.status("1")["is_running"])

    def test_sensitive_config_is_not_written(self):
        self.manager.update_config(
            "1",
            {
                "keywords": ["alpha"],
                "ai_config": {"api_key": "secret-value", "model": "test-model"},
                "email_config": {"password": "mail-secret"},
            },
        )
        config_file = self.root / "1" / "monitor.json"
        saved = json.loads(config_file.read_text(encoding="utf-8"))
        self.assertNotIn("secret-value", config_file.read_text(encoding="utf-8"))
        self.assertNotIn("mail-secret", config_file.read_text(encoding="utf-8"))
        self.assertEqual(saved["ai_config"]["model"], "test-model")

    def test_runtime_credentials_reach_core_but_never_persist(self):
        runtime = {
            "ai_config": {
                "enable": True,
                "api_key": "runtime-secret",
                "base_url": "https://model.example.test/chat/completions",
                "model": "runtime-model",
            },
            "email": "user@example.test",
            "phone": "13800000000",
            "notify_method": "both",
            "email_config": {
                "smtp_server": "smtp.example.test",
                "sender": "sender@example.test",
                "password": "smtp-secret",
                "receiver": "user@example.test",
            },
            "sms_config": {
                "provider": "aliyun",
                "access_key_id": "access-id",
                "access_key_secret": "sms-secret",
            },
        }
        self.assertTrue(self.manager.run_once("1", runtime))
        self.assertTrue(self.manager.wait_for_idle("1", timeout=2))
        self.assertEqual(self.created[0][0], "1")
        core_config = self.manager._states["1"].core.config
        self.assertEqual(core_config["ai_config"]["api_key"], "runtime-secret")
        self.assertEqual(core_config["email"], "user@example.test")
        self.assertEqual(core_config["phone"], "13800000000")
        self.assertEqual(core_config["email_config"]["password"], "smtp-secret")
        self.assertEqual(core_config["sms_config"]["access_key_secret"], "sms-secret")
        config_file = self.root / "1" / "monitor.json"
        if config_file.exists():
            self.assertNotIn("runtime-secret", config_file.read_text(encoding="utf-8"))
        self.assertNotIn("runtime-secret", str(self.manager.status("1")))


if __name__ == "__main__":
    unittest.main()
