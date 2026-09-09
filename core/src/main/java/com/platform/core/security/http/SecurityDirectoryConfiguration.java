package com.platform.core.security.http;

import com.platform.core.security.authorization.EmptyPermissionGrantDirectory;
import com.platform.core.security.authorization.PermissionGrantDirectory;
import com.platform.core.security.identity.DirectoryIdentityResolver;
import com.platform.core.security.identity.EmptyIdentityDirectory;
import com.platform.core.security.identity.IdentityDirectory;
import com.platform.core.security.identity.IdentityResolver;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class SecurityDirectoryConfiguration {

    @Bean
    @ConditionalOnMissingBean(IdentityDirectory.class)
    IdentityDirectory identityDirectory() {
        return new EmptyIdentityDirectory();
    }

    @Bean
    @ConditionalOnMissingBean(PermissionGrantDirectory.class)
    PermissionGrantDirectory permissionGrantDirectory() {
        return new EmptyPermissionGrantDirectory();
    }

    @Bean
    @ConditionalOnMissingBean(IdentityResolver.class)
    IdentityResolver identityResolver(IdentityDirectory identityDirectory) {
        return new DirectoryIdentityResolver(identityDirectory);
    }
}
