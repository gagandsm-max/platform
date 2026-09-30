package com.platform.core.logging;

import ch.qos.logback.core.PropertyDefinerBase;

import java.util.UUID;

/**
 * Process-scoped service instance identity for structured logs.
 * Generated once per JVM; not persisted; not authorization or node authority.
 */
public final class ServiceInstanceIdPropertyDefiner extends PropertyDefinerBase {

    private static final String INSTANCE_ID = UUID.randomUUID().toString();

    @Override
    public String getPropertyValue() {
        return INSTANCE_ID;
    }

    /** Test/helper access to the JVM-stable value. */
    public static String currentInstanceId() {
        return INSTANCE_ID;
    }
}
