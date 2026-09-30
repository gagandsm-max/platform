package com.platform.core.logging;

import ch.qos.logback.classic.spi.ILoggingEvent;
import com.fasterxml.jackson.core.JsonGenerator;
import net.logstash.logback.composite.AbstractJsonProvider;

import java.io.IOException;
import java.util.Map;
import java.util.regex.Pattern;

/**
 * Emits only certified BUILD-005 contextual MDC fields with schema-correct JSON types.
 * Invalid values are omitted (never emitted schema-invalid; never crash the logger).
 */
public final class SupportabilityMdcJsonProvider extends AbstractJsonProvider<ILoggingEvent> {

    private static final long RETRY_COUNT_MAX = 9_007_199_254_740_991L;

    private static final Pattern CORRELATION_ID =
            Pattern.compile("^[A-Za-z0-9._:-]{1,128}$");
    private static final Pattern TENANT_ID =
            Pattern.compile("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$");
    private static final Pattern EVENT_CODE =
            Pattern.compile("^[A-Za-z0-9._:-]{1,64}$");
    private static final Pattern BOUNDED_ID =
            Pattern.compile("^[A-Za-z0-9._:-]{1,128}$");
    private static final Pattern SHORT_TOKEN =
            Pattern.compile("^[A-Za-z0-9._:-]{1,32}$");
    private static final Pattern STRICT_DECIMAL =
            Pattern.compile("^[0-9]+$");

    @Override
    public void writeTo(JsonGenerator generator, ILoggingEvent event) throws IOException {
        Map<String, String> mdc = event.getMDCPropertyMap();
        if (mdc == null || mdc.isEmpty()) {
            return;
        }

        writeStringIfValid(generator, SupportabilityMdcKeys.TENANT_ID, mdc.get(SupportabilityMdcKeys.TENANT_ID), TENANT_ID);
        writeStringIfValid(generator, SupportabilityMdcKeys.EVENT_CODE, mdc.get(SupportabilityMdcKeys.EVENT_CODE), EVENT_CODE);
        writeStringIfValid(generator, SupportabilityMdcKeys.CORRELATION_ID, mdc.get(SupportabilityMdcKeys.CORRELATION_ID), CORRELATION_ID);
        writeStringIfValid(generator, SupportabilityMdcKeys.REQUEST_ID, mdc.get(SupportabilityMdcKeys.REQUEST_ID), BOUNDED_ID);
        writeStringIfValid(generator, SupportabilityMdcKeys.JOB_ID, mdc.get(SupportabilityMdcKeys.JOB_ID), BOUNDED_ID);
        writeStringIfValid(generator, SupportabilityMdcKeys.ENTITY_ID, mdc.get(SupportabilityMdcKeys.ENTITY_ID), BOUNDED_ID);
        writeStringIfValid(generator, SupportabilityMdcKeys.PROTOCOL, mdc.get(SupportabilityMdcKeys.PROTOCOL), SHORT_TOKEN);
        writeStringIfValid(generator, SupportabilityMdcKeys.RESULT, mdc.get(SupportabilityMdcKeys.RESULT), SHORT_TOKEN);
        writeRetryCountIfValid(generator, mdc.get(SupportabilityMdcKeys.RETRY_COUNT));
    }

    private static void writeStringIfValid(
            JsonGenerator generator,
            String field,
            String value,
            Pattern pattern
    ) throws IOException {
        if (value == null || value.isBlank()) {
            return;
        }
        if (!pattern.matcher(value).matches()) {
            return;
        }
        generator.writeStringField(field, value);
    }

    private static void writeRetryCountIfValid(JsonGenerator generator, String raw) throws IOException {
        if (raw == null || raw.isBlank()) {
            return;
        }
        if (!STRICT_DECIMAL.matcher(raw).matches()) {
            return;
        }
        final long parsed;
        try {
            parsed = Long.parseLong(raw);
        } catch (NumberFormatException ignored) {
            return;
        }
        if (parsed < 0L || parsed > RETRY_COUNT_MAX) {
            return;
        }
        generator.writeNumberField(SupportabilityMdcKeys.RETRY_COUNT, parsed);
    }
}
