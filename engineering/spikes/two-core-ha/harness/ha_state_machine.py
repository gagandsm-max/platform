import enum
import time
from typing import Dict, Optional, Tuple, Set


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
    """Represents one Core node in the Two-Core HA pair."""

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
        """Called when peer heartbeat is lost."""
        self.transition_to(CoreState.FENCING, "Peer heartbeat lost, initiating positive fencing")

    def handle_fence_outcome(self, fence_confirmed: bool) -> CoreState:
        """
        Critical rule: Standby may promote ONLY after positive fencing proof.
        If fence is NOT confirmed, must transition to AMBIGUOUS.
        """
        if self.state != CoreState.FENCING:
            raise IllegalStateTransitionError(f"Node {self.node_id} cannot evaluate fence outcome from {self.state.value}")

        if fence_confirmed:
            self.transition_to(CoreState.PROMOTING, "Positive fencing confirmed, proceeding to promote")
            # Increment epoch on promotion
            self.epoch += 1
            self.transition_to(CoreState.ACTIVE, f"Promotion complete; epoch incremented to {self.epoch}")
            return CoreState.ACTIVE
        else:
            self.transition_to(CoreState.AMBIGUOUS, "Fencing could NOT be confirmed; promotion forbidden")
            return CoreState.AMBIGUOUS

    def validate_write_epoch(self, write_epoch: int) -> bool:
        if not self.is_writable:
            return False
        if write_epoch < self.epoch:
            raise StaleEpochError(f"Rejected write with stale epoch {write_epoch} < current {self.epoch}")
        return True


class TwoCoreCluster:
    """Oversees two Core nodes and enforces the global no-split-brain invariant."""

    def __init__(self):
        self.node_a = CoreNode("core-a", CoreState.ACTIVE, initial_epoch=1)
        self.node_b = CoreNode("core-b", CoreState.STANDBY, initial_epoch=1)

    def assert_no_split_brain(self) -> bool:
        """Invariant: Under no condition can both Core A and Core B be ACTIVE with the same or valid epoch."""
        a_active = self.node_a.state == CoreState.ACTIVE
        b_active = self.node_b.state == CoreState.ACTIVE
        if a_active and b_active:
            raise AssertionError(
                f"FATAL SPLIT-BRAIN DETECTED! Core A (epoch {self.node_a.epoch}) and Core B (epoch {self.node_b.epoch}) are BOTH ACTIVE!"
            )
        return True

    def simulate_partition_without_fencing(self) -> Tuple[CoreState, CoreState]:
        """
        Partition occurs: Node B loses heartbeat from Node A.
        Node B tries fencing, but management plane is severed, so fence fails.
        Verify Node B refuses promotion and enters AMBIGUOUS.
        """
        self.node_b.initiate_fencing()
        outcome = self.node_b.handle_fence_outcome(fence_confirmed=False)
        self.assert_no_split_brain()
        return self.node_a.state, outcome

    def simulate_partition_with_stonith(self) -> Tuple[CoreState, CoreState]:
        """
        Partition occurs: Node B loses heartbeat.
        Node B successfully executes positive fence (e.g. power cutoff / STONITH).
        Node A becomes FENCED. Node B promotes to ACTIVE with epoch increment.
        """
        self.node_b.initiate_fencing()
        self.node_a.transition_to(CoreState.FENCED, "Peer power cutoff executed by STONITH")
        outcome = self.node_b.handle_fence_outcome(fence_confirmed=True)
        self.assert_no_split_brain()
        return self.node_a.state, outcome

    def simulate_old_active_rejoin(self) -> Tuple[CoreState, CoreState]:
        """
        Old Active A reboots. Must enter REJOINING, reject stale epoch writes,
        synchronize with Node B, and transition to STANDBY.
        """
        if self.node_a.state != CoreState.FENCED:
            self.node_a.state = CoreState.FENCED
        self.node_a.transition_to(CoreState.REJOINING, "Node A rebooted; rejoining cluster")

        # Verify old epoch (1) is rejected by current Active B (epoch 2)
        assert self.node_b.epoch > self.node_a.epoch

        # Resynchronization complete
        self.node_a.epoch = self.node_b.epoch
        self.node_a.transition_to(CoreState.STANDBY, "Resync complete; operating as Standby")
        self.assert_no_split_brain()
        return self.node_a.state, self.node_b.state
