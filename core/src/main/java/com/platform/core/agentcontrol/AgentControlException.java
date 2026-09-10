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
    public static final String ERR_SENSOR_NOT_FOUND = "ERR_SENSOR_NOT_FOUND";

    public AgentControlException(String code, String message) {
        super(code, message);
    }

    public AgentControlException(String code, String message, Map<String, String> fieldErrors) {
        super(code, message, fieldErrors);
    }
}
