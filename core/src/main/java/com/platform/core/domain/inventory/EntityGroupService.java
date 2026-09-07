package com.platform.core.domain.inventory;

import com.platform.core.api.PageResult;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.TextRules;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.id.UuidV7;
import com.platform.core.persistence.EntityGroupRepository;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.UUID;

@Service
public class EntityGroupService {

    private static final int MAX_HIERARCHY_DEPTH = 64;

    private final EntityGroupRepository groups;
    private final TenantService tenants;

    public EntityGroupService(EntityGroupRepository groups, TenantService tenants) {
        this.groups = groups;
        this.tenants = tenants;
    }

    @Transactional
    public EntityGroup create(UUID tenantId, UUID parentGroupId, String name, String description) {
        tenants.requireExists(tenantId);
        if (parentGroupId != null) {
            requireGroup(tenantId, parentGroupId);
        }
        Instant now = Instant.now();
        EntityGroup group = new EntityGroup(
                UuidV7.generate(),
                tenantId,
                parentGroupId,
                TextRules.requireTrimmed(name, "name", TextRules.NAME_MAX, true),
                TextRules.requireTrimmed(description, "description", TextRules.DESCRIPTION_MAX, false),
                now,
                now
        );
        groups.insert(group);
        return group;
    }

    public EntityGroup get(UUID tenantId, UUID groupId) {
        return requireGroup(tenantId, groupId);
    }

    public PageResult<EntityGroup> list(UUID tenantId, int page, int limit) {
        tenants.requireExists(tenantId);
        int p = PageResult.normalizePage(page);
        int l = PageResult.normalizeLimit(limit);
        long total = groups.countByTenant(tenantId);
        return new PageResult<>(groups.listByTenant(tenantId, l, PageResult.offset(p, l)), p, l, total, (long) p * l < total);
    }

    @Transactional
    public EntityGroup update(UUID tenantId, UUID groupId, UUID parentGroupId, boolean parentProvided,
                              String name, String description) {
        EntityGroup existing = requireGroup(tenantId, groupId);
        UUID newParent = parentProvided ? parentGroupId : existing.parentGroupId();
        if (newParent != null) {
            if (newParent.equals(groupId)) {
                throw new DomainException("HIERARCHY_CYCLE", "Group cannot be its own parent");
            }
            requireGroup(tenantId, newParent);
            assertNoCycle(tenantId, groupId, newParent);
        }
        EntityGroup updated = new EntityGroup(
                existing.groupId(),
                existing.tenantId(),
                newParent,
                name == null ? existing.name() : TextRules.requireTrimmed(name, "name", TextRules.NAME_MAX, true),
                description == null ? existing.description()
                        : TextRules.requireTrimmed(description, "description", TextRules.DESCRIPTION_MAX, false),
                existing.createdAt(),
                Instant.now()
        );
        groups.update(updated);
        return updated;
    }

    EntityGroup requireGroup(UUID tenantId, UUID groupId) {
        return groups.findByTenantAndId(tenantId, groupId)
                .orElseThrow(() -> new DomainException("GROUP_NOT_FOUND", "Group not found"));
    }

    private void assertNoCycle(UUID tenantId, UUID groupId, UUID proposedParentId) {
        UUID cursor = proposedParentId;
        int depth = 0;
        while (cursor != null) {
            if (cursor.equals(groupId)) {
                throw new DomainException("HIERARCHY_CYCLE", "Group hierarchy cycle detected");
            }
            if (++depth > MAX_HIERARCHY_DEPTH) {
                throw new DomainException("HIERARCHY_CYCLE", "Group hierarchy depth exceeds " + MAX_HIERARCHY_DEPTH);
            }
            cursor = groups.findByTenantAndId(tenantId, cursor)
                    .map(EntityGroup::parentGroupId)
                    .orElse(null);
        }
    }
}
