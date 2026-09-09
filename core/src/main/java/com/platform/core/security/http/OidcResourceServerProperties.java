package com.platform.core.security.http;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Typed OIDC resource-server boundary. Does not embed secrets or default localhost issuers.
 * Blank issuer/audience keeps JwtDecoder in reject-all mode (fail closed for bearer APIs).
 */
@ConfigurationProperties(prefix = "platform.security.oidc")
public class OidcResourceServerProperties {

    /**
     * Expected token issuer (IdP realm URL). Empty means OIDC is not configured.
     */
    private String issuerUri = "";

    /**
     * Expected audience / API resource indicator. Empty means audience cannot be validated.
     */
    private String audience = "";

    /**
     * Stable Platform provider-instance id for identity mapping keys.
     */
    private String providerInstanceId = "platform-oidc";

    public String getIssuerUri() {
        return issuerUri;
    }

    public void setIssuerUri(String issuerUri) {
        this.issuerUri = issuerUri == null ? "" : issuerUri.trim();
    }

    public String getAudience() {
        return audience;
    }

    public void setAudience(String audience) {
        this.audience = audience == null ? "" : audience.trim();
    }

    public String getProviderInstanceId() {
        return providerInstanceId;
    }

    public void setProviderInstanceId(String providerInstanceId) {
        this.providerInstanceId = providerInstanceId == null ? "" : providerInstanceId.trim();
    }

    public boolean isConfigured() {
        return !issuerUri.isBlank() && !audience.isBlank() && !providerInstanceId.isBlank();
    }
}
