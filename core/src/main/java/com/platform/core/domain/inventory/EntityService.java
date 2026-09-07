package com.platform.core.domain.inventory;

import com.platform.core.api.PageResult;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.TextRules;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.id.UuidV7;
import com.platform.core.persistence.EntityAttributeRepository;
import com.platform.core.persistence.EntityIdentifierRepository;
import com.platform.core.persistence.EntityRepository;
import com.platform.core.persistence.MembershipRepository;
import com.platform.core.persistence.SiteRepository;
import org.springframework.dao.DuplicateKeyException;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.math.BigDecimal;
import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.UUID;

@Service
public class EntityService {

    public static final int MAX_INITIAL_IDENTIFIERS = 200;
    public static final int ATTRIBUTE_JSON_MAX = 65_536;

    private final EntityRepository entities;
    private final EntityIdentifierRepository identifiers;
    private final EntityAttributeRepository attributes;
    private final MembershipRepository memberships;
    private final SiteRepository sites;
    private final EntityGroupService groups;
    private final TagService tags;
    private final TenantService tenants;

    public EntityService(
            EntityRepository entities,
            EntityIdentifierRepository identifiers,
            EntityAttributeRepository attributes,
            MembershipRepository memberships,
            SiteRepository sites,
            EntityGroupService groups,
            TagService tags,
            TenantService tenants
    ) {
        this.entities = entities;
        this.identifiers = identifiers;
        this.attributes = attributes;
        this.memberships = memberships;
        this.sites = sites;
        this.groups = groups;
        this.tags = tags;
        this.tenants = tenants;
    }

    @Transactional
    public Entity create(
            UUID tenantId,
            String entityType,
            String displayName,
            String description,
            String discoveredName,
            LifecycleState lifecycleState,
            UUID siteId,
            String criticality,
            UUID ownerTeamId,
            String devicePackId,
            String devicePackVersion,
            String createdBy,
            List<IdentifierInput> initialIdentifiers
    ) {
        tenants.requireExists(tenantId);
        if (initialIdentifiers != null && initialIdentifiers.size() > MAX_INITIAL_IDENTIFIERS) {
            throw new DomainException("QUERY_LIMIT_EXCEEDED",
                    "identifiers must contain at most " + MAX_INITIAL_IDENTIFIERS + " items");
        }
        if (siteId != null) {
            sites.findByTenantAndId(tenantId, siteId)
                    .orElseThrow(() -> new DomainException("SITE_NOT_FOUND", "Site not found"));
        }
        Instant now = Instant.now();
        String actor = TextRules.requireTrimmed(createdBy, "createdBy", TextRules.CREATED_BY_MAX, false);
        if (actor == null) {
            actor = "system";
        }
        Entity entity = new Entity(
                UuidV7.generate(),
                tenantId,
                TextRules.requireTrimmed(entityType, "entityType", TextRules.TYPE_MAX, true),
                TextRules.requireTrimmed(displayName, "displayName", TextRules.NAME_MAX, true),
                TextRules.requireTrimmed(description, "description", TextRules.DESCRIPTION_MAX, false),
                TextRules.requireTrimmed(discoveredName, "discoveredName", TextRules.NAME_MAX, false),
                lifecycleState == null ? LifecycleState.DISCOVERED : lifecycleState,
                siteId,
                TextRules.requireTrimmed(criticality, "criticality", TextRules.TYPE_MAX, false),
                ownerTeamId,
                TextRules.requireTrimmed(devicePackId, "devicePackId", TextRules.DEVICE_PACK_ID_MAX, false),
                TextRules.requireTrimmed(devicePackVersion, "devicePackVersion", TextRules.DEVICE_PACK_VERSION_MAX, false),
                now,
                now,
                actor,
                now,
                now
        );
        entities.insert(entity);
        if (initialIdentifiers != null) {
            for (IdentifierInput input : initialIdentifiers) {
                addIdentifier(tenantId, entity.entityId(), input, false);
            }
        }
        return entity;
    }

    public Entity get(UUID tenantId, UUID entityId) {
        return requireEntity(tenantId, entityId);
    }

    public PageResult<Entity> list(UUID tenantId, int page, int limit) {
        tenants.requireExists(tenantId);
        int p = PageResult.normalizePage(page);
        int l = PageResult.normalizeLimit(limit);
        long total = entities.countByTenant(tenantId);
        return new PageResult<>(entities.listByTenant(tenantId, l, PageResult.offset(p, l)), p, l, total, (long) p * l < total);
    }

    @Transactional
    public Entity update(
            UUID tenantId,
            UUID entityId,
            String entityType,
            String displayName,
            String description,
            String discoveredName,
            LifecycleState lifecycleState,
            UUID siteId,
            boolean siteProvided,
            String criticality,
            UUID ownerTeamId,
            boolean ownerProvided,
            String devicePackId,
            String devicePackVersion,
            Instant lastSeenAt
    ) {
        Entity existing = requireEntity(tenantId, entityId);
        UUID newSiteId = siteProvided ? siteId : existing.siteId();
        if (newSiteId != null) {
            sites.findByTenantAndId(tenantId, newSiteId)
                    .orElseThrow(() -> new DomainException("SITE_NOT_FOUND", "Site not found"));
        }
        Instant newLastSeen = lastSeenAt == null ? existing.lastSeenAt() : lastSeenAt;
        if (newLastSeen.isBefore(existing.firstSeenAt())) {
            throw new DomainException("VALIDATION_FAILED", "lastSeenAt must be >= firstSeenAt",
                    Map.of("lastSeenAt", "ordering"));
        }
        Entity updated = new Entity(
                existing.entityId(),
                existing.tenantId(),
                entityType == null ? existing.entityType()
                        : TextRules.requireTrimmed(entityType, "entityType", TextRules.TYPE_MAX, true),
                displayName == null ? existing.displayName()
                        : TextRules.requireTrimmed(displayName, "displayName", TextRules.NAME_MAX, true),
                description == null ? existing.description()
                        : TextRules.requireTrimmed(description, "description", TextRules.DESCRIPTION_MAX, false),
                discoveredName == null ? existing.discoveredName()
                        : TextRules.requireTrimmed(discoveredName, "discoveredName", TextRules.NAME_MAX, false),
                lifecycleState == null ? existing.lifecycleState() : lifecycleState,
                newSiteId,
                criticality == null ? existing.criticality()
                        : TextRules.requireTrimmed(criticality, "criticality", TextRules.TYPE_MAX, false),
                ownerProvided ? ownerTeamId : existing.ownerTeamId(),
                devicePackId == null ? existing.devicePackId()
                        : TextRules.requireTrimmed(devicePackId, "devicePackId", TextRules.DEVICE_PACK_ID_MAX, false),
                devicePackVersion == null ? existing.devicePackVersion()
                        : TextRules.requireTrimmed(devicePackVersion, "devicePackVersion", TextRules.DEVICE_PACK_VERSION_MAX, false),
                existing.firstSeenAt(),
                newLastSeen,
                existing.createdBy(),
                existing.createdAt(),
                Instant.now()
        );
        entities.update(updated);
        return updated;
    }

    @Transactional
    public EntityIdentifier addIdentifier(UUID tenantId, UUID entityId, IdentifierInput input) {
        return addIdentifier(tenantId, entityId, input, true);
    }

    private EntityIdentifier addIdentifier(UUID tenantId, UUID entityId, IdentifierInput input, boolean requireEntity) {
        if (requireEntity) {
            requireEntity(tenantId, entityId);
        }
        Instant now = Instant.now();
        Instant firstSeen = input.firstSeenAt() == null ? now : input.firstSeenAt();
        Instant lastSeen = input.lastSeenAt() == null ? firstSeen : input.lastSeenAt();
        if (lastSeen.isBefore(firstSeen)) {
            throw new DomainException("VALIDATION_FAILED", "lastSeenAt must be >= firstSeenAt",
                    Map.of("lastSeenAt", "ordering"));
        }
        BigDecimal confidence = requireConfidence(input.confidence());
        EntityIdentifier identifier = new EntityIdentifier(
                UuidV7.generate(),
                tenantId,
                entityId,
                TextRules.requireTrimmed(input.identifierType(), "identifierType", TextRules.TYPE_MAX, true),
                TextRules.requireTrimmed(input.namespace(), "namespace", TextRules.NAMESPACE_MAX, true),
                TextRules.requireTrimmed(input.identifierValue(), "identifierValue", TextRules.IDENTIFIER_VALUE_MAX, true),
                confidence,
                firstSeen,
                lastSeen
        );
        try {
            identifiers.insert(identifier);
        } catch (DuplicateKeyException ex) {
            throw new DomainException("IDENTIFIER_CONFLICT",
                    "Identifier already exists for tenant/namespace/type/value");
        }
        return identifier;
    }

    public PageResult<EntityIdentifier> listIdentifiers(UUID tenantId, UUID entityId, int page, int limit) {
        requireEntity(tenantId, entityId);
        int p = PageResult.normalizePage(page);
        int l = PageResult.normalizeLimit(limit);
        long total = identifiers.countByEntity(tenantId, entityId);
        return new PageResult<>(
                identifiers.listByEntity(tenantId, entityId, l, PageResult.offset(p, l)),
                p, l, total, (long) p * l < total
        );
    }

    @Transactional
    public void removeIdentifier(UUID tenantId, UUID entityId, UUID identifierId) {
        requireEntity(tenantId, entityId);
        int deleted = identifiers.delete(tenantId, entityId, identifierId);
        if (deleted == 0) {
            throw new DomainException("IDENTIFIER_NOT_FOUND", "Identifier not found");
        }
    }

    public EntityIdentifier resolveIdentifier(
            UUID tenantId, String namespace, String identifierType, String identifierValue
    ) {
        tenants.requireExists(tenantId);
        return identifiers.findByResolutionKey(
                        tenantId,
                        TextRules.requireTrimmed(namespace, "namespace", TextRules.NAMESPACE_MAX, true),
                        TextRules.requireTrimmed(identifierType, "identifierType", TextRules.TYPE_MAX, true),
                        TextRules.requireTrimmed(identifierValue, "identifierValue", TextRules.IDENTIFIER_VALUE_MAX, true)
                )
                .orElseThrow(() -> new DomainException("IDENTIFIER_NOT_FOUND", "Identifier not found"));
    }

    @Transactional
    public EntityAttribute upsertAttribute(
            UUID tenantId, UUID entityId, String schemaKey, String valueJson, String source, Instant observedAt
    ) {
        requireEntity(tenantId, entityId);
        EntityAttribute attribute = new EntityAttribute(
                UuidV7.generate(),
                tenantId,
                entityId,
                TextRules.requireTrimmed(schemaKey, "schemaKey", TextRules.SCHEMA_KEY_MAX, true),
                TextRules.requireTrimmed(valueJson, "valueJson", ATTRIBUTE_JSON_MAX, true),
                TextRules.requireTrimmed(source, "source", TextRules.SOURCE_MAX, true),
                observedAt == null ? Instant.now() : observedAt
        );
        attributes.upsert(attribute);
        return attribute;
    }

    @Transactional
    public GroupMembership assignGroup(UUID tenantId, UUID entityId, UUID groupId) {
        requireEntity(tenantId, entityId);
        groups.requireGroup(tenantId, groupId);
        GroupMembership membership = new GroupMembership(tenantId, entityId, groupId, Instant.now());
        memberships.assignGroup(membership);
        return membership;
    }

    @Transactional
    public void unassignGroup(UUID tenantId, UUID entityId, UUID groupId) {
        requireEntity(tenantId, entityId);
        groups.requireGroup(tenantId, groupId);
        memberships.unassignGroup(tenantId, entityId, groupId);
    }

    public PageResult<GroupMembership> listGroups(UUID tenantId, UUID entityId, int page, int limit) {
        requireEntity(tenantId, entityId);
        int p = PageResult.normalizePage(page);
        int l = PageResult.normalizeLimit(limit);
        long total = memberships.countGroupsForEntity(tenantId, entityId);
        return new PageResult<>(
                memberships.listGroupsForEntity(tenantId, entityId, l, PageResult.offset(p, l)),
                p, l, total, (long) p * l < total
        );
    }

    @Transactional
    public EntityTagAssignment assignTag(UUID tenantId, UUID entityId, UUID tagId) {
        requireEntity(tenantId, entityId);
        tags.requireTag(tenantId, tagId);
        EntityTagAssignment assignment = new EntityTagAssignment(tenantId, entityId, tagId, Instant.now());
        memberships.assignTag(assignment);
        return assignment;
    }

    @Transactional
    public void unassignTag(UUID tenantId, UUID entityId, UUID tagId) {
        requireEntity(tenantId, entityId);
        tags.requireTag(tenantId, tagId);
        memberships.unassignTag(tenantId, entityId, tagId);
    }

    public PageResult<EntityTagAssignment> listTags(UUID tenantId, UUID entityId, int page, int limit) {
        requireEntity(tenantId, entityId);
        int p = PageResult.normalizePage(page);
        int l = PageResult.normalizeLimit(limit);
        long total = memberships.countTagsForEntity(tenantId, entityId);
        return new PageResult<>(
                memberships.listTagsForEntity(tenantId, entityId, l, PageResult.offset(p, l)),
                p, l, total, (long) p * l < total
        );
    }

    private Entity requireEntity(UUID tenantId, UUID entityId) {
        return entities.findByTenantAndId(tenantId, entityId)
                .orElseThrow(() -> new DomainException("ENTITY_NOT_FOUND", "Entity not found"));
    }

    private static BigDecimal requireConfidence(BigDecimal confidence) {
        BigDecimal value = confidence == null ? BigDecimal.ONE : confidence;
        if (value.compareTo(BigDecimal.ZERO) < 0 || value.compareTo(BigDecimal.ONE) > 0) {
            throw new DomainException("VALIDATION_FAILED", "confidence must be between 0 and 1",
                    Map.of("confidence", "range"));
        }
        return value;
    }

    public record IdentifierInput(
            String identifierType,
            String namespace,
            String identifierValue,
            BigDecimal confidence,
            Instant firstSeenAt,
            Instant lastSeenAt
    ) {}
}
