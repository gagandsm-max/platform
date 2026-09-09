package com.platform.core.domain.tenant;

import com.platform.core.api.PageResult;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.TextRules;
import com.platform.core.id.UuidV7;
import com.platform.core.persistence.TenantRepository;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.Collection;
import java.util.List;
import java.util.Set;
import java.util.UUID;

@Service
public class TenantService {

    private final TenantRepository tenants;

    public TenantService(TenantRepository tenants) {
        this.tenants = tenants;
    }

    @Transactional
    public Tenant create(String name, TenantMode mode, TenantStatus status, String timezone) {
        Instant now = Instant.now();
        Tenant tenant = new Tenant(
                UuidV7.generate(),
                TextRules.requireTrimmed(name, "name", TextRules.NAME_MAX, true),
                mode == null ? TenantMode.SINGLE : mode,
                status == null ? TenantStatus.ACTIVE : status,
                TextRules.requireTimezone(timezone == null ? "UTC" : timezone),
                now,
                now
        );
        tenants.insert(tenant);
        return tenant;
    }

    public Tenant get(UUID tenantId) {
        return tenants.findById(tenantId)
                .orElseThrow(() -> new DomainException("TENANT_NOT_FOUND", "Tenant not found"));
    }

    public PageResult<Tenant> list(int page, int limit) {
        int p = PageResult.normalizePage(page);
        int l = PageResult.normalizeLimit(limit);
        long total = tenants.count();
        return new PageResult<>(tenants.list(l, PageResult.offset(p, l)), p, l, total, (long) p * l < total);
    }

    /**
     * Lists tenants visible to the given scope. Scope is applied in persistence before pagination.
     * Empty scope fails closed to an empty page.
     */
    public PageResult<Tenant> listScoped(Collection<UUID> tenantScopes, int page, int limit) {
        int p = PageResult.normalizePage(page);
        int l = PageResult.normalizeLimit(limit);
        if (tenantScopes == null || tenantScopes.isEmpty()) {
            return new PageResult<>(List.of(), p, l, 0L, false);
        }
        Set<UUID> scope = Set.copyOf(tenantScopes);
        long total = tenants.countByIds(scope);
        return new PageResult<>(
                tenants.listByIds(scope, l, PageResult.offset(p, l)),
                p,
                l,
                total,
                (long) p * l < total
        );
    }

    @Transactional
    public Tenant update(UUID tenantId, String name, TenantMode mode, TenantStatus status, String timezone) {
        Tenant existing = get(tenantId);
        Tenant updated = new Tenant(
                existing.tenantId(),
                name == null ? existing.name() : TextRules.requireTrimmed(name, "name", TextRules.NAME_MAX, true),
                mode == null ? existing.mode() : mode,
                status == null ? existing.status() : status,
                timezone == null ? existing.timezone() : TextRules.requireTimezone(timezone),
                existing.createdAt(),
                Instant.now()
        );
        tenants.update(updated);
        return updated;
    }

    public void requireExists(UUID tenantId) {
        if (!tenants.exists(tenantId)) {
            throw new DomainException("TENANT_NOT_FOUND", "Tenant not found");
        }
    }

    public void lockForHierarchyMutation(UUID tenantId) {
        if (!tenants.lockForHierarchyMutation(tenantId)) {
            throw new DomainException("TENANT_NOT_FOUND", "Tenant not found");
        }
    }
}
