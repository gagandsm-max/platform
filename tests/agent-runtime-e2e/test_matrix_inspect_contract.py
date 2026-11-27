"""Fake-only tests of the durability matrix's exact Docker inspect forms."""
import unittest
from types import SimpleNamespace
from unittest.mock import Mock, patch
import isolation

PREFIX = 'agent-e2e-' + '1' * 32
REF = 'a' * 64
HEALTH = '{{.State.Health.Status}}'
START = '{{.State.StartedAt}}'


class MatrixInspectTests(unittest.TestCase):
    def fixture(self, service='core', present=True):
        name = PREFIX+'-'+service
        item = {'Id': REF, 'Name': '/'+name}
        return SimpleNamespace(prefix=PREFIX, docker=['/fake/docker'],
            active=Mock(return_value={'container': {REF:(name,item)} if present else {}, 'volume': {}}),
            inspect=Mock(return_value=item), check=Mock())

    def read(self, c, fmt, target, kind='container'):
        return isolation.resource_read(c, ['--format', fmt, target], kind, 'inspect')

    def admitted(self, service, fmt, by_id=False):
        c = self.fixture(service)
        with patch.object(isolation.os,'execv') as command:
            self.read(c, fmt, REF if by_id else PREFIX+'-'+service)
        c.active.assert_called_once_with()
        c.inspect.assert_called_once_with('container', REF)
        c.check.assert_called_once_with('container', REF, PREFIX+'-'+service, c.inspect.return_value)
        command.assert_called_once_with('/fake/docker', ['/fake/docker','container','inspect','--format',fmt,REF])

    def denied(self, c, fmt, target, kind='container'):
        with patch.object(isolation.os,'execv') as command:
            with self.assertRaises(ValueError): self.read(c,fmt,target,kind)
            command.assert_not_called()

    def test_core_health(self): self.admitted('core',HEALTH)
    def test_sensor_health(self): self.admitted('sensor-infra',HEALTH)
    def test_core_started_at(self): self.admitted('core',START)
    def test_core_started_at_by_id(self): self.admitted('core',START,True)

    def test_wrong_services(self):
        for service,fmt in [('agent-runtime',HEALTH),('kafka',HEALTH),('sensor-infra',START),('postgres',START)]:
            with self.subTest(service=service,fmt=fmt):
                self.denied(self.fixture(service),fmt,PREFIX+'-'+service)

    def test_missing_and_foreign_target(self):
        self.denied(self.fixture(present=False),HEALTH,PREFIX+'-core')
        self.denied(self.fixture(),START,'foreign-core')

    def test_no_arbitrary_format_or_volume_access(self):
        self.denied(self.fixture(),'{{.Config.Env}}',PREFIX+'-core')
        self.denied(self.fixture(),START,'volume','volume')
        self.denied(self.fixture(),HEALTH,'volume','volume')

    def test_ownership_recheck_failure(self):
        c=self.fixture(); c.check.side_effect=ValueError('identity drift')
        self.denied(c,START,PREFIX+'-core')

    def test_reservation_failure(self):
        c=self.fixture(); c.active.side_effect=ValueError('reservation missing')
        self.denied(c,HEALTH,PREFIX+'-core')

    def test_existing_bootstrap_health_missing_still_retryable(self):
        c=self.fixture(present=False)
        with patch.object(isolation.os,'execv') as command:
            self.assertEqual(self.read(c,isolation.HEALTH_FORMAT,PREFIX+'-core'),1)
            command.assert_not_called()
