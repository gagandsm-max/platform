package com.platform.core.api;

import com.platform.core.api.dto.TenantDtos.CreateTenantRequest;
import com.platform.core.api.dto.TenantDtos.TenantResponse;
import com.platform.core.api.dto.TenantDtos.UpdateTenantRequest;
import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.security.authorization.PrincipalContext;
import com.platform.core.security.http.PlatformSecurityContext;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;
import java.util.UUID;

@RestController
@RequestMapping("/api/v1/tenants")
public class TenantController {

    private final TenantService tenants;

    public TenantController(TenantService tenants) {
        this.tenants = tenants;
    }

    @PostMapping
    public ResponseEntity<TenantResponse> create(@RequestBody CreateTenantRequest request) {
        Tenant created = tenants.create(
                request.name(),
                request.mode() == null ? null : TenantMode.from(request.mode()),
                request.status() == null ? null : TenantStatus.from(request.status()),
                request.timezone()
        );
        return ResponseEntity.status(HttpStatus.CREATED).body(toResponse(created));
    }

    @GetMapping("/{tenantId}")
    public TenantResponse get(@PathVariable UUID tenantId) {
        return toResponse(tenants.get(tenantId));
    }

    @GetMapping
    public PageResult<TenantResponse> list(
            @RequestParam(defaultValue = "1") int page,
            @RequestParam(required = false) Integer limit
    ) {
        PrincipalContext principal = PlatformSecurityContext.requirePrincipal();
        PageResult<Tenant> result = tenants.list(page, PageResult.normalizeLimit(limit));
        List<TenantResponse> filtered = result.items().stream()
                .filter(tenant -> principal.tenantScopes().contains(tenant.tenantId()))
                .map(TenantController::toResponse)
                .toList();
        return new PageResult<>(
                filtered,
                result.page(),
                result.limit(),
                filtered.size(),
                false
        );
    }

    @PutMapping("/{tenantId}")
    public TenantResponse update(@PathVariable UUID tenantId, @RequestBody UpdateTenantRequest request) {
        return toResponse(tenants.update(
                tenantId,
                request.name(),
                request.mode() == null ? null : TenantMode.from(request.mode()),
                request.status() == null ? null : TenantStatus.from(request.status()),
                request.timezone()
        ));
    }

    private static TenantResponse toResponse(Tenant tenant) {
        return new TenantResponse(
                tenant.tenantId(),
                tenant.name(),
                tenant.mode().name(),
                tenant.status().name(),
                tenant.timezone(),
                tenant.createdAt(),
                tenant.updatedAt()
        );
    }
}
