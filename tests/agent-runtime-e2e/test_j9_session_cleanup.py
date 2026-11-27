"""Focused session lifecycle tests; no Docker calls."""
import contextlib
import io
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import Mock, call
from real_sensor_commit_gap import close_psql_sessions

class J9SessionTests(unittest.TestCase):
    def proc(self, rc=0):
        p=Mock();p.returncode=rc;p.poll.return_value=rc;p.communicate.return_value=('', '');return p
    def test_two_real_children_complete_on_eof(self):
        with tempfile.TemporaryDirectory() as d:
            paths=[Path(d)/x for x in ('holder','t1')];ps=[]
            try:
                for path in paths:
                    ps.append(subprocess.Popen([sys.executable,'-c','import sys; from pathlib import Path; sys.stdin.read(); Path(sys.argv[1]).write_text("normal")',str(path)],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True))
                close_psql_sessions(ps)
                self.assertEqual([p.returncode for p in ps],[0,0]);self.assertEqual([p.read_text() for p in paths],['normal','normal'])
            finally:
                for p in ps:
                    if p.poll() is None:p.kill();p.communicate(timeout=5)
    def test_success_does_not_signal_and_preserves_order(self):
        h,t=self.proc(),self.proc();both=Mock();both.attach_mock(h,'holder');both.attach_mock(t,'t1')
        close_psql_sessions((h,t));self.assertEqual(both.mock_calls,[call.holder.communicate(timeout=30),call.t1.communicate(timeout=30)])
    def test_nonzero_still_attempts_both(self):
        h,t=self.proc(1),self.proc();
        with self.assertRaisesRegex(RuntimeError,'J9_PSQL_SESSION_CLOSE_FAILED'):close_psql_sessions((h,t))
        t.communicate.assert_called_once_with(timeout=30);h.terminate.assert_not_called()
    def test_primary_failure_preserved(self):
        h,t=self.proc(1),self.proc(2)
        with self.assertRaisesRegex(ValueError,'original'):
            try:raise ValueError('original')
            finally:close_psql_sessions((h,t),primary_failure=sys.exc_info()[0] is not None)
    def test_timeout_is_not_success(self):
        h,t=self.proc(),self.proc();h.poll.return_value=None;h.communicate.side_effect=[subprocess.TimeoutExpired('fixture',30),('', '')]
        with self.assertRaises(RuntimeError):close_psql_sessions((h,t))
        h.terminate.assert_called_once();h.kill.assert_not_called();t.communicate.assert_called_once_with(timeout=30)
    def test_timeout_escalation_bounded(self):
        h,t=self.proc(),self.proc();h.poll.return_value=None;h.communicate.side_effect=subprocess.TimeoutExpired('fixture',30)
        with self.assertRaises(RuntimeError):close_psql_sessions((h,t))
        h.terminate.assert_called_once();h.kill.assert_called_once();h.wait.assert_called_once_with(timeout=5);t.communicate.assert_called_once_with(timeout=30)
    def test_partial_session_creation(self):
        h=self.proc();close_psql_sessions((h,None),primary_failure=True);h.communicate.assert_called_once_with(timeout=30)
    def test_unexpected_io_failure(self):
        h,t=self.proc(),self.proc();h.communicate.side_effect=[OSError('fixture'),('', '')]
        with self.assertRaises(RuntimeError):close_psql_sessions((h,t))
        t.communicate.assert_called_once_with(timeout=30)
