package com.platform.core.domain.capabilitypack;

import com.platform.core.domain.DomainException;

import java.util.List;
import java.util.Map;

/**
 * Fail-closed Capability Pack validation failure.
 * Codes mirror {@code device-packs/tools/validate.mjs} where applicable.
 */
public final class CapabilityPackValidationException extends DomainException {

    private final List<Issue> issues;

    public CapabilityPackValidationException(String code, String message) {
        this(code, message, List.of(), Map.of());
    }

    public CapabilityPackValidationException(String code, String message, List<Issue> issues) {
        this(code, message, issues, Map.of());
    }

    public CapabilityPackValidationException(String code, String message, List<Issue> issues, Map<String, String> fieldErrors) {
        super(code, message, fieldErrors);
        this.issues = issues == null ? List.of() : List.copyOf(issues);
    }

    public List<Issue> getIssues() {
        return issues;
    }

    public record Issue(String code, String path, String message) {
        public Issue {
            if (code == null || code.isBlank()) {
                throw new IllegalArgumentException("issue code required");
            }
            path = path == null ? "$" : path;
            message = message == null ? "" : message;
        }
    }
}
