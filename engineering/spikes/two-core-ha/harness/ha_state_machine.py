import enum
import time
from typing import Dict, Optional, Set, Tuple


class CoreState(enum.Enum):
    ACTIVE = "ACTIVE"
    STANDBY = "STANDBY"
    FENCING = "FENCING"
    FENCED = "FENCED"
    PROMOTING = "PROMOTING"
    REJOINING = "REJOINING"
    AMBIGUOUS = "AMBIGUOUS"


class IllegalStateTransitionError(Exception):
    pass


class StaleEpochError(Exception):
    pass


class FutureEpochError(Exception):
    pass


VALID_TRANSITIONS: Dict[CoreState, Set[CoreState]] = {
    CoreState.STANDBY: {CoreState.FENCING, CoreState.FENCED, CoreState.STANDBY},
    CoreState.FENCING: {CoreState.PROMOTING, CoreState.AMBIGUOUS, CoreState.FENCED},
    CoreState.AMBIGUOUS: {CoreState.STANDBY, CoreState.FENCING, CoreState.FENCED},
    CoreState.PROMOTING: {CoreState.ACTIVE, CoreState.FENCED},
    CoreState.ACTIVE: {CoreState.FENCED, CoreState.STANDBY},
    CoreState.FENCED: {CoreState.REJOINING},
    CoreState.REJOINING: {CoreState.STANDBY, CoreState.FENCED},
}


class CoreNode:
    """In-process model of one Core node. Not a production fencing agent."""

    def __init__(self, node_id: str, initial_state: CoreState, initial_epoch: int = 1):
        self.node_id = node_id
        self.state = initial_state
        self.epoch = initial_epoch
        self.history = [(time.time(), initial_state, initial_epoch)]

    @property
    def is_writable(self) -> bool:
        return self.state == CoreState.ACTIVE

    def transition_to(self, new_state: CoreState, reason: str = ""):
        allowed = VALID_TRANSITIONS.get(self.state, set())
        if new_state not in allowed:
            raise IllegalStateTransitionError(
                f"Illegal state transition on node {self.node_id}: {self.state.value} -> {new_state.value} (Reason: {reason})"
            )
        self.state = new_state
        self.history.append((time.time(), new_state, self.epoch))

    def initiate_fencing(self) -> None:
        self.transition_to(CoreState.FENCING, "Peer heartbeat lost, initiating positive fencing")

    def handle_fence_outcome(self, fence_confirmed: bool) -> CoreState:
        """Standby may promote ONLY after positive fencing proof (lab analogue or production)."""
        if self.state != CoreState.FENCING:
            raise IllegalStateTransitionError(f"Node {self.node_id} cannot evaluate fence outcome from {self.state.value}")

        if fence_confirmed:
            self.transition_to(CoreState.PROMOTING, "Positive fencing confirmed, proceeding to promote")
            self.epoch += 1
            self.transition_to(CoreState.ACTIVE, f"Promotion complete; epoch incremented to {self.epoch}")
            return CoreState.ACTIVE
        self.transition_to(CoreState.AMBIGUOUS, "Fencing could NOT be confirmed; promotion forbidden")
        return CoreState.AMBIGUOUS

    def validate_write_epoch(self, write_epoch: int) -> bool:
        """
        Exact-authority fencing: writes must present the current Active epoch.
        Reject both stale (<) and future (>) epochs.
        """
        if not self.is_writable:
            return False
        if write_epoch < self.epoch:
            raise StaleEpochError(f"Rejected write with stale epoch {write_epoch} < current {self.epoch}")
        if write_epoch > self.epoch:
            raise FutureEpochError(f"Rejected write with future epoch {write_epoch} > current {self.epoch}")
        return True


class TwoCoreCluster:
    """In-process model enforcing the global no-split-brain invariant for FSM testing only."""

    def __init__(self):
        self.node_a = CoreNode("core-a", CoreState.ACTIVE, initial_epoch=1)
        self.node_b = CoreNode("core-b", CoreState.STANDBY, initial_epoch=1)

    def assert_no_split_brain(self) -> bool:
        a_active = self.node_a.state == CoreState.ACTIVE
        b_active = self.node_b.state == CoreState.ACTIVE
        if a_active and b_active:
            raise AssertionError(
                f"FATAL SPLIT-BRAIN DETECTED! Core A (epoch {self.node_a.epoch}) and Core B (epoch {self.node_b.epoch}) are BOTH ACTIVE!"
            )
        return True

    def simulate_partition_without_fencing(self) -> Tuple[CoreState, CoreState]:
        self.node_b.initiate_fencing()
        outcome = self.node_b.handle_fence_outcome(fence_confirmed=False)
        self.assert_no_split_brain()
        return self.node_a.state, outcome

    def simulate_partition_with_lab_fence_analogue(self) -> Tuple[CoreState, CoreState]:
        """
        Lab model: peer confirmed stopped (Docker kill analogue), not production STONITH.
        """
        self.node_b.initiate_fencing()
        self.node_a.transition_to(CoreState.FENCED, "Lab fencing analogue confirmed peer stopped")
        outcome = self.node_b.handle_fence_outcome(fence_confirmed=True)
        self.assert_no_split_brain()
        return self.node_a.state, outcome

    def simulate_old_active_rejoin(self) -> Tuple[CoreState, CoreState]:
        if self.node_a.state != CoreState.FENCED:
            self.node_a.state = CoreState.FENCED
        self.node_a.transition_to(CoreState.REJOINING, "Node A rebooted; rejoining cluster")
        assert self.node_b.epoch > self.node_a.epoch
        try:
            self.node_b.validate_write_epoch(self.node_a.epoch)
            raise AssertionError("stale epoch was incorrectly accepted")
        except StaleEpochError:
            pass
        try:
            self.node_b.validate_write_epoch(self.node_b.epoch + 10)
            raise AssertionError("future epoch was incorrectly accepted")
        except FutureEpochError:
            pass
        self.node_a.epoch = self.node_b.epoch
        self.node_a.transition_to(CoreState.STANDBY, "Resync complete; operating as Standby")
        self.assert_no_split_brain()
        return self.node_a.state, self.node_b.state
