package com.platform.core.security.authorization;

/**
 * Small Day-03 foundation for resources already authoritative in Day-02, plus mandatory credential USE/REVEAL.
 * This is not the final Platform permission catalog or a role definition.
 */
public final class PlatformPermissions {
    public static final Permission TENANT_READ = Permission.of("tenant.read");
    public static final Permission TENANT_CREATE = Permission.of("tenant.create");
    public static final Permission TENANT_UPDATE = Permission.of("tenant.update");

    public static final Permission SITE_READ = Permission.of("site.read");
    public static final Permission SITE_CREATE = Permission.of("site.create");
    public static final Permission SITE_UPDATE = Permission.of("site.update");

    public static final Permission GROUP_READ = Permission.of("group.read");
    public static final Permission GROUP_CREATE = Permission.of("group.create");
    public static final Permission GROUP_UPDATE = Permission.of("group.update");

    public static final Permission TAG_READ = Permission.of("tag.read");
    public static final Permission TAG_CREATE = Permission.of("tag.create");

    public static final Permission ENTITY_READ = Permission.of("entity.read");
    public static final Permission ENTITY_CREATE = Permission.of("entity.create");
    public static final Permission ENTITY_UPDATE = Permission.of("entity.update");

    public static final Permission ENTITY_IDENTIFIER_READ = Permission.of("entity.identifier.read");
    public static final Permission ENTITY_IDENTIFIER_CREATE = Permission.of("entity.identifier.create");
    public static final Permission ENTITY_IDENTIFIER_REMOVE = Permission.of("entity.identifier.remove");

    public static final Permission ENTITY_GROUP_READ = Permission.of("entity.group.read");
    public static final Permission ENTITY_GROUP_ASSIGN = Permission.of("entity.group.assign");
    public static final Permission ENTITY_GROUP_REMOVE = Permission.of("entity.group.remove");

    public static final Permission ENTITY_TAG_READ = Permission.of("entity.tag.read");
    public static final Permission ENTITY_TAG_ASSIGN = Permission.of("entity.tag.assign");
    public static final Permission ENTITY_TAG_REMOVE = Permission.of("entity.tag.remove");

    public static final Permission CREDENTIAL_USE = Permission.of("credential.use");
    public static final Permission CREDENTIAL_REVEAL = Permission.of("credential.reveal");

    private PlatformPermissions() {
    }
}
