"""Offline source-drift and stale-output checks for the pull-agent reference."""

import contextlib
import io
from pathlib import Path
import runpy
import sys
import unittest
from unittest.mock import Mock, patch

SCRIPT = Path(__file__).with_name("generate-ci-agent-reference.py")
GEN = runpy.run_path(str(SCRIPT))


class AgentReferenceTests(unittest.TestCase):
    def test_actual_checkout_is_current(self):
        self.assertEqual(GEN["PAGE"].read_text(), GEN["render"]())

    def test_new_source_flags_and_reasons_reach_reference(self):
        original = Path.read_text

        def changed(path, *args, **kwargs):
            text = original(path, *args, **kwargs)
            if path.name == "pull.go":
                text += '\nfs.BoolVar(&f.future, "future-flag", false, "future option")\n'
            if path.name == "codes.go":
                text += '\nconst ReasonFuture = "future_token_reason"\n'
                text += '\nconst ReasonFutureTransport = "transport.agent_future"\n'
            return text

        with patch.object(Path, "read_text", changed):
            rendered = GEN["render"]()
        for expected in ("`--future-flag`", "`future_token_reason`", "`transport.agent_future`"):
            self.assertIn(expected, rendered)
        self.assertNotEqual(rendered, GEN["PAGE"].read_text())

    def test_unrecognized_source_fails_closed(self):
        with patch.object(Path, "read_text", return_value=""):
            with self.assertRaisesRegex(ValueError, "cannot read"):
                GEN["render"]()

    def test_check_rejects_missing_and_stale_output(self):
        for exists in (False, True):
            page = Mock()
            page.exists.return_value = exists
            page.read_text.return_value = "stale reference"
            with patch.dict(GEN["main"].__globals__, PAGE=page):
                with patch.object(sys, "argv", [str(SCRIPT), "--check"]):
                    with contextlib.redirect_stderr(io.StringIO()):
                        self.assertEqual(GEN["main"](), 1)
            page.write_text.assert_not_called()


if __name__ == "__main__":
    unittest.main()
