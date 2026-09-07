package com.platform.core.api;

import com.platform.core.api.dto.TagDtos.CreateTagRequest;
import com.platform.core.api.dto.TagDtos.TagResponse;
import com.platform.core.domain.inventory.Tag;
import com.platform.core.domain.inventory.TagService;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import java.util.UUID;

@RestController
@RequestMapping("/api/v1/tenants/{tenantId}/tags")
public class TagController {

    private final TagService tags;

    public TagController(TagService tags) {
        this.tags = tags;
    }

    @PostMapping
    public ResponseEntity<TagResponse> create(
            @PathVariable UUID tenantId,
            @RequestBody CreateTagRequest request
    ) {
        Tag created = tags.create(tenantId, request.key(), request.value());
        return ResponseEntity.status(HttpStatus.CREATED).body(toResponse(created));
    }

    @GetMapping("/{tagId}")
    public TagResponse get(@PathVariable UUID tenantId, @PathVariable UUID tagId) {
        return toResponse(tags.get(tenantId, tagId));
    }

    @GetMapping
    public PageResult<TagResponse> list(
            @PathVariable UUID tenantId,
            @RequestParam(defaultValue = "1") int page,
            @RequestParam(required = false) Integer limit
    ) {
        PageResult<Tag> result = tags.list(tenantId, page, PageResult.normalizeLimit(limit));
        return new PageResult<>(
                result.items().stream().map(TagController::toResponse).toList(),
                result.page(),
                result.limit(),
                result.total(),
                result.hasMore()
        );
    }

    private static TagResponse toResponse(Tag tag) {
        return new TagResponse(
                tag.tagId(),
                tag.tenantId(),
                tag.tagKey(),
                tag.tagValue(),
                tag.createdAt()
        );
    }
}
