"""Regression cases for free/infra/scripts/parity-envelope.py (D-0615).

The script compares core's legacy JSON with the plugin's v1 envelope. It must
treat only a missing (null) IP list as empty, and must check the envelope's
error message type and its command path.
"""
import importlib.util
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[3] / "free" / "infra" / "scripts" / "parity-envelope.py"
spec = importlib.util.spec_from_file_location("parity_envelope", SCRIPT)
pe = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pe)


def destroy_data(retained, released):
    return {"server_id": "1", "snapshot_id": "s", "retained_ips": retained, "released_ips": released}


def core_destroy(retained, released):
    return {"SnapshotID": "s", "RetainedIPs": retained, "ReleasedIPs": released}


def error_envelope(**over):
    err = {"class": "user", "exit_code": 1, "code": "E401", "message": "bad flag"}
    err.update(over)
    return {"schema_version": "1", "command": "infra server list", "error": err}


class DestroyIPs(unittest.TestCase):
    def test_null_equals_empty(self):
        self.assertEqual(pe.compare_data("destroy", core_destroy(None, None), destroy_data([], [])), [])
        self.assertEqual(pe.compare_data("destroy", core_destroy([], []), destroy_data(None, None)), [])

    def test_falsy_non_list_is_a_difference(self):
        for bad in (False, 0, "", {}):
            with self.subTest(core=bad):
                errs = pe.compare_data("destroy", core_destroy(bad, []), destroy_data([], []))
                self.assertTrue(errs, "a falsy non-list core IP value must not equal []")
            with self.subTest(head=bad):
                errs = pe.compare_data("destroy", core_destroy([], []), destroy_data([], bad))
                self.assertTrue(errs, "a falsy non-list head IP value must not equal []")

    def test_equal_lists_pass(self):
        self.assertEqual(pe.compare_data("destroy", core_destroy(["1.2.3.4"], []), destroy_data(["1.2.3.4"], [])), [])


class EnvelopeShape(unittest.TestCase):
    def test_valid_error_envelope(self):
        self.assertEqual(pe.check_envelope(error_envelope()), [])

    def test_error_message_must_be_a_string(self):
        for bad in (None, 5, ["x"], {}):
            with self.subTest(message=bad):
                self.assertTrue(pe.check_envelope(error_envelope(message=bad)))

    def test_command_must_be_under_infra_server(self):
        for ok in ("infra server list", "infra server provision"):
            doc = error_envelope()
            doc["command"] = ok
            self.assertEqual(pe.check_envelope(doc), [], ok)
        for bad in ("infra plan", "k8s server list", "infra serverx list"):
            doc = error_envelope()
            doc["command"] = bad
            with self.subTest(command=bad):
                self.assertTrue(pe.check_envelope(doc))


if __name__ == "__main__":
    unittest.main()
