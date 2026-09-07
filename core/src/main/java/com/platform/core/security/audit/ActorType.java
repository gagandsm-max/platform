package com.platform.core.security.audit;

/**
 * Actor kinds recorded on security audit facts.
 * POLICY_ENGINE is intentionally absent: policy evaluation acts on behalf of USER/SERVICE/SYSTEM
 * and is not itself a Platform security actor identity.
 */
public enum ActorType {
    USER,
    SERVICE,
    SYSTEM
}
