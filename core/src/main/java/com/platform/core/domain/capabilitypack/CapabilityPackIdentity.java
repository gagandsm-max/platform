package com.platform.core.domain.capabilitypack;

import java.util.Objects;

/** Observed or provisioned identity evidence used for pack matching. */
public record CapabilityPackIdentity(
        String vendor,
        String family,
        String product,
        String model,
        String platform,
        String software,
        String firmware,
        String api,
        String mib
) {
    public CapabilityPackIdentity {
        vendor = emptyToNull(vendor);
        family = emptyToNull(family);
        product = emptyToNull(product);
        model = emptyToNull(model);
        platform = emptyToNull(platform);
        software = emptyToNull(software);
        firmware = emptyToNull(firmware);
        api = emptyToNull(api);
        mib = emptyToNull(mib);
    }

    public String value(MatchField field) {
        Objects.requireNonNull(field, "field");
        return switch (field) {
            case vendor -> vendor;
            case family -> family;
            case product -> product;
            case model -> model;
            case platform -> platform;
            case software -> software;
            case firmware -> firmware;
            case api -> api;
            case mib -> mib;
        };
    }

    private static String emptyToNull(String value) {
        if (value == null) {
            return null;
        }
        String trimmed = value.trim();
        return trimmed.isEmpty() ? null : trimmed;
    }
}
