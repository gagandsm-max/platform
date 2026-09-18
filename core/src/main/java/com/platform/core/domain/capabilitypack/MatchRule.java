package com.platform.core.domain.capabilitypack;

import java.util.List;
import java.util.Objects;

public record MatchRule(String ruleId, int priority, List<MatchCondition> conditions) {
    public MatchRule {
        Objects.requireNonNull(ruleId, "ruleId");
        conditions = List.copyOf(Objects.requireNonNull(conditions, "conditions"));
    }

    public int exactCount() {
        int count = 0;
        for (MatchCondition condition : conditions) {
            if (condition.operator() == MatchOperator.EXACT) {
                count++;
            }
        }
        return count;
    }
}
