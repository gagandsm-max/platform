package com.platform.core.security.http;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.security.oauth2.core.DelegatingOAuth2TokenValidator;
import org.springframework.security.oauth2.core.OAuth2Error;
import org.springframework.security.oauth2.core.OAuth2TokenValidator;
import org.springframework.security.oauth2.core.OAuth2TokenValidatorResult;
import org.springframework.security.oauth2.jose.jws.SignatureAlgorithm;
import org.springframework.security.oauth2.jwt.Jwt;
import org.springframework.security.oauth2.jwt.JwtDecoder;
import org.springframework.security.oauth2.jwt.JwtException;
import org.springframework.security.oauth2.jwt.JwtValidators;
import org.springframework.security.oauth2.jwt.NimbusJwtDecoder;

import java.util.ArrayList;
import java.util.List;

@Configuration
@EnableConfigurationProperties(OidcResourceServerProperties.class)
public class JwtDecoderConfiguration {

    private static final Logger log = LoggerFactory.getLogger(JwtDecoderConfiguration.class);

    @Bean
    JwtDecoder jwtDecoder(OidcResourceServerProperties properties) {
        if (!properties.isConfigured()) {
            log.warn("event_code=PLT-CORE-SEC-0001 OIDC resource server is not configured; bearer JWTs will be rejected");
            return token -> {
                throw new JwtException("OIDC resource server is not configured");
            };
        }

        NimbusJwtDecoder decoder = NimbusJwtDecoder.withIssuerLocation(properties.getIssuerUri())
                .jwsAlgorithm(SignatureAlgorithm.RS256)
                .build();

        List<OAuth2TokenValidator<Jwt>> validators = new ArrayList<>();
        validators.add(JwtValidators.createDefaultWithIssuer(properties.getIssuerUri()));
        validators.add(audienceValidator(properties.getAudience()));
        decoder.setJwtValidator(new DelegatingOAuth2TokenValidator<>(validators));
        return decoder;
    }

    private static OAuth2TokenValidator<Jwt> audienceValidator(String expectedAudience) {
        return token -> {
            if (token.getAudience() != null && token.getAudience().contains(expectedAudience)) {
                return OAuth2TokenValidatorResult.success();
            }
            OAuth2Error error = new OAuth2Error("invalid_token", "Required audience is missing", null);
            return OAuth2TokenValidatorResult.failure(error);
        };
    }
}
