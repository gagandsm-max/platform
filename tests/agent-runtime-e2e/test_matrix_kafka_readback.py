"""Fake-only tests: Kafka partition deadline and complete readback receipts."""
import contextlib
import io
import json
import subprocess
import unittest
from unittest.mock import patch
from types import SimpleNamespace
from sequence_disposition import Matrix

IDLE = "[fixture] ERROR Error processing message, terminating consumer process: (org.apache.kafka.tools.consumer.ConsoleConsumer)\norg.apache.kafka.common.errors.TimeoutException\n"


def output(rows=(), rc=0, stderr=None):
    rows=list(rows)
    return subprocess.CompletedProcess([],rc,"".join(json.dumps(r)+"\n" for r in rows), IDLE+f"Processed a total of {len(rows)} messages\n" if stderr is None else stderr)


class KafkaReadbackTests(unittest.TestCase):
    def run_probe(self, replies):
        with patch('sequence_disposition.subprocess.run', side_effect=replies) as call, contextlib.redirect_stdout(io.StringIO()):
            result=Matrix.kafka_metric(SimpleNamespace(prefix='agent-e2e-fixture'), 'wanted')
        return result, call

    def test_all_partitions_and_outer_budget(self):
        result,call=self.run_probe([output([{'metricId':'wanted'}]),output(),output([{'metricId':'other'},{'metricId':'wanted'}])])
        self.assertEqual(len(result),2)
        self.assertEqual(call.call_count,3)
        for n,c in enumerate(call.call_args_list):
            self.assertEqual(c.kwargs,{'text':True,'capture_output':True,'timeout':90})
            args=c.args[0];self.assertEqual(args[args.index('--partition')+1],str(n));self.assertEqual(args[-2:],['--timeout-ms','3000']);self.assertIn('--from-beginning',args)

    def test_empty_complete_read(self): self.assertEqual(self.run_probe([output(),output(),output()])[0],[])
    def test_outer_timeout_is_not_empty(self):
        with self.assertRaises(subprocess.TimeoutExpired):self.run_probe([subprocess.TimeoutExpired(['docker'],90)])
    def test_nonzero_not_empty(self):
        with self.assertRaises(AssertionError):self.run_probe([output(rc=1)])
    def test_error_with_zero_exit_not_empty(self):
        with self.assertRaises(AssertionError):self.run_probe([output(stderr='ERROR broker authentication failed\nProcessed a total of 0 messages\n')])
    def test_missing_count(self):
        with self.assertRaises(AssertionError):self.run_probe([output(stderr=IDLE)])
    def test_count_mismatch(self):
        with self.assertRaises(AssertionError):self.run_probe([output(stderr=IDLE+'Processed a total of 1 messages\n')])
    def test_extra_error(self):
        with self.assertRaises(AssertionError):self.run_probe([output(stderr='ERROR transport failed\n'+IDLE+'Processed a total of 0 messages\n')])
    def test_invalid_record(self):
        for bad in ({}, {'metricId':None}, 'not a record'):
            with self.subTest(bad=bad), self.assertRaises(AssertionError): self.run_probe([output([bad])])
    def test_later_partition_failure_does_not_return_partial_hits(self):
        with self.assertRaises(AssertionError):self.run_probe([output([{'metricId':'wanted'}]),output(rc=1)])
