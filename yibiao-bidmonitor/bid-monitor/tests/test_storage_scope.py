import tempfile
import unittest
from pathlib import Path

from service.scope import safe_user_id, user_db_path, user_data_path
from src.database.storage import BidInfo, Storage


class StorageScopeTests(unittest.TestCase):
    def test_user_id_must_be_a_positive_integer(self):
        self.assertEqual(safe_user_id(7), "7")
        self.assertEqual(safe_user_id("12"), "12")
        with self.assertRaises(ValueError):
            safe_user_id("../12")
        with self.assertRaises(ValueError):
            safe_user_id("0")
        with self.assertRaises(ValueError):
            safe_user_id("user")

    def test_user_paths_stay_inside_data_root(self):
        root = Path(tempfile.mkdtemp())
        data_path = user_data_path(root, 7)
        db_path = user_db_path(root, 7)
        self.assertEqual(data_path, root / "7")
        self.assertEqual(db_path, root / "7" / "bids.db")
        self.assertTrue(data_path.is_relative_to(root))

    def test_user_databases_are_isolated(self):
        root = Path(tempfile.mkdtemp())
        first = Storage(str(user_db_path(root, 1)))
        second = Storage(str(user_db_path(root, 2)))
        bid = BidInfo(
            title="Scope test",
            url="https://example.test/bid/1",
            publish_date="2026-09-19",
            source="test",
        )

        self.assertTrue(first.save(bid))
        self.assertEqual(first.count_all(), 1)
        self.assertEqual(second.count_all(), 0)
        self.assertFalse(first.save(bid))
        first.clear_all()
        self.assertEqual(first.count_all(), 0)
        self.assertEqual(second.count_all(), 0)


if __name__ == "__main__":
    unittest.main()
