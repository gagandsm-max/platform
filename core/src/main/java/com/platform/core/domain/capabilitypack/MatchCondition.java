package com.platform.core.domain.capabilitypack;

import java.util.Objects;

public record MatchCondition(
        MatchField field,
        MatchOperator operator,
        String value,
        boolean caseSensitive
) {
    public MatchCondition {
        Objects.requireNonNull(field, "field");
        Objects.requireNonNull(operator, "operator");
        Objects.requireNonNull(value, "value");
    }
}
