package com.platform.core.query;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.platform.core.query.LiveKeysetCursorCodec.CursorContext;
import com.platform.core.query.LiveKeysetCursorCodec.CursorException;
import com.platform.core.query.LiveKeysetCursorCodec.DecodedCursor;
import com.platform.core.query.LiveKeysetCursorCodec.ErrorCode;
import com.platform.core.query.LiveKeysetCursorCodec.NullsOrder;
import com.platform.core.query.LiveKeysetCursorCodec.SortDirection;
import com.platform.core.query.LiveKeysetCursorCodec.SortField;
import com.platform.core.query.LiveKeysetCursorCodec.SortValue;
import com.platform.core.query.LiveKeysetCursorCodec.SortValueType;
import java.io.ByteArrayOutputStream;
import java.io.Serializable;
import java.math.BigDecimal;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Base64;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import java.util.UUID;
import javax.crypto.Cipher;
import javax.crypto.spec.GCMParameterSpec;
import javax.crypto.spec.SecretKeySpec;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

class LiveKeysetCursorCodecTest {

    private static final byte[] PRIMARY_KEY = deterministicKey(0x10);
    private static final byte[] OTHER_KEY = deterministicKey(0x50);
    private static final Instant FIXED_NOW = Instant.parse("2026-11-25T10:00:00.123456789Z");
    private static final Duration STANDARD_TTL = Duration.ofMinutes(10);

    private static final CursorContext BASE_CONTEXT = new CursorContext(
            "inventory.devices.list",
            "tenant-test-alpha",
            "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
            "sha256:fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
            List.of(
                    SortField.of("updated_at", SortDirection.DESC, NullsOrder.LAST),
                    SortField.of("hostname", SortDirection.ASC, NullsOrder.FIRST)));

    private static final List<SortValue> BASE_TUPLE = List.of(
            SortValue.ofInstant(Instant.parse("2026-11-25T09:59:12.987654321Z")),
            SortValue.ofString("edge-router-01"));

    private static final String BASE_RESOURCE_ID = "res-00000000-0000-4000-8000-000000000042";

    @Test
    @DisplayName("Round trip preserves all CC-CURSOR bound fields and immutability without plaintext leakage")
    void roundTripPreservesAllBoundFields() {
        LiveKeysetCursorCodec codec = new LiveKeysetCursorCodec();
        String token = codec.encode(
                PRIMARY_KEY,
                BASE_CONTEXT,
                BASE_TUPLE,
                BASE_RESOURCE_ID,
                FIXED_NOW,
                STANDARD_TTL);

        assertFalse(token.isBlank());
        assertTrue(token.length() <= codec.maxTokenLength());
        assertFalse(token.contains("="));
        assertFalse(token.contains("tenant-test-alpha"));
        assertFalse(token.contains("inventory.devices.list"));
        assertFalse(token.contains(BASE_RESOURCE_ID));

        DecodedCursor decoded = codec.decode(
                PRIMARY_KEY,
                token,
                BASE_CONTEXT,
                FIXED_NOW.plusSeconds(30));

        assertEquals("1.0", decoded.schemaVersion());
        assertEquals("LIVE_KEYSET", decoded.cursorMode());
        assertEquals(BASE_CONTEXT.queryOperationId(), decoded.queryOperationId());
        assertEquals(BASE_CONTEXT.tenantId(), decoded.tenantId());
        assertEquals(BASE_CONTEXT.normalizedFilterFingerprint(), decoded.normalizedFilterFingerprint());
        assertEquals(BASE_CONTEXT.authorizedResourceScopeFingerprint(), decoded.authorizedResourceScopeFingerprint());
        assertEquals(BASE_CONTEXT.sortDefinition(), decoded.sortDefinition());
        assertEquals(BASE_CONTEXT, decoded.context());
        assertEquals(BASE_TUPLE, decoded.lastSortTuple());
        assertEquals(BASE_RESOURCE_ID, decoded.immutableUniqueResourceId());
        assertEquals(FIXED_NOW, decoded.issuedAt());
        assertEquals(FIXED_NOW.plus(STANDARD_TTL), decoded.expiresAt());

        assertThrows(UnsupportedOperationException.class, () -> decoded.sortDefinition().add(SortField.asc("x")));
        assertThrows(UnsupportedOperationException.class, () -> decoded.lastSortTuple().add(SortValue.ofNull()));
    }

    @Test
    @DisplayName("Round trip preserves all explicitly supported scalar types and NULL without coercion")
    void preservesTypedAndNullSortValuesWithoutCoercion() {
        LiveKeysetCursorCodec codec = new LiveKeysetCursorCodec();
        UUID sampleUuid = UUID.fromString("123e4567-e89b-12d3-a456-426614174000");
        BigDecimal scaledDecimal = new BigDecimal("12345.6700");
        Instant preciseInstant = Instant.parse("2026-11-25T08:15:30.000000999Z");

        CursorContext context = new CursorContext(
                "telemetry.records.query",
                "tenant-test-alpha",
                "sha256:aaaa",
                "sha256:bbbb",
                List.of(
                        SortField.of("f_null", SortDirection.ASC, NullsOrder.FIRST),
                        SortField.of("f_empty_str", SortDirection.ASC, NullsOrder.LAST),
                        SortField.of("f_num_str", SortDirection.ASC, NullsOrder.LAST),
                        SortField.of("f_unicode_str", SortDirection.DESC, NullsOrder.LAST),
                        SortField.of("f_int64_min", SortDirection.ASC, NullsOrder.LAST),
                        SortField.of("f_int64_max", SortDirection.DESC, NullsOrder.LAST),
                        SortField.of("f_float64", SortDirection.ASC, NullsOrder.LAST),
                        SortField.of("f_float64_neg_zero", SortDirection.ASC, NullsOrder.LAST),
                        SortField.of("f_decimal", SortDirection.DESC, NullsOrder.LAST),
                        SortField.of("f_bool_true", SortDirection.ASC, NullsOrder.LAST),
                        SortField.of("f_bool_false", SortDirection.DESC, NullsOrder.LAST),
                        SortField.of("f_instant", SortDirection.DESC, NullsOrder.LAST),
                        SortField.of("f_uuid", SortDirection.ASC, NullsOrder.LAST)));

        List<SortValue> tuple = List.of(
                SortValue.ofNull(),
                SortValue.ofString(""),
                SortValue.ofString("42"),
                SortValue.ofString("site-\u00e9-\u6771\u4eac"),
                SortValue.ofInt64(Long.MIN_VALUE),
                SortValue.ofInt64(Long.MAX_VALUE),
                SortValue.ofFloat64(-123.456789d),
                SortValue.ofFloat64(-0.0d),
                SortValue.ofDecimal(scaledDecimal),
                SortValue.ofBoolean(true),
                SortValue.ofBoolean(false),
                SortValue.ofInstant(preciseInstant),
                SortValue.ofUuid(sampleUuid));

        String token = codec.encode(PRIMARY_KEY, context, tuple, BASE_RESOURCE_ID, FIXED_NOW, STANDARD_TTL);
        DecodedCursor decoded = codec.decode(PRIMARY_KEY, token, context, FIXED_NOW);

        assertEquals(tuple, decoded.lastSortTuple());
        assertEquals(SortValueType.NULL, decoded.lastSortTuple().get(0).type());
        assertEquals(SortValueType.STRING, decoded.lastSortTuple().get(1).type());
        assertNotEquals(decoded.lastSortTuple().get(0), decoded.lastSortTuple().get(1));
        assertNotEquals(SortValue.ofString("42"), SortValue.ofInt64(42L));
        assertNotEquals(SortValue.ofInt64(1L), SortValue.ofBoolean(true));
        assertNotEquals(SortValue.ofFloat64(42.0d), SortValue.ofInt64(42L));

        SortValue.DecimalValue decOut = assertInstanceOf(
                SortValue.DecimalValue.class,
                decoded.lastSortTuple().get(8));
        assertEquals(4, decOut.value().scale());
        assertEquals(scaledDecimal, decOut.value());
        assertNotEquals(new BigDecimal("12345.67"), decOut.value());

        SortValue.Float64Value negZeroOut = assertInstanceOf(
                SortValue.Float64Value.class,
                decoded.lastSortTuple().get(7));
        assertEquals(Double.doubleToLongBits(-0.0d), Double.doubleToLongBits(negZeroOut.value()));

        assertThrows(IllegalArgumentException.class, () -> SortValue.ofFloat64(Double.NaN));
        assertThrows(IllegalArgumentException.class, () -> SortValue.ofFloat64(Double.POSITIVE_INFINITY));
        assertThrows(IllegalArgumentException.class, () -> SortValue.ofFloat64(Double.NEGATIVE_INFINITY));
        assertThrows(IllegalArgumentException.class, () -> SortValue.ofString(null));
        assertThrows(IllegalArgumentException.class, () -> SortValue.ofDecimal(null));
        assertThrows(IllegalArgumentException.class, () -> SortValue.ofInstant(null));
        assertThrows(IllegalArgumentException.class, () -> SortValue.ofUuid(null));
    }

    @Test
    @DisplayName("Wrong key fails closed with CURSOR_INVALID and invalid key sizes fail fast")
    void wrongKeyRejected() {
        LiveKeysetCursorCodec codec = new LiveKeysetCursorCodec();
        String token = codec.encode(PRIMARY_KEY, BASE_CONTEXT, BASE_TUPLE, BASE_RESOURCE_ID, FIXED_NOW, STANDARD_TTL);

        CursorException otherKeyEx = assertThrows(
                CursorException.class,
                () -> codec.decode(OTHER_KEY, token, BASE_CONTEXT, FIXED_NOW));
        assertEquals(ErrorCode.CURSOR_INVALID, otherKeyEx.getErrorCode());

        byte[] oneBitFlippedKey = PRIMARY_KEY.clone();
        oneBitFlippedKey[31] ^= 0x01;
        CursorException bitFlipEx = assertThrows(
                CursorException.class,
                () -> codec.decode(oneBitFlippedKey, token, BASE_CONTEXT, FIXED_NOW));
        assertEquals(ErrorCode.CURSOR_INVALID, bitFlipEx.getErrorCode());

        assertThrows(
                IllegalArgumentException.class,
                () -> codec.encode(new byte[16], BASE_CONTEXT, BASE_TUPLE, BASE_RESOURCE_ID, FIXED_NOW, STANDARD_TTL));
        assertThrows(
                IllegalArgumentException.class,
                () -> codec.decode(new byte[31], token, BASE_CONTEXT, FIXED_NOW));
        assertThrows(
                IllegalArgumentException.class,
                () -> codec.decode(null, token, BASE_CONTEXT, FIXED_NOW));
    }

    @Test
    @DisplayName("Tampering with header, nonce, ciphertext, authentication tag, or AAD fails closed with CURSOR_INVALID")
    void tamperingHeaderNonceCiphertextTagAndAadRejected() throws Exception {
        LiveKeysetCursorCodec codec = new LiveKeysetCursorCodec();
        String token = codec.encode(PRIMARY_KEY, BASE_CONTEXT, BASE_TUPLE, BASE_RESOURCE_ID, FIXED_NOW, STANDARD_TTL);
        byte[] raw = Base64.getUrlDecoder().decode(token);

        // Tamper every byte of the 6-byte wire header (magic, format version, mode)
        for (int i = 0; i < LiveKeysetCursorCodec.HEADER_LENGTH_BYTES; i++) {
            byte[] mutated = raw.clone();
            mutated[i] ^= 0x01;
            String tampered = Base64.getUrlEncoder().withoutPadding().encodeToString(mutated);
            CursorException ex = assertThrows(
                    CursorException.class,
                    () -> codec.decode(PRIMARY_KEY, tampered, BASE_CONTEXT, FIXED_NOW));
            assertEquals(ErrorCode.CURSOR_INVALID, ex.getErrorCode());
        }

        // Tamper nonce
        byte[] mutatedNonce = raw.clone();
        mutatedNonce[LiveKeysetCursorCodec.HEADER_LENGTH_BYTES] ^= 0x80;
        String tamperedNonce = Base64.getUrlEncoder().withoutPadding().encodeToString(mutatedNonce);
        assertEquals(
                ErrorCode.CURSOR_INVALID,
                assertThrows(
                        CursorException.class,
                        () -> codec.decode(PRIMARY_KEY, tamperedNonce, BASE_CONTEXT, FIXED_NOW)).getErrorCode());

        // Tamper ciphertext byte
        byte[] mutatedCiphertext = raw.clone();
        int ciphertextIndex = LiveKeysetCursorCodec.HEADER_LENGTH_BYTES + LiveKeysetCursorCodec.GCM_NONCE_LENGTH_BYTES;
        mutatedCiphertext[ciphertextIndex] ^= 0x04;
        String tamperedCiphertext = Base64.getUrlEncoder().withoutPadding().encodeToString(mutatedCiphertext);
        assertEquals(
                ErrorCode.CURSOR_INVALID,
                assertThrows(
                        CursorException.class,
                        () -> codec.decode(PRIMARY_KEY, tamperedCiphertext, BASE_CONTEXT, FIXED_NOW)).getErrorCode());

        // Tamper authentication tag (first and last byte of 16-byte tag)
        for (int offset : new int[] {raw.length - LiveKeysetCursorCodec.GCM_TAG_LENGTH_BYTES, raw.length - 1}) {
            byte[] mutatedTag = raw.clone();
            mutatedTag[offset] ^= 0x02;
            String tamperedTag = Base64.getUrlEncoder().withoutPadding().encodeToString(mutatedTag);
            assertEquals(
                    ErrorCode.CURSOR_INVALID,
                    assertThrows(
                            CursorException.class,
                            () -> codec.decode(PRIMARY_KEY, tamperedTag, BASE_CONTEXT, FIXED_NOW)).getErrorCode());
        }

        // Truncated tag / appended byte
        String truncated = Base64.getUrlEncoder().withoutPadding().encodeToString(Arrays.copyOf(raw, raw.length - 1));
        assertEquals(
                ErrorCode.CURSOR_INVALID,
                assertThrows(
                        CursorException.class,
                        () -> codec.decode(PRIMARY_KEY, truncated, BASE_CONTEXT, FIXED_NOW)).getErrorCode());

        byte[] extended = Arrays.copyOf(raw, raw.length + 1);
        extended[extended.length - 1] = 0x55;
        String extendedToken = Base64.getUrlEncoder().withoutPadding().encodeToString(extended);
        assertEquals(
                ErrorCode.CURSOR_INVALID,
                assertThrows(
                        CursorException.class,
                        () -> codec.decode(PRIMARY_KEY, extendedToken, BASE_CONTEXT, FIXED_NOW)).getErrorCode());

        // Valid plaintext encrypted with same key and valid wire header, but mismatched AAD
        byte[] validPlaintext = buildCanonicalPlaintext("1.0", "LIVE_KEYSET", FIXED_NOW, FIXED_NOW.plus(STANDARD_TTL));
        byte[] wrongAad = "platform.core.query.cc-cursor.wrong-purpose.v1".getBytes(StandardCharsets.UTF_8);
        String wrongAadToken = encryptSyntheticToken(
                PRIMARY_KEY,
                LiveKeysetCursorCodec.fixedHeaderCopy(),
                wrongAad,
                deterministicNonce(0x01),
                validPlaintext);
        assertEquals(
                ErrorCode.CURSOR_INVALID,
                assertThrows(
                        CursorException.class,
                        () -> codec.decode(PRIMARY_KEY, wrongAadToken, BASE_CONTEXT, FIXED_NOW)).getErrorCode());
    }

    @Test
    @DisplayName("Cross tenant, scope, filter, sort, and operation mismatches fail closed with CURSOR_SCOPE_CHANGED")
    void crossContextMismatchesFailClosedWithScopeChanged() {
        LiveKeysetCursorCodec codec = new LiveKeysetCursorCodec();
        String token = codec.encode(PRIMARY_KEY, BASE_CONTEXT, BASE_TUPLE, BASE_RESOURCE_ID, FIXED_NOW, STANDARD_TTL);

        List<CursorContext> mismatchedContexts = List.of(
                // Cross-tenant
                new CursorContext(
                        BASE_CONTEXT.queryOperationId(),
                        "tenant-test-beta",
                        BASE_CONTEXT.normalizedFilterFingerprint(),
                        BASE_CONTEXT.authorizedResourceScopeFingerprint(),
                        BASE_CONTEXT.sortDefinition()),
                // Cross-scope
                new CursorContext(
                        BASE_CONTEXT.queryOperationId(),
                        BASE_CONTEXT.tenantId(),
                        BASE_CONTEXT.normalizedFilterFingerprint(),
                        "sha256:9999999999999999999999999999999999999999999999999999999999999999",
                        BASE_CONTEXT.sortDefinition()),
                // Cross-filter
                new CursorContext(
                        BASE_CONTEXT.queryOperationId(),
                        BASE_CONTEXT.tenantId(),
                        "sha256:1111111111111111111111111111111111111111111111111111111111111111",
                        BASE_CONTEXT.authorizedResourceScopeFingerprint(),
                        BASE_CONTEXT.sortDefinition()),
                // Cross-operation
                new CursorContext(
                        "inventory.interfaces.list",
                        BASE_CONTEXT.tenantId(),
                        BASE_CONTEXT.normalizedFilterFingerprint(),
                        BASE_CONTEXT.authorizedResourceScopeFingerprint(),
                        BASE_CONTEXT.sortDefinition()),
                // Different sort direction
                new CursorContext(
                        BASE_CONTEXT.queryOperationId(),
                        BASE_CONTEXT.tenantId(),
                        BASE_CONTEXT.normalizedFilterFingerprint(),
                        BASE_CONTEXT.authorizedResourceScopeFingerprint(),
                        List.of(
                                SortField.of("updated_at", SortDirection.ASC, NullsOrder.LAST),
                                SortField.of("hostname", SortDirection.ASC, NullsOrder.FIRST))),
                // Different nulls order
                new CursorContext(
                        BASE_CONTEXT.queryOperationId(),
                        BASE_CONTEXT.tenantId(),
                        BASE_CONTEXT.normalizedFilterFingerprint(),
                        BASE_CONTEXT.authorizedResourceScopeFingerprint(),
                        List.of(
                                SortField.of("updated_at", SortDirection.DESC, NullsOrder.FIRST),
                                SortField.of("hostname", SortDirection.ASC, NullsOrder.FIRST))),
                // Different sort field order
                new CursorContext(
                        BASE_CONTEXT.queryOperationId(),
                        BASE_CONTEXT.tenantId(),
                        BASE_CONTEXT.normalizedFilterFingerprint(),
                        BASE_CONTEXT.authorizedResourceScopeFingerprint(),
                        List.of(
                                SortField.of("hostname", SortDirection.ASC, NullsOrder.FIRST),
                                SortField.of("updated_at", SortDirection.DESC, NullsOrder.LAST))),
                // Different sort field count
                new CursorContext(
                        BASE_CONTEXT.queryOperationId(),
                        BASE_CONTEXT.tenantId(),
                        BASE_CONTEXT.normalizedFilterFingerprint(),
                        BASE_CONTEXT.authorizedResourceScopeFingerprint(),
                        List.of(SortField.of("updated_at", SortDirection.DESC, NullsOrder.LAST))));

        for (CursorContext wrongContext : mismatchedContexts) {
            CursorException ex = assertThrows(
                    CursorException.class,
                    () -> codec.decode(PRIMARY_KEY, token, wrongContext, FIXED_NOW.plusSeconds(1)));
            assertEquals(ErrorCode.CURSOR_SCOPE_CHANGED, ex.getErrorCode());
            assertFalse(ex.getMessage().contains("tenant-test-alpha"));
            assertFalse(ex.getMessage().contains(BASE_CONTEXT.authorizedResourceScopeFingerprint()));
            assertFalse(ex.getMessage().contains(BASE_CONTEXT.normalizedFilterFingerprint()));
        }

        // Cross-tenant expired cursor must still fail with CURSOR_SCOPE_CHANGED (no expiry oracle across tenants)
        CursorContext otherTenant = new CursorContext(
                BASE_CONTEXT.queryOperationId(),
                "tenant-test-beta",
                BASE_CONTEXT.normalizedFilterFingerprint(),
                BASE_CONTEXT.authorizedResourceScopeFingerprint(),
                BASE_CONTEXT.sortDefinition());
        CursorException crossTenantExpiredEx = assertThrows(
                CursorException.class,
                () -> codec.decode(PRIMARY_KEY, token, otherTenant, FIXED_NOW.plus(STANDARD_TTL).plusSeconds(60)));
        assertEquals(ErrorCode.CURSOR_SCOPE_CHANGED, crossTenantExpiredEx.getErrorCode());
    }

    @Test
    @DisplayName("Expiry boundary and future issued_at are strictly enforced against externally supplied time")
    void expiryBoundaryAndFutureIssuedEnforced() {
        LiveKeysetCursorCodec codec = new LiveKeysetCursorCodec(Duration.ofMinutes(15), 4096);
        String token = codec.encode(PRIMARY_KEY, BASE_CONTEXT, BASE_TUPLE, BASE_RESOURCE_ID, FIXED_NOW, STANDARD_TTL);
        Instant expiresAt = FIXED_NOW.plus(STANDARD_TTL);

        // Exact issued_at is valid
        DecodedCursor atIssued = codec.decode(PRIMARY_KEY, token, BASE_CONTEXT, FIXED_NOW);
        assertEquals(FIXED_NOW, atIssued.issuedAt());

        // 1 nanosecond before expires_at is valid
        DecodedCursor justBeforeExpiry = codec.decode(PRIMARY_KEY, token, BASE_CONTEXT, expiresAt.minusNanos(1));
        assertEquals(expiresAt, justBeforeExpiry.expiresAt());

        // Exact expires_at is expired
        CursorException atExpiryEx = assertThrows(
                CursorException.class,
                () -> codec.decode(PRIMARY_KEY, token, BASE_CONTEXT, expiresAt));
        assertEquals(ErrorCode.CURSOR_EXPIRED, atExpiryEx.getErrorCode());

        // After expires_at is expired
        CursorException afterExpiryEx = assertThrows(
                CursorException.class,
                () -> codec.decode(PRIMARY_KEY, token, BASE_CONTEXT, expiresAt.plusNanos(1)));
        assertEquals(ErrorCode.CURSOR_EXPIRED, afterExpiryEx.getErrorCode());

        // Future issued_at (1 nanosecond before issued_at) is CURSOR_INVALID
        CursorException futureIssuedEx = assertThrows(
                CursorException.class,
                () -> codec.decode(PRIMARY_KEY, token, BASE_CONTEXT, FIXED_NOW.minusNanos(1)));
        assertEquals(ErrorCode.CURSOR_INVALID, futureIssuedEx.getErrorCode());

        // Invalid TTL bounds on encode
        assertThrows(
                IllegalArgumentException.class,
                () -> codec.encode(PRIMARY_KEY, BASE_CONTEXT, BASE_TUPLE, BASE_RESOURCE_ID, FIXED_NOW, Duration.ZERO));
        assertThrows(
                IllegalArgumentException.class,
                () -> codec.encode(
                        PRIMARY_KEY,
                        BASE_CONTEXT,
                        BASE_TUPLE,
                        BASE_RESOURCE_ID,
                        FIXED_NOW,
                        Duration.ofSeconds(-1)));
        assertThrows(
                IllegalArgumentException.class,
                () -> codec.encode(
                        PRIMARY_KEY,
                        BASE_CONTEXT,
                        BASE_TUPLE,
                        BASE_RESOURCE_ID,
                        FIXED_NOW,
                        Duration.ofMinutes(15).plusNanos(1)));
    }

    @Test
    @DisplayName("Malformed, non-canonical, unsupported-mode, and oversized inputs fail closed")
    void malformedUnsupportedAndOversizedInputsRejected() throws Exception {
        LiveKeysetCursorCodec codec = new LiveKeysetCursorCodec(Duration.ofMinutes(15), 512);

        List<String> malformedTokens = Arrays.asList(
                null,
                "",
                "   ",
                "A",
                "AAAA",
                "not+standard/base64==",
                "invalid\nnewline",
                "a".repeat(513),
                "a".repeat(20_000));
        for (String badToken : malformedTokens) {
            CursorException ex = assertThrows(
                    CursorException.class,
                    () -> codec.decode(PRIMARY_KEY, badToken, BASE_CONTEXT, FIXED_NOW));
            assertEquals(ErrorCode.CURSOR_INVALID, ex.getErrorCode());
        }

        // Non-canonical trailing Base64URL bits (valid token with last char altered in unused bit)
        String validToken = codec.encode(
                PRIMARY_KEY,
                BASE_CONTEXT,
                BASE_TUPLE,
                BASE_RESOURCE_ID,
                FIXED_NOW,
                STANDARD_TTL);
        String paddedToken = validToken + "=";
        assertEquals(
                ErrorCode.CURSOR_INVALID,
                assertThrows(
                        CursorException.class,
                        () -> codec.decode(PRIMARY_KEY, paddedToken, BASE_CONTEXT, FIXED_NOW)).getErrorCode());

        // Oversized encode payload exceeding configured maxTokenLength
        LiveKeysetCursorCodec tightCodec = new LiveKeysetCursorCodec(Duration.ofMinutes(15), 256);
        CursorContext singleStringSort = new CursorContext(
                "inventory.devices.list",
                "tenant-test-alpha",
                "sha256:0123",
                "sha256:4567",
                List.of(SortField.asc("hostname")));
        assertThrows(
                IllegalArgumentException.class,
                () -> tightCodec.encode(
                        PRIMARY_KEY,
                        singleStringSort,
                        List.of(SortValue.ofString("x".repeat(300))),
                        BASE_RESOURCE_ID,
                        FIXED_NOW,
                        STANDARD_TTL));

        // Unsupported cursorMode inside authenticated plaintext (e.g. SNAPSHOT)
        byte[] snapshotPlaintext = buildCanonicalPlaintext("1.0", "SNAPSHOT", FIXED_NOW, FIXED_NOW.plus(STANDARD_TTL));
        String snapshotToken = encryptValidHeaderAndAad(snapshotPlaintext, deterministicNonce(0x02));
        assertEquals(
                ErrorCode.CURSOR_INVALID,
                assertThrows(
                        CursorException.class,
                        () -> codec.decode(PRIMARY_KEY, snapshotToken, BASE_CONTEXT, FIXED_NOW)).getErrorCode());

        // Unsupported schemaVersion inside authenticated plaintext
        byte[] v2Plaintext = buildCanonicalPlaintext("2.0", "LIVE_KEYSET", FIXED_NOW, FIXED_NOW.plus(STANDARD_TTL));
        String v2Token = encryptValidHeaderAndAad(v2Plaintext, deterministicNonce(0x03));
        assertEquals(
                ErrorCode.CURSOR_INVALID,
                assertThrows(
                        CursorException.class,
                        () -> codec.decode(PRIMARY_KEY, v2Token, BASE_CONTEXT, FIXED_NOW)).getErrorCode());

        // Trailing bytes inside authenticated plaintext
        byte[] basePlaintext = buildCanonicalPlaintext("1.0", "LIVE_KEYSET", FIXED_NOW, FIXED_NOW.plus(STANDARD_TTL));
        byte[] withTrailing = Arrays.copyOf(basePlaintext, basePlaintext.length + 1);
        withTrailing[withTrailing.length - 1] = 0x7F;
        String trailingToken = encryptValidHeaderAndAad(withTrailing, deterministicNonce(0x04));
        assertEquals(
                ErrorCode.CURSOR_INVALID,
                assertThrows(
                        CursorException.class,
                        () -> codec.decode(PRIMARY_KEY, trailingToken, BASE_CONTEXT, FIXED_NOW)).getErrorCode());

        // Plaintext with TTL exceeding configured maxTtl
        byte[] overTtlPlaintext = buildCanonicalPlaintext(
                "1.0",
                "LIVE_KEYSET",
                FIXED_NOW,
                FIXED_NOW.plus(Duration.ofMinutes(16)));
        String overTtlToken = encryptValidHeaderAndAad(overTtlPlaintext, deterministicNonce(0x05));
        assertEquals(
                ErrorCode.CURSOR_INVALID,
                assertThrows(
                        CursorException.class,
                        () -> codec.decode(PRIMARY_KEY, overTtlToken, BASE_CONTEXT, FIXED_NOW)).getErrorCode());

        // Plaintext with expiresAt <= issuedAt
        byte[] invertedLifetime = buildCanonicalPlaintext("1.0", "LIVE_KEYSET", FIXED_NOW, FIXED_NOW);
        String invertedToken = encryptValidHeaderAndAad(invertedLifetime, deterministicNonce(0x06));
        assertEquals(
                ErrorCode.CURSOR_INVALID,
                assertThrows(
                        CursorException.class,
                        () -> codec.decode(PRIMARY_KEY, invertedToken, BASE_CONTEXT, FIXED_NOW)).getErrorCode());

        // Plaintext with unsupported SortValue type tag and malformed UTF-8
        byte[] badTagPlaintext = buildPlaintextWithCustomFirstValueTag((byte) 0x7F);
        String badTagToken = encryptValidHeaderAndAad(badTagPlaintext, deterministicNonce(0x07));
        assertEquals(
                ErrorCode.CURSOR_INVALID,
                assertThrows(
                        CursorException.class,
                        () -> codec.decode(PRIMARY_KEY, badTagToken, BASE_CONTEXT, FIXED_NOW)).getErrorCode());

        byte[] badUtf8Plaintext = buildPlaintextWithInvalidUtf8ResourceId();
        String badUtf8Token = encryptValidHeaderAndAad(badUtf8Plaintext, deterministicNonce(0x08));
        assertEquals(
                ErrorCode.CURSOR_INVALID,
                assertThrows(
                        CursorException.class,
                        () -> codec.decode(PRIMARY_KEY, badUtf8Token, BASE_CONTEXT, FIXED_NOW)).getErrorCode());

        // Invalid codec configuration bounds
        assertThrows(IllegalArgumentException.class, () -> new LiveKeysetCursorCodec(Duration.ZERO, 4096));
        assertThrows(
                IllegalArgumentException.class,
                () -> new LiveKeysetCursorCodec(LiveKeysetCursorCodec.ABSOLUTE_MAX_TTL.plusSeconds(1), 4096));
        assertThrows(IllegalArgumentException.class, () -> new LiveKeysetCursorCodec(Duration.ofMinutes(5), 64));
        assertThrows(
                IllegalArgumentException.class,
                () -> new LiveKeysetCursorCodec(
                        Duration.ofMinutes(5),
                        LiveKeysetCursorCodec.ABSOLUTE_MAX_TOKEN_LENGTH + 1));
    }

    @Test
    @DisplayName("Fresh 96-bit SecureRandom nonce is generated per encode for identical inputs")
    void nonceFreshnessAcrossRepeatedEncodes() {
        LiveKeysetCursorCodec codec = new LiveKeysetCursorCodec();
        int iterations = 64;
        Set<String> tokens = new HashSet<>(iterations);
        Set<String> noncesHex = new HashSet<>(iterations);

        for (int i = 0; i < iterations; i++) {
            String token = codec.encode(
                    PRIMARY_KEY,
                    BASE_CONTEXT,
                    BASE_TUPLE,
                    BASE_RESOURCE_ID,
                    FIXED_NOW,
                    STANDARD_TTL);
            assertTrue(tokens.add(token), "Encoded token must be unique per invocation");

            byte[] raw = Base64.getUrlDecoder().decode(token);
            byte[] nonce = Arrays.copyOfRange(
                    raw,
                    LiveKeysetCursorCodec.HEADER_LENGTH_BYTES,
                    LiveKeysetCursorCodec.HEADER_LENGTH_BYTES + LiveKeysetCursorCodec.GCM_NONCE_LENGTH_BYTES);
            assertEquals(12, nonce.length);
            assertTrue(noncesHex.add(Base64.getUrlEncoder().withoutPadding().encodeToString(nonce)));

            DecodedCursor decoded = codec.decode(PRIMARY_KEY, token, BASE_CONTEXT, FIXED_NOW);
            assertEquals(BASE_TUPLE, decoded.lastSortTuple());
            assertEquals(BASE_RESOURCE_ID, decoded.immutableUniqueResourceId());
        }

        assertEquals(iterations, tokens.size());
        assertEquals(iterations, noncesHex.size());
    }

    @Test
    @DisplayName("Codec types do not use native Java serialization and enforce structural invariants")
    void noNativeJavaSerializationAndStructuralBounds() {
        LiveKeysetCursorCodec codec = new LiveKeysetCursorCodec();
        String token = codec.encode(PRIMARY_KEY, BASE_CONTEXT, BASE_TUPLE, BASE_RESOURCE_ID, FIXED_NOW, STANDARD_TTL);
        DecodedCursor decoded = codec.decode(PRIMARY_KEY, token, BASE_CONTEXT, FIXED_NOW);

        assertFalse(Serializable.class.isInstance(codec));
        assertFalse(Serializable.class.isInstance(BASE_CONTEXT));
        assertFalse(Serializable.class.isInstance(BASE_CONTEXT.sortDefinition().get(0)));
        assertFalse(Serializable.class.isInstance(BASE_TUPLE.get(0)));
        assertFalse(Serializable.class.isInstance(decoded));

        // Duplicate sort field names rejected
        assertThrows(
                IllegalArgumentException.class,
                () -> new CursorContext(
                        "op",
                        "tenant-1",
                        "filter-1",
                        "scope-1",
                        List.of(SortField.asc("hostname"), SortField.desc("hostname"))));

        // Too many sort fields rejected
        List<SortField> tooManyFields = new ArrayList<>();
        for (int i = 0; i < LiveKeysetCursorCodec.MAX_SORT_FIELDS + 1; i++) {
            tooManyFields.add(SortField.asc("field_" + i));
        }
        assertThrows(
                IllegalArgumentException.class,
                () -> new CursorContext("op", "tenant-1", "filter-1", "scope-1", tooManyFields));

        // Mismatched tuple size or Java null inside tuple rejected
        assertThrows(
                IllegalArgumentException.class,
                () -> codec.encode(
                        PRIMARY_KEY,
                        BASE_CONTEXT,
                        List.of(SortValue.ofNull()),
                        BASE_RESOURCE_ID,
                        FIXED_NOW,
                        STANDARD_TTL));
        List<SortValue> tupleWithJavaNull = new ArrayList<>();
        tupleWithJavaNull.add(SortValue.ofNull());
        tupleWithJavaNull.add(null);
        assertThrows(
                IllegalArgumentException.class,
                () -> codec.encode(
                        PRIMARY_KEY,
                        BASE_CONTEXT,
                        tupleWithJavaNull,
                        BASE_RESOURCE_ID,
                        FIXED_NOW,
                        STANDARD_TTL));
    }

    @Test
    void rejectsUnpairedSurrogatesInEveryInputField() {
        LiveKeysetCursorCodec codec = new LiveKeysetCursorCodec();
        for (String malformed : List.of("x\uD800", "\uDC00x", "x\uDC00\uD800y")) {
            List<SortField> sort = BASE_CONTEXT.sortDefinition();
            assertThrows(IllegalArgumentException.class,
                    () -> new CursorContext(malformed, "tenant", "filter", "scope", sort), "operation");
            assertThrows(IllegalArgumentException.class,
                    () -> new CursorContext("op", malformed, "filter", "scope", sort), "tenant");
            assertThrows(IllegalArgumentException.class,
                    () -> new CursorContext("op", "tenant", malformed, "scope", sort), "filter");
            assertThrows(IllegalArgumentException.class,
                    () -> new CursorContext("op", "tenant", "filter", malformed, sort), "scope");
            assertThrows(IllegalArgumentException.class, () -> SortField.asc(malformed), "sort field");
            assertThrows(IllegalArgumentException.class, () -> SortValue.ofString(malformed), "sort value");
            assertThrows(IllegalArgumentException.class, () -> codec.encode(
                    PRIMARY_KEY, BASE_CONTEXT, BASE_TUPLE, malformed, FIXED_NOW, STANDARD_TTL), "resource ID");
        }
    }

    @Test
    void roundTripPreservesSupplementaryCharactersInEveryInputField() {
        LiveKeysetCursorCodec codec = new LiveKeysetCursorCodec();
        String supplementary = "\uD83D\uDE80";
        CursorContext context = new CursorContext(
                "operation-" + supplementary,
                "tenant-" + supplementary,
                "filter-" + supplementary,
                "scope-" + supplementary,
                List.of(SortField.asc("field-" + supplementary)));
        List<SortValue> tuple = List.of(SortValue.ofString("value-" + supplementary));
        String resourceId = "resource-" + supplementary;

        String token = codec.encode(PRIMARY_KEY, context, tuple, resourceId, FIXED_NOW, STANDARD_TTL);
        DecodedCursor decoded = codec.decode(PRIMARY_KEY, token, context, FIXED_NOW);
        assertEquals(context, decoded.context());
        assertEquals(tuple, decoded.lastSortTuple());
        assertEquals(resourceId, decoded.immutableUniqueResourceId());
    }

    @Test
    void malformedTenantCannotAliasLiteralQuestionMarkTenant() {
        LiveKeysetCursorCodec codec = new LiveKeysetCursorCodec();
        CursorContext literal = new CursorContext("op", "tenant-?", "filter", "scope", BASE_CONTEXT.sortDefinition());
        String token = codec.encode(PRIMARY_KEY, literal, BASE_TUPLE, BASE_RESOURCE_ID, FIXED_NOW, STANDARD_TTL);
        assertEquals(literal, codec.decode(PRIMARY_KEY, token, literal, FIXED_NOW).context());

        assertThrows(IllegalArgumentException.class, () -> codec.decode(
                PRIMARY_KEY, token,
                new CursorContext("op", "tenant-\uD800", "filter", "scope", literal.sortDefinition()), FIXED_NOW));
    }

    private static byte[] deterministicKey(int seed) {
        byte[] key = new byte[LiveKeysetCursorCodec.AES_KEY_LENGTH_BYTES];
        for (int i = 0; i < key.length; i++) {
            key[i] = (byte) ((seed + i * 7) & 0xFF);
        }
        return key;
    }

    private static byte[] deterministicNonce(int seed) {
        byte[] nonce = new byte[LiveKeysetCursorCodec.GCM_NONCE_LENGTH_BYTES];
        for (int i = 0; i < nonce.length; i++) {
            nonce[i] = (byte) ((seed + i * 13) & 0xFF);
        }
        return nonce;
    }

    private static String encryptValidHeaderAndAad(byte[] plaintext, byte[] nonce) throws Exception {
        byte[] header = LiveKeysetCursorCodec.fixedHeaderCopy();
        byte[] aad = LiveKeysetCursorCodec.buildAad(header);
        return encryptSyntheticToken(PRIMARY_KEY, header, aad, nonce, plaintext);
    }

    private static String encryptSyntheticToken(
            byte[] key,
            byte[] header,
            byte[] aad,
            byte[] nonce,
            byte[] plaintext) throws Exception {
        Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
        cipher.init(
                Cipher.ENCRYPT_MODE,
                new SecretKeySpec(key, "AES"),
                new GCMParameterSpec(LiveKeysetCursorCodec.GCM_TAG_LENGTH_BITS, nonce));
        cipher.updateAAD(aad);
        byte[] ciphertextAndTag = cipher.doFinal(plaintext);

        byte[] raw = new byte[header.length + nonce.length + ciphertextAndTag.length];
        System.arraycopy(header, 0, raw, 0, header.length);
        System.arraycopy(nonce, 0, raw, header.length, nonce.length);
        System.arraycopy(ciphertextAndTag, 0, raw, header.length + nonce.length, ciphertextAndTag.length);
        return Base64.getUrlEncoder().withoutPadding().encodeToString(raw);
    }

    private static byte[] buildCanonicalPlaintext(
            String schemaVersion,
            String cursorMode,
            Instant issuedAt,
            Instant expiresAt) {
        ByteArrayOutputStream out = new ByteArrayOutputStream();
        writeUtf8(out, schemaVersion);
        writeUtf8(out, cursorMode);
        writeUtf8(out, BASE_CONTEXT.queryOperationId());
        writeUtf8(out, BASE_CONTEXT.tenantId());
        writeUtf8(out, BASE_CONTEXT.normalizedFilterFingerprint());
        writeUtf8(out, BASE_CONTEXT.authorizedResourceScopeFingerprint());
        out.write(2);
        writeUtf8(out, "updated_at");
        out.write(0x02); // DESC
        out.write(0x02); // LAST
        writeUtf8(out, "hostname");
        out.write(0x01); // ASC
        out.write(0x01); // FIRST
        out.write(2);
        out.write(0x06); // INSTANT
        Instant tupleInstant = Instant.parse("2026-11-25T09:59:12.987654321Z");
        writeInt64(out, tupleInstant.getEpochSecond());
        writeInt32(out, tupleInstant.getNano());
        out.write(0x01); // STRING
        writeUtf8(out, "edge-router-01");
        writeUtf8(out, BASE_RESOURCE_ID);
        writeInt64(out, issuedAt.getEpochSecond());
        writeInt32(out, issuedAt.getNano());
        writeInt64(out, expiresAt.getEpochSecond());
        writeInt32(out, expiresAt.getNano());
        return out.toByteArray();
    }

    private static byte[] buildPlaintextWithCustomFirstValueTag(byte invalidTag) {
        ByteArrayOutputStream out = new ByteArrayOutputStream();
        writeUtf8(out, "1.0");
        writeUtf8(out, "LIVE_KEYSET");
        writeUtf8(out, BASE_CONTEXT.queryOperationId());
        writeUtf8(out, BASE_CONTEXT.tenantId());
        writeUtf8(out, BASE_CONTEXT.normalizedFilterFingerprint());
        writeUtf8(out, BASE_CONTEXT.authorizedResourceScopeFingerprint());
        out.write(1);
        writeUtf8(out, "updated_at");
        out.write(0x02);
        out.write(0x02);
        out.write(1);
        out.write(invalidTag & 0xFF);
        writeUtf8(out, BASE_RESOURCE_ID);
        writeInt64(out, FIXED_NOW.getEpochSecond());
        writeInt32(out, FIXED_NOW.getNano());
        Instant exp = FIXED_NOW.plus(STANDARD_TTL);
        writeInt64(out, exp.getEpochSecond());
        writeInt32(out, exp.getNano());
        return out.toByteArray();
    }

    private static byte[] buildPlaintextWithInvalidUtf8ResourceId() {
        ByteArrayOutputStream out = new ByteArrayOutputStream();
        writeUtf8(out, "1.0");
        writeUtf8(out, "LIVE_KEYSET");
        writeUtf8(out, BASE_CONTEXT.queryOperationId());
        writeUtf8(out, BASE_CONTEXT.tenantId());
        writeUtf8(out, BASE_CONTEXT.normalizedFilterFingerprint());
        writeUtf8(out, BASE_CONTEXT.authorizedResourceScopeFingerprint());
        out.write(1);
        writeUtf8(out, "updated_at");
        out.write(0x02);
        out.write(0x02);
        out.write(1);
        out.write(0x00); // NULL
        byte[] invalidUtf8 = new byte[] {(byte) 0xFF, (byte) 0xFE, (byte) 0xFD};
        out.write(0);
        out.write(invalidUtf8.length);
        out.writeBytes(invalidUtf8);
        writeInt64(out, FIXED_NOW.getEpochSecond());
        writeInt32(out, FIXED_NOW.getNano());
        Instant exp = FIXED_NOW.plus(STANDARD_TTL);
        writeInt64(out, exp.getEpochSecond());
        writeInt32(out, exp.getNano());
        return out.toByteArray();
    }

    private static void writeUtf8(ByteArrayOutputStream out, String value) {
        byte[] bytes = value.getBytes(StandardCharsets.UTF_8);
        out.write((bytes.length >>> 8) & 0xFF);
        out.write(bytes.length & 0xFF);
        out.writeBytes(bytes);
    }

    private static void writeInt32(ByteArrayOutputStream out, int value) {
        out.write((value >>> 24) & 0xFF);
        out.write((value >>> 16) & 0xFF);
        out.write((value >>> 8) & 0xFF);
        out.write(value & 0xFF);
    }

    private static void writeInt64(ByteArrayOutputStream out, long value) {
        out.write((int) (value >>> 56) & 0xFF);
        out.write((int) (value >>> 48) & 0xFF);
        out.write((int) (value >>> 40) & 0xFF);
        out.write((int) (value >>> 32) & 0xFF);
        out.write((int) (value >>> 24) & 0xFF);
        out.write((int) (value >>> 16) & 0xFF);
        out.write((int) (value >>> 8) & 0xFF);
        out.write((int) value & 0xFF);
    }
}
