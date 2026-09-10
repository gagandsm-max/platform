package com.platform.core.security.authorization;

import com.platform.core.security.identity.PlatformSubject;

import java.util.Set;

/** Fail-closed default: authenticated identities receive no grants until a directory is configured. */
public final class EmptyPermissionGrantDirectory implements PermissionGrantDirectory {
    @Override
    public PrincipalAuthorization load(PlatformSubject subject) {
        return new PrincipalAuthorization(Set.of(), Set.of());
    }
}
