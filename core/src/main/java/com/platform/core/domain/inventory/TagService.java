package com.platform.core.domain.inventory;

import com.platform.core.api.PageResult;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.TextRules;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.id.UuidV7;
import com.platform.core.persistence.TagRepository;
import org.springframework.dao.DuplicateKeyException;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.UUID;

@Service
public class TagService {

    private final TagRepository tags;
    private final TenantService tenants;

    public TagService(TagRepository tags, TenantService tenants) {
        this.tags = tags;
        this.tenants = tenants;
    }

    @Transactional
    public Tag create(UUID tenantId, String tagKey, String tagValue) {
        tenants.requireExists(tenantId);
        Tag tag = new Tag(
                UuidV7.generate(),
                tenantId,
                TextRules.requireTrimmed(tagKey, "key", TextRules.TAG_KEY_MAX, true),
                TextRules.requireTrimmed(tagValue, "value", TextRules.TAG_VALUE_MAX, false),
                Instant.now()
        );
        try {
            tags.insert(tag);
        } catch (DuplicateKeyException ex) {
            throw new DomainException("RESOURCE_CONFLICT", "Tag key/value already exists for tenant");
        }
        return tag;
    }

    public Tag get(UUID tenantId, UUID tagId) {
        return requireTag(tenantId, tagId);
    }

    public PageResult<Tag> list(UUID tenantId, int page, int limit) {
        tenants.requireExists(tenantId);
        int p = PageResult.normalizePage(page);
        int l = PageResult.normalizeLimit(limit);
        long total = tags.countByTenant(tenantId);
        return new PageResult<>(tags.listByTenant(tenantId, l, PageResult.offset(p, l)), p, l, total, (long) p * l < total);
    }

    Tag requireTag(UUID tenantId, UUID tagId) {
        return tags.findByTenantAndId(tenantId, tagId)
                .orElseThrow(() -> new DomainException("TAG_NOT_FOUND", "Tag not found"));
    }
}
