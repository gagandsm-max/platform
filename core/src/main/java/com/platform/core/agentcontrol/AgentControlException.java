package com.platform.core.agentcontrol;

import com.platform.core.domain.DomainException;

import java.util.Map;

/**
 * Control-plane Agent enrollment / credential / assignment errors (internal ops; not public REST).
 */
public class AgentControlException extends DomainException {

    public static final String ERR_AGENT_NOT_FOUND = "ERR_AGENT_NOT_FOUND";
    public static final String ERR_AGENT_ALREADY_EXISTS = "ERR_AGENT_ALREADY_EXISTS";
    public static final String ERR_AGENT_SITE_INVALID = "ERR_AGENT_SITE_INVALID";
    public static final String ERR_AGENT_STATE_INVALID = "ERR_AGENT_STATE_INVALID";
    public static final String ERR_AGENT_CREDENTIAL_DUPLICATE = "ERR_AGENT_CREDENTIAL_DUPLICATE";
    public static final String ERR_AGENT_CREDENTIAL_NOT_FOUND = "ERR_AGENT_CREDENTIAL_NOT_FOUND";
    public static final String ERR_AGENT_CREDENTIAL_INVALID = "ERR_AGENT_CREDENTIAL_INVALID";
    public static final String ERR_AGENT_ASSIGNMENT_CONFLICT = "ERR_AGENT_ASSIGNMENT_CONFLICT";
    public static final String ERR_AGENT_ASSIGNMENT_NOT_FOUND = "ERR_AGENT_ASSIGNMENT_NOT_FOUND";
    public static final String ERR_AGENT_SENSOR_INELIGIBLE = "ERR_AGENT_SENSOR_INELIGIBLE";
    public static final String ERR_AGENT_TENANT_MISMATCH = "ERR_AGENT_TENANT_MISMATCH";
    public static final String ERR_AGENT_SITE_INCOMPATIBLE = "ERR_AGENT_SITE_INCOMPATIBLE";
    /** Unique entity/Agent/certificate binding collision; governed reconciliation required. */
    public static final String ERR_AGENT_BINDING_IDENTITY_CONFLICT = "ERR_AGENT_BINDING_IDENTITY_CONFLICT";
    public static final String ERR_SENSOR_NOT_FOUND = "ERR_SENSOR_NOT_FOUND";
    /** Alias of {@link com.platform.core.control.ControlGenerationLimits#ERR_CONTROL_GENERATION_EXHAUSTED}. */
    public static final String ERR_CONTROL_GENERATION_EXHAUSTED =
            com.platform.core.control.ControlGenerationLimits.ERR_CONTROL_GENERATION_EXHAUSTED;

    public AgentControlException(String code, String message) {
        super(code, message);
    }

    public AgentControlException(String code, String message, Map<String, String> fieldErrors) {
        super(code, message, fieldErrors);
    }
}
