package com.platform.core.api;

import com.platform.core.api.dto.SiteDtos.CreateSiteRequest;
import com.platform.core.api.dto.SiteDtos.SiteResponse;
import com.platform.core.api.dto.SiteDtos.UpdateSiteRequest;
import com.platform.core.domain.inventory.Site;
import com.platform.core.domain.inventory.SiteService;
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

import java.util.UUID;

@RestController
@RequestMapping("/api/v1/tenants/{tenantId}/sites")
public class SiteController {

    private final SiteService sites;

    public SiteController(SiteService sites) {
        this.sites = sites;
    }

    @PostMapping
    public ResponseEntity<SiteResponse> create(
            @PathVariable UUID tenantId,
            @RequestBody CreateSiteRequest request
    ) {
        Site created = sites.create(
                tenantId,
                request.parentSiteId(),
                request.name(),
                request.description(),
                request.timezone()
        );
        return ResponseEntity.status(HttpStatus.CREATED).body(toResponse(created));
    }

    @GetMapping("/{siteId}")
    public SiteResponse get(@PathVariable UUID tenantId, @PathVariable UUID siteId) {
        return toResponse(sites.get(tenantId, siteId));
    }

    @GetMapping
    public PageResult<SiteResponse> list(
            @PathVariable UUID tenantId,
            @RequestParam(defaultValue = "1") int page,
            @RequestParam(required = false) Integer limit
    ) {
        PageResult<Site> result = sites.list(tenantId, page, PageResult.normalizeLimit(limit));
        return new PageResult<>(
                result.items().stream().map(SiteController::toResponse).toList(),
                result.page(),
                result.limit(),
                result.total(),
                result.hasMore()
        );
    }

    @PutMapping("/{siteId}")
    public SiteResponse update(
            @PathVariable UUID tenantId,
            @PathVariable UUID siteId,
            @RequestBody UpdateSiteRequest request
    ) {
        return toResponse(sites.update(
                tenantId,
                siteId,
                request.getParentSiteId(),
                request.isParentSiteIdPresent(),
                request.getName(),
                request.getDescription(),
                request.getTimezone()
        ));
    }

    private static SiteResponse toResponse(Site site) {
        return new SiteResponse(
                site.siteId(),
                site.tenantId(),
                site.parentSiteId(),
                site.name(),
                site.description(),
                site.timezone(),
                site.createdAt(),
                site.updatedAt()
        );
    }
}
