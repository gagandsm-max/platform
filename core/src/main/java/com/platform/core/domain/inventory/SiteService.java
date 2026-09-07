package com.platform.core.domain.inventory;

import com.platform.core.api.PageResult;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.TextRules;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.id.UuidV7;
import com.platform.core.persistence.SiteRepository;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.UUID;

@Service
public class SiteService {

    private static final int MAX_HIERARCHY_DEPTH = 64;

    private final SiteRepository sites;
    private final TenantService tenants;

    public SiteService(SiteRepository sites, TenantService tenants) {
        this.sites = sites;
        this.tenants = tenants;
    }

    @Transactional
    public Site create(UUID tenantId, UUID parentSiteId, String name, String description, String timezone) {
        tenants.lockForHierarchyMutation(tenantId);
        if (parentSiteId != null) {
            requireSite(tenantId, parentSiteId);
        }
        Instant now = Instant.now();
        Site site = new Site(
                UuidV7.generate(),
                tenantId,
                parentSiteId,
                TextRules.requireTrimmed(name, "name", TextRules.NAME_MAX, true),
                TextRules.requireTrimmed(description, "description", TextRules.DESCRIPTION_MAX, false),
                TextRules.optionalTimezone(timezone),
                now,
                now
        );
        sites.insert(site);
        return site;
    }

    public Site get(UUID tenantId, UUID siteId) {
        return requireSite(tenantId, siteId);
    }

    public PageResult<Site> list(UUID tenantId, int page, int limit) {
        tenants.requireExists(tenantId);
        int p = PageResult.normalizePage(page);
        int l = PageResult.normalizeLimit(limit);
        long total = sites.countByTenant(tenantId);
        return new PageResult<>(sites.listByTenant(tenantId, l, PageResult.offset(p, l)), p, l, total, (long) p * l < total);
    }

    @Transactional
    public Site update(UUID tenantId, UUID siteId, UUID parentSiteId, boolean parentProvided,
                       String name, String description, String timezone) {
        tenants.lockForHierarchyMutation(tenantId);
        Site existing = requireSite(tenantId, siteId);
        UUID newParent = parentProvided ? parentSiteId : existing.parentSiteId();
        if (newParent != null) {
            if (newParent.equals(siteId)) {
                throw new DomainException("HIERARCHY_CYCLE", "Site cannot be its own parent");
            }
            requireSite(tenantId, newParent);
            assertNoCycle(tenantId, siteId, newParent);
        }
        Site updated = new Site(
                existing.siteId(),
                existing.tenantId(),
                newParent,
                name == null ? existing.name() : TextRules.requireTrimmed(name, "name", TextRules.NAME_MAX, true),
                description == null ? existing.description()
                        : TextRules.requireTrimmed(description, "description", TextRules.DESCRIPTION_MAX, false),
                timezone == null ? existing.timezone() : TextRules.optionalTimezone(timezone),
                existing.createdAt(),
                Instant.now()
        );
        sites.update(updated);
        return updated;
    }

    private Site requireSite(UUID tenantId, UUID siteId) {
        return sites.findByTenantAndId(tenantId, siteId)
                .orElseThrow(() -> new DomainException("SITE_NOT_FOUND", "Site not found"));
    }

    private void assertNoCycle(UUID tenantId, UUID siteId, UUID proposedParentId) {
        UUID cursor = proposedParentId;
        int depth = 0;
        while (cursor != null) {
            if (cursor.equals(siteId)) {
                throw new DomainException("HIERARCHY_CYCLE", "Site hierarchy cycle detected");
            }
            if (++depth > MAX_HIERARCHY_DEPTH) {
                throw new DomainException("HIERARCHY_CYCLE", "Site hierarchy depth exceeds " + MAX_HIERARCHY_DEPTH);
            }
            cursor = sites.findByTenantAndId(tenantId, cursor)
                    .map(Site::parentSiteId)
                    .orElse(null);
        }
    }
}
