#!/usr/bin/env python3
"""Tests for free/infra/scripts/parity-envelope.py (D-0615).

Run: python3 free/infra/scripts/test_parity_envelope.py  (or pytest). No Docker, no network.
"""
import importlib.util
import os
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("pe", os.path.join(HERE, "parity-envelope.py"))
pe = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(pe)


def err_env(command="infra server destroy", message="refused", **over):
    e = {"class": "user", "exit_code": 1, "code": "E401", "message": message}
    e.update(over)
    return {"schema_version": "1", "command": command, "error": e}


class CheckEnvelope(unittest.TestCase):
    def test_good_error_envelope(self):
        self.assertEqual(pe.check_envelope(err_env(), "destroy"), [])

    def test_command_must_be_infra_server_sub(self):
        for cmd in ("infra", "infra plan", "infra server list", "server destroy"):
            self.assertTrue(pe.check_envelope(err_env(command=cmd), "destroy"), cmd)

    def test_message_must_be_a_string(self):
        for bad in (None, 7, ["x"], {"a": 1}):
            errs = pe.check_envelope(err_env(message=bad), "destroy")
            self.assertTrue(any("error.message" in e for e in errs), bad)
        e = err_env()
        del e["error"]["message"]
        self.assertTrue(any("error.message" in x for x in pe.check_envelope(e, "destroy")))

    def test_error_that_is_not_an_object(self):
        doc = {"schema_version": "1", "command": "infra server destroy", "error": "boom"}
        self.assertTrue(pe.check_envelope(doc, "destroy"))


class DestroyData(unittest.TestCase):
    DATA = {"server_id": 1, "snapshot_id": None, "retained_ips": [], "released_ips": []}

    def compare(self, core, **over):
        data = dict(self.DATA)
        data.update(over)
        return pe.compare_data("destroy", core, data)

    def test_null_lists_equal_empty_lists(self):
        self.assertEqual(self.compare({"SnapshotID": None, "RetainedIPs": None, "ReleasedIPs": None}), [])
        self.assertEqual(self.compare({"SnapshotID": None, "RetainedIPs": [], "ReleasedIPs": []}, retained_ips=None), [])

    def test_other_empty_values_are_not_an_empty_list(self):
        for falsy in (False, 0, "", {}):
            core = {"SnapshotID": None, "RetainedIPs": falsy, "ReleasedIPs": []}
            self.assertTrue(self.compare(core), "core %r" % (falsy,))
            core = {"SnapshotID": None, "RetainedIPs": [], "ReleasedIPs": []}
            self.assertTrue(self.compare(core, released_ips=falsy), "head %r" % (falsy,))


if __name__ == "__main__":
    unittest.main()
