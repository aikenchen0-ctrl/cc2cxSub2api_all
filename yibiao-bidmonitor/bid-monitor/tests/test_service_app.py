import unittest

from fastapi.testclient import TestClient

from service.app import create_app


class FakeManager:
    def __init__(self):
        self.updated = []

    def status(self, user_id):
        return {"user_id": str(user_id), "is_running": False}

    def start(self, user_id):
        return True

    def stop(self, user_id):
        return True

    def run_once(self, user_id):
        return True

    def update_config(self, user_id, config):
        self.updated.append((str(user_id), config))
        return config

    def results(self, user_id, limit, offset):
        return {"user_id": str(user_id), "items": [], "limit": limit, "offset": offset}

    def logs(self, user_id, limit):
        return [f"log:{user_id}:{limit}"]

    def clear_history(self, user_id):
        return None

    def sites(self, user_id):
        return {"sites": [], "custom_sites": []}

    def update_sites(self, user_id, enabled_sites, custom_sites):
        return {"sites": [], "custom_sites": custom_sites}

    def test_notification(self, user_id, channel, target, runtime_config=None):
        return {"success": True, "channel": channel, "target": target}

    def test_ai(self, user_id, runtime_config=None):
        return {"success": True, "relevant": True, "reason": "ok"}


class ServiceAppTests(unittest.TestCase):
    def setUp(self):
        self.manager = FakeManager()
        self.client = TestClient(create_app(self.manager, service_token="test-token"))

    def test_internal_routes_require_service_token(self):
        response = self.client.get("/internal/health")
        self.assertEqual(response.status_code, 401)
        response = self.client.get(
            "/internal/health",
            headers={"X-BidMonitor-Service-Token": "test-token"},
        )
        self.assertEqual(response.status_code, 200)
        self.assertEqual(response.json()["status"], "ok")

    def test_user_actions_are_forwarded_to_manager(self):
        headers = {"X-BidMonitor-Service-Token": "test-token"}
        response = self.client.put(
            "/internal/users/7/config",
            headers=headers,
            json={"keywords": ["alpha"]},
        )
        self.assertEqual(response.status_code, 200)
        self.assertEqual(self.manager.updated, [("7", {"keywords": ["alpha"]})])

        response = self.client.post("/internal/users/7/run-once", headers=headers)
        self.assertEqual(response.status_code, 200)
        self.assertTrue(response.json()["accepted"])

        response = self.client.get("/internal/users/7/logs?limit=4", headers=headers)
        self.assertEqual(response.status_code, 200)
        self.assertEqual(response.json()["logs"], ["log:7:4"])

    def test_invalid_user_id_is_rejected(self):
        response = self.client.get(
            "/internal/users/../status",
            headers={"X-BidMonitor-Service-Token": "test-token"},
        )
        self.assertIn(response.status_code, (404, 400))

    def test_extended_monitor_actions_are_forwarded(self):
        headers = {"X-BidMonitor-Service-Token": "test-token"}
        response = self.client.get("/internal/users/7/sites", headers=headers)
        self.assertEqual(response.status_code, 200)
        response = self.client.put(
            "/internal/users/7/sites",
            headers=headers,
            json={"enabled_sites": ["chinabidding"], "custom_sites": []},
        )
        self.assertEqual(response.status_code, 200)
        response = self.client.post(
            "/internal/users/7/test-notification",
            headers=headers,
            json={"channel": "email", "target": "user@example.test"},
        )
        self.assertEqual(response.status_code, 200)
        response = self.client.post("/internal/users/7/test-ai", headers=headers)
        self.assertEqual(response.status_code, 200)


if __name__ == "__main__":
    unittest.main()
