package com.platform.core.api;

import java.util.List;

public record PageResult<T>(
        List<T> items,
        int page,
        int limit,
        long total,
        boolean hasMore
) {
    public static final int DEFAULT_LIMIT = 50;
    public static final int MAX_LIMIT = 200;

    public static int normalizePage(int page) {
        if (page < 1) {
            throw new com.platform.core.domain.DomainException("QUERY_LIMIT_EXCEEDED", "page must be >= 1");
        }
        return page;
    }

    public static int normalizeLimit(Integer limit) {
        int value = limit == null ? DEFAULT_LIMIT : limit;
        if (value < 1) {
            throw new com.platform.core.domain.DomainException("QUERY_LIMIT_EXCEEDED", "limit must be >= 1");
        }
        if (value > MAX_LIMIT) {
            throw new com.platform.core.domain.DomainException("QUERY_LIMIT_EXCEEDED", "limit must be <= " + MAX_LIMIT);
        }
        return value;
    }

    public static int offset(int page, int limit) {
        return (page - 1) * limit;
    }
}
