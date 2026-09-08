package com.platform.core.security.http;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.core.annotation.Order;
import org.springframework.http.HttpMethod;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.config.annotation.web.configuration.EnableWebSecurity;
import org.springframework.security.config.annotation.web.configurers.AbstractHttpConfigurer;
import org.springframework.security.config.http.SessionCreationPolicy;
import org.springframework.security.oauth2.server.resource.web.authentication.BearerTokenAuthenticationFilter;
import org.springframework.security.web.SecurityFilterChain;
import org.springframework.security.web.util.matcher.AntPathRequestMatcher;
import org.springframework.security.web.util.matcher.OrRequestMatcher;
import org.springframework.security.web.util.matcher.RequestMatcher;
import org.springframework.boot.web.servlet.FilterRegistrationBean;

/**
 * Explicit route classification:
 * <ul>
 *   <li>Sensor transport — not bearer-JWT; controller/Tomcat mTLS remains authoritative.</li>
 *   <li>Health/version — unauthenticated probes.</li>
 *   <li>Business APIs — authenticated OIDC bearer JWT + Platform authorization.</li>
 * </ul>
 * Stateless bearer API: CSRF disabled deliberately (no cookie session login).
 */
@Configuration
@EnableWebSecurity
public class HttpSecurityConfiguration {

    @Bean
    @Order(1)
    SecurityFilterChain sensorTransportSecurityFilterChain(HttpSecurity http) throws Exception {
        http.securityMatcher(sensorMatchers())
                .csrf(AbstractHttpConfigurer::disable)
                .cors(AbstractHttpConfigurer::disable)
                .sessionManagement(session -> session.sessionCreationPolicy(SessionCreationPolicy.STATELESS))
                .authorizeHttpRequests(auth -> auth.anyRequest().permitAll())
                .httpBasic(AbstractHttpConfigurer::disable)
                .formLogin(AbstractHttpConfigurer::disable)
                .logout(AbstractHttpConfigurer::disable);
        return http.build();
    }

    @Bean
    @Order(2)
    SecurityFilterChain probeSecurityFilterChain(HttpSecurity http) throws Exception {
        http.securityMatcher(probeMatchers())
                .csrf(AbstractHttpConfigurer::disable)
                .cors(AbstractHttpConfigurer::disable)
                .sessionManagement(session -> session.sessionCreationPolicy(SessionCreationPolicy.STATELESS))
                .authorizeHttpRequests(auth -> auth.anyRequest().permitAll())
                .httpBasic(AbstractHttpConfigurer::disable)
                .formLogin(AbstractHttpConfigurer::disable)
                .logout(AbstractHttpConfigurer::disable);
        return http.build();
    }

    @Bean
    @Order(3)
    SecurityFilterChain apiSecurityFilterChain(
            HttpSecurity http,
            PlatformJwtAuthenticationConverter jwtAuthenticationConverter,
            SecurityErrorResponseWriter errorWriter,
            SecurityAuditEmitter auditEmitter,
            AuthorizationEnforcementFilter authorizationEnforcementFilter
    ) throws Exception {
        http.securityMatcher("/api/**")
                .csrf(AbstractHttpConfigurer::disable)
                .cors(AbstractHttpConfigurer::disable)
                .sessionManagement(session -> session.sessionCreationPolicy(SessionCreationPolicy.STATELESS))
                .authorizeHttpRequests(auth -> auth.anyRequest().authenticated())
                .oauth2ResourceServer(oauth2 -> oauth2
                        .jwt(jwt -> jwt.jwtAuthenticationConverter(jwtAuthenticationConverter))
                        .authenticationEntryPoint((request, response, ex) -> {
                            auditEmitter.authenticationFailure("AUTH_TOKEN_INVALID");
                            errorWriter.writeUnauthorized(request, response, "AUTH_TOKEN_INVALID");
                        })
                        .accessDeniedHandler((request, response, ex) -> errorWriter.writeNotFound(response))
                )
                .exceptionHandling(ex -> ex
                        .authenticationEntryPoint((request, response, authEx) -> {
                            auditEmitter.authenticationFailure("AUTH_REQUIRED");
                            errorWriter.writeUnauthorized(request, response, "AUTH_REQUIRED");
                        })
                        .accessDeniedHandler((request, response, accessEx) -> errorWriter.writeNotFound(response))
                )
                .httpBasic(AbstractHttpConfigurer::disable)
                .formLogin(AbstractHttpConfigurer::disable)
                .logout(AbstractHttpConfigurer::disable)
                .addFilterAfter(authorizationEnforcementFilter, BearerTokenAuthenticationFilter.class);
        return http.build();
    }

    /**
     * Keep the authorization filter out of the generic servlet chain; it belongs only in the API security chain.
     */
    @Bean
    FilterRegistrationBean<AuthorizationEnforcementFilter> disableAuthorizationFilterRegistration(
            AuthorizationEnforcementFilter filter
    ) {
        FilterRegistrationBean<AuthorizationEnforcementFilter> registration = new FilterRegistrationBean<>(filter);
        registration.setEnabled(false);
        return registration;
    }

    @Bean
    @Order(4)
    SecurityFilterChain defaultDenySecurityFilterChain(HttpSecurity http) throws Exception {
        http.securityMatcher("/**")
                .csrf(AbstractHttpConfigurer::disable)
                .cors(AbstractHttpConfigurer::disable)
                .sessionManagement(session -> session.sessionCreationPolicy(SessionCreationPolicy.STATELESS))
                .authorizeHttpRequests(auth -> auth
                        .requestMatchers(HttpMethod.OPTIONS, "/**").denyAll()
                        .anyRequest().denyAll()
                )
                .httpBasic(AbstractHttpConfigurer::disable)
                .formLogin(AbstractHttpConfigurer::disable)
                .logout(AbstractHttpConfigurer::disable);
        return http.build();
    }

    private static RequestMatcher sensorMatchers() {
        return new OrRequestMatcher(
                new AntPathRequestMatcher("/api/v1/sensor/**"),
                new AntPathRequestMatcher("/api/v1/telemetry")
        );
    }

    /**
     * Exact unauthenticated probe surface only. Do not widen with {@code /actuator/**}:
     * newly enabled actuator endpoints must stay deny-by-default until deliberately classified.
     */
    private static RequestMatcher probeMatchers() {
        return new OrRequestMatcher(
                new AntPathRequestMatcher("/actuator/health"),
                new AntPathRequestMatcher("/actuator/health/liveness"),
                new AntPathRequestMatcher("/actuator/health/readiness"),
                new AntPathRequestMatcher("/api/v1/version"),
                new AntPathRequestMatcher("/api/v1/health/**")
        );
    }
}
