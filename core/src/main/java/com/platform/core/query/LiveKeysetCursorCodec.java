package com.platform.core.query;

import java.io.ByteArrayOutputStream;
import java.math.BigDecimal;
import java.math.BigInteger;
import java.nio.ByteBuffer;
import java.nio.CharBuffer;
import java.nio.charset.CharacterCodingException;
import java.nio.charset.CodingErrorAction;
import java.nio.charset.StandardCharsets;
import java.security.GeneralSecurityException;
import java.security.MessageDigest;
import java.security.SecureRandom;
import java.time.DateTimeException;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Base64;
import java.util.HashSet;
import java.util.List;
import java.util.Objects;
import java.util.Set;
import java.util.UUID;
import javax.crypto.Cipher;
import javax.crypto.spec.GCMParameterSpec;
import javax.crypto.spec.SecretKeySpec;

/**
 * Bounded, stateless, authenticated codec for Platform_17 CC-CURSOR {@code LIVE_KEYSET} pagination cursors.
 *
 * <p>Security and semantics:
 * <ul>
 *   <li>Uses AES-256-GCM with a caller-supplied 32-byte key, a fresh 96-bit {@link SecureRandom} nonce per encode,
 *       a 128-bit authentication tag, and fixed purpose plus wire format/version/mode AAD.</li>
 *   <li>Binds schema version {@code 1.0}, literal mode {@code LIVE_KEYSET}, query operation ID, tenant ID,
 *       normalized filter fingerprint, authorized resource scope fingerprint, sort definition, typed/null
 *       last sort tuple, immutable unique resource ID, {@code issued_at}, and {@code expires_at}.</li>
 *   <li>Never embeds or defaults secret keys, never uses native Java serialization, and never reads wall-clock time.</li>
 *   <li>Decoded cursor state is strictly a pagination resume position and never grants authority; callers must
 *       supply the authenticated expected query context on decode.</li>
 *   <li>{@code LIVE_KEYSET} cursors make no {@code SNAPSHOT} consistency claim and maintain no {@code seen_ids};
 *       rows may be omitted or reappear when sort keys mutate concurrently. {@code CURSOR_STALE} belongs to the
 *       owning query execution layer and is intentionally not produced by this codec.</li>
 * </ul>
 */
public final class LiveKeysetCursorCodec {

    public static final String SCHEMA_VERSION = "1.0";
    public static final String CURSOR_MODE = "LIVE_KEYSET";

    public static final int AES_KEY_LENGTH_BYTES = 32;
    public static final int GCM_NONCE_LENGTH_BYTES = 12;
    public static final int GCM_TAG_LENGTH_BITS = 128;
    public static final int GCM_TAG_LENGTH_BYTES = 16;
    public static final int HEADER_LENGTH_BYTES = 6;

    public static final Duration DEFAULT_MAX_TTL = Duration.ofMinutes(15);
    public static final Duration ABSOLUTE_MAX_TTL = Duration.ofHours(24);
    public static final int DEFAULT_MAX_TOKEN_LENGTH = 4096;
    public static final int MIN_CONFIGURABLE_MAX_TOKEN_LENGTH = 256;
    public static final int ABSOLUTE_MAX_TOKEN_LENGTH = 16384;

    public static final int MAX_OPERATION_ID_BYTES = 128;
    public static final int MAX_TENANT_ID_BYTES = 128;
    public static final int MAX_FINGERPRINT_BYTES = 256;
    public static final int MAX_SORT_FIELDS = 16;
    public static final int MAX_SORT_FIELD_NAME_BYTES = 128;
    public static final int MAX_SORT_STRING_BYTES = 1024;
    public static final int MAX_DECIMAL_UNSCALED_BYTES = 64;
    public static final int MIN_DECIMAL_SCALE = -1000;
    public static final int MAX_DECIMAL_SCALE = 1000;
    public static final int MAX_RESOURCE_ID_BYTES = 256;

    private static final int MAX_SCHEMA_VERSION_BYTES = 16;
    private static final int MAX_CURSOR_MODE_BYTES = 32;
    private static final String CIPHER_TRANSFORMATION = "AES/GCM/NoPadding";
    private static final String KEY_ALGORITHM = "AES";

    private static final byte[] WIRE_MAGIC = new byte[] {'P', 'L', 'K', 'C'};
    private static final byte WIRE_FORMAT_VERSION = 0x01;
    private static final byte WIRE_MODE_LIVE_KEYSET = 0x01;
    private static final byte[] FIXED_HEADER = new byte[] {
            WIRE_MAGIC[0], WIRE_MAGIC[1], WIRE_MAGIC[2], WIRE_MAGIC[3],
            WIRE_FORMAT_VERSION,
            WIRE_MODE_LIVE_KEYSET
    };
    private static final byte[] AAD_PURPOSE =
            "platform.core.query.cc-cursor.live-keyset.v1\0".getBytes(StandardCharsets.UTF_8);

    private final Duration maxTtl;
    private final int maxTokenLength;
    private final SecureRandom secureRandom;

    public LiveKeysetCursorCodec() {
        this(DEFAULT_MAX_TTL, DEFAULT_MAX_TOKEN_LENGTH, new SecureRandom());
    }

    public LiveKeysetCursorCodec(Duration maxTtl, int maxTokenLength) {
        this(maxTtl, maxTokenLength, new SecureRandom());
    }

    LiveKeysetCursorCodec(Duration maxTtl, int maxTokenLength, SecureRandom secureRandom) {
        if (maxTtl == null || maxTtl.isZero() || maxTtl.isNegative() || maxTtl.compareTo(ABSOLUTE_MAX_TTL) > 0) {
            throw new IllegalArgumentException(
                    "maxTtl must be positive and at most " + ABSOLUTE_MAX_TTL);
        }
        if (maxTokenLength < MIN_CONFIGURABLE_MAX_TOKEN_LENGTH || maxTokenLength > ABSOLUTE_MAX_TOKEN_LENGTH) {
            throw new IllegalArgumentException(
                    "maxTokenLength must be between "
                            + MIN_CONFIGURABLE_MAX_TOKEN_LENGTH
                            + " and "
                            + ABSOLUTE_MAX_TOKEN_LENGTH);
        }
        if (secureRandom == null) {
            throw new IllegalArgumentException("secureRandom must not be null");
        }
        this.maxTtl = maxTtl;
        this.maxTokenLength = maxTokenLength;
        this.secureRandom = secureRandom;
    }

    public Duration maxTtl() {
        return maxTtl;
    }

    public int maxTokenLength() {
        return maxTokenLength;
    }

    public String encode(
            byte[] encryptionKey,
            CursorContext context,
            List<SortValue> lastSortTuple,
            String immutableUniqueResourceId,
            Instant currentTime,
            Duration ttl) {
        validateEncryptionKey(encryptionKey);
        if (context == null) {
            throw new IllegalArgumentException("context must not be null");
        }
        if (lastSortTuple == null) {
            throw new IllegalArgumentException("lastSortTuple must not be null");
        }
        if (lastSortTuple.size() != context.sortDefinition().size()) {
            throw new IllegalArgumentException("lastSortTuple size must match sortDefinition size");
        }
        List<SortValue> tupleCopy = new ArrayList<>(lastSortTuple.size());
        for (SortValue value : lastSortTuple) {
            if (value == null) {
                throw new IllegalArgumentException(
                        "lastSortTuple must not contain Java null; use SortValue.ofNull()");
            }
            tupleCopy.add(value);
        }
        validateBoundedNonBlankString(immutableUniqueResourceId, MAX_RESOURCE_ID_BYTES, "immutableUniqueResourceId");
        if (currentTime == null) {
            throw new IllegalArgumentException("currentTime must not be null");
        }
        if (ttl == null || ttl.isZero() || ttl.isNegative() || ttl.compareTo(maxTtl) > 0) {
            throw new IllegalArgumentException("ttl must be positive and at most configured maxTtl");
        }
        final Instant expiresAt;
        try {
            expiresAt = currentTime.plus(ttl);
        } catch (DateTimeException | ArithmeticException ex) {
            throw new IllegalArgumentException("currentTime + ttl overflows supported Instant range", ex);
        }
        if (!expiresAt.isAfter(currentTime)) {
            throw new IllegalArgumentException("expiresAt must be strictly after currentTime");
        }

        byte[] plaintext = serializePayload(
                context,
                tupleCopy,
                immutableUniqueResourceId,
                currentTime,
                expiresAt);
        try {
            int rawLength = HEADER_LENGTH_BYTES + GCM_NONCE_LENGTH_BYTES + plaintext.length + GCM_TAG_LENGTH_BYTES;
            int encodedLength = encodedBase64UrlLength(rawLength);
            if (encodedLength > maxTokenLength) {
                throw new IllegalArgumentException("Encoded cursor exceeds configured maxTokenLength");
            }

            byte[] nonce = new byte[GCM_NONCE_LENGTH_BYTES];
            secureRandom.nextBytes(nonce);

            byte[] ciphertextAndTag = encryptGcm(encryptionKey, nonce, buildAad(FIXED_HEADER), plaintext);
            byte[] raw = new byte[HEADER_LENGTH_BYTES + GCM_NONCE_LENGTH_BYTES + ciphertextAndTag.length];
            System.arraycopy(FIXED_HEADER, 0, raw, 0, HEADER_LENGTH_BYTES);
            System.arraycopy(nonce, 0, raw, HEADER_LENGTH_BYTES, GCM_NONCE_LENGTH_BYTES);
            System.arraycopy(
                    ciphertextAndTag,
                    0,
                    raw,
                    HEADER_LENGTH_BYTES + GCM_NONCE_LENGTH_BYTES,
                    ciphertextAndTag.length);

            String token = Base64.getUrlEncoder().withoutPadding().encodeToString(raw);
            if (token.length() > maxTokenLength) {
                throw new IllegalArgumentException("Encoded cursor exceeds configured maxTokenLength");
            }
            return token;
        } finally {
            Arrays.fill(plaintext, (byte) 0);
        }
    }

    public DecodedCursor decode(
            byte[] encryptionKey,
            String cursorToken,
            CursorContext expectedContext,
            Instant currentTime) {
        validateEncryptionKey(encryptionKey);
        if (expectedContext == null) {
            throw new IllegalArgumentException("expectedContext must not be null");
        }
        if (currentTime == null) {
            throw new IllegalArgumentException("currentTime must not be null");
        }

        byte[] raw = decodeStrictBase64Url(cursorToken);
        int minRawLength = HEADER_LENGTH_BYTES + GCM_NONCE_LENGTH_BYTES + GCM_TAG_LENGTH_BYTES + 1;
        if (raw.length < minRawLength) {
            throw invalid("Cursor token is malformed");
        }

        byte[] header = Arrays.copyOfRange(raw, 0, HEADER_LENGTH_BYTES);
        validateWireHeader(header);

        byte[] nonce = Arrays.copyOfRange(
                raw,
                HEADER_LENGTH_BYTES,
                HEADER_LENGTH_BYTES + GCM_NONCE_LENGTH_BYTES);
        byte[] ciphertextAndTag = Arrays.copyOfRange(
                raw,
                HEADER_LENGTH_BYTES + GCM_NONCE_LENGTH_BYTES,
                raw.length);

        byte[] plaintext = decryptGcm(encryptionKey, nonce, buildAad(header), ciphertextAndTag);
        final DecodedCursor decoded;
        try {
            decoded = parsePayload(plaintext);
        } finally {
            Arrays.fill(plaintext, (byte) 0);
        }

        if (!matchesContext(decoded, expectedContext)) {
            throw new CursorException(
                    ErrorCode.CURSOR_SCOPE_CHANGED,
                    "Cursor query context does not match expected caller context");
        }
        if (decoded.issuedAt().isAfter(currentTime)) {
            throw invalid("Cursor issued_at is in the future");
        }
        if (!currentTime.isBefore(decoded.expiresAt())) {
            throw new CursorException(ErrorCode.CURSOR_EXPIRED, "Cursor has expired");
        }

        return decoded;
    }

    static byte[] fixedHeaderCopy() {
        return FIXED_HEADER.clone();
    }

    static byte[] buildAad(byte[] header) {
        byte[] aad = new byte[AAD_PURPOSE.length + header.length];
        System.arraycopy(AAD_PURPOSE, 0, aad, 0, AAD_PURPOSE.length);
        System.arraycopy(header, 0, aad, AAD_PURPOSE.length, header.length);
        return aad;
    }

    private static void validateEncryptionKey(byte[] key) {
        if (key == null || key.length != AES_KEY_LENGTH_BYTES) {
            throw new IllegalArgumentException("encryptionKey must be exactly 32 bytes");
        }
    }

    private static void validateWireHeader(byte[] header) {
        if (header.length != HEADER_LENGTH_BYTES) {
            throw invalid("Cursor header is malformed");
        }
        for (int i = 0; i < WIRE_MAGIC.length; i++) {
            if (header[i] != WIRE_MAGIC[i]) {
                throw invalid("Cursor magic is invalid");
            }
        }
        if (header[4] != WIRE_FORMAT_VERSION) {
            throw invalid("Unsupported cursor format version");
        }
        if (header[5] != WIRE_MODE_LIVE_KEYSET) {
            throw invalid("Unsupported cursor mode");
        }
    }

    private static byte[] encryptGcm(byte[] key, byte[] nonce, byte[] aad, byte[] plaintext) {
        try {
            Cipher cipher = Cipher.getInstance(CIPHER_TRANSFORMATION);
            SecretKeySpec keySpec = new SecretKeySpec(key, KEY_ALGORITHM);
            GCMParameterSpec gcmSpec = new GCMParameterSpec(GCM_TAG_LENGTH_BITS, nonce);
            cipher.init(Cipher.ENCRYPT_MODE, keySpec, gcmSpec);
            cipher.updateAAD(aad);
            return cipher.doFinal(plaintext);
        } catch (GeneralSecurityException ex) {
            throw new IllegalStateException("AES-GCM encryption failed", ex);
        }
    }

    private static byte[] decryptGcm(byte[] key, byte[] nonce, byte[] aad, byte[] ciphertextAndTag) {
        try {
            Cipher cipher = Cipher.getInstance(CIPHER_TRANSFORMATION);
            SecretKeySpec keySpec = new SecretKeySpec(key, KEY_ALGORITHM);
            GCMParameterSpec gcmSpec = new GCMParameterSpec(GCM_TAG_LENGTH_BITS, nonce);
            cipher.init(Cipher.DECRYPT_MODE, keySpec, gcmSpec);
            cipher.updateAAD(aad);
            return cipher.doFinal(ciphertextAndTag);
        } catch (GeneralSecurityException ex) {
            throw invalid("Cursor authentication failed");
        }
    }

    private byte[] decodeStrictBase64Url(String token) {
        if (token == null || token.isEmpty()) {
            throw invalid("Cursor token is empty");
        }
        int len = token.length();
        if (len > maxTokenLength) {
            throw invalid("Cursor token exceeds maximum length");
        }
        if (len % 4 == 1) {
            throw invalid("Cursor token encoding is malformed");
        }
        for (int i = 0; i < len; i++) {
            char c = token.charAt(i);
            boolean valid = (c >= 'A' && c <= 'Z')
                    || (c >= 'a' && c <= 'z')
                    || (c >= '0' && c <= '9')
                    || c == '-'
                    || c == '_';
            if (!valid) {
                throw invalid("Cursor token contains invalid characters");
            }
        }
        final byte[] decoded;
        try {
            decoded = Base64.getUrlDecoder().decode(token);
        } catch (IllegalArgumentException ex) {
            throw invalid("Cursor token encoding is malformed");
        }
        String canonical = Base64.getUrlEncoder().withoutPadding().encodeToString(decoded);
        if (!canonical.equals(token)) {
            throw invalid("Cursor token encoding is non-canonical");
        }
        return decoded;
    }

    private static int encodedBase64UrlLength(int rawBytes) {
        int fullGroups = (rawBytes / 3) * 4;
        int rem = rawBytes % 3;
        if (rem == 0) {
            return fullGroups;
        }
        return fullGroups + rem + 1;
    }

    private static byte[] serializePayload(
            CursorContext context,
            List<SortValue> lastSortTuple,
            String immutableUniqueResourceId,
            Instant issuedAt,
            Instant expiresAt) {
        ByteArrayOutputStream out = new ByteArrayOutputStream(256);

        writeBoundedUtf8String(out, SCHEMA_VERSION, 1, MAX_SCHEMA_VERSION_BYTES, "schemaVersion");
        writeBoundedUtf8String(out, CURSOR_MODE, 1, MAX_CURSOR_MODE_BYTES, "cursorMode");
        writeBoundedUtf8String(out, context.queryOperationId(), 1, MAX_OPERATION_ID_BYTES, "queryOperationId");
        writeBoundedUtf8String(out, context.tenantId(), 1, MAX_TENANT_ID_BYTES, "tenantId");
        writeBoundedUtf8String(
                out,
                context.normalizedFilterFingerprint(),
                1,
                MAX_FINGERPRINT_BYTES,
                "normalizedFilterFingerprint");
        writeBoundedUtf8String(
                out,
                context.authorizedResourceScopeFingerprint(),
                1,
                MAX_FINGERPRINT_BYTES,
                "authorizedResourceScopeFingerprint");

        List<SortField> sortDefinition = context.sortDefinition();
        writeUInt8(out, sortDefinition.size());
        for (SortField field : sortDefinition) {
            writeBoundedUtf8String(out, field.field(), 1, MAX_SORT_FIELD_NAME_BYTES, "sortField.field");
            writeUInt8(out, field.direction().wireCode());
            writeUInt8(out, field.nullsOrder().wireCode());
        }

        writeUInt8(out, lastSortTuple.size());
        for (SortValue value : lastSortTuple) {
            writeSortValue(out, value);
        }

        writeBoundedUtf8String(out, immutableUniqueResourceId, 1, MAX_RESOURCE_ID_BYTES, "immutableUniqueResourceId");
        writeInstant(out, issuedAt);
        writeInstant(out, expiresAt);

        return out.toByteArray();
    }

    private DecodedCursor parsePayload(byte[] plaintext) {
        ByteBuffer buf = ByteBuffer.wrap(plaintext);

        String schemaVersion = readBoundedNonBlankUtf8String(buf, MAX_SCHEMA_VERSION_BYTES);
        if (!SCHEMA_VERSION.equals(schemaVersion)) {
            throw invalid("Unsupported cursor schema version");
        }

        String cursorMode = readBoundedNonBlankUtf8String(buf, MAX_CURSOR_MODE_BYTES);
        if (!CURSOR_MODE.equals(cursorMode)) {
            throw invalid("Unsupported cursor mode");
        }

        String queryOperationId = readBoundedNonBlankUtf8String(buf, MAX_OPERATION_ID_BYTES);
        String tenantId = readBoundedNonBlankUtf8String(buf, MAX_TENANT_ID_BYTES);
        String normalizedFilterFingerprint = readBoundedNonBlankUtf8String(buf, MAX_FINGERPRINT_BYTES);
        String authorizedResourceScopeFingerprint = readBoundedNonBlankUtf8String(buf, MAX_FINGERPRINT_BYTES);

        int sortCount = readUInt8(buf);
        if (sortCount < 1 || sortCount > MAX_SORT_FIELDS) {
            throw invalid("Invalid sort field count");
        }
        List<SortField> sortDefinition = new ArrayList<>(sortCount);
        Set<String> seenFields = new HashSet<>(sortCount);
        for (int i = 0; i < sortCount; i++) {
            String fieldName = readBoundedNonBlankUtf8String(buf, MAX_SORT_FIELD_NAME_BYTES);
            if (!seenFields.add(fieldName)) {
                throw invalid("Duplicate sort field in cursor payload");
            }
            SortDirection direction = SortDirection.fromWireCode(readUInt8(buf));
            NullsOrder nullsOrder = NullsOrder.fromWireCode(readUInt8(buf));
            sortDefinition.add(new SortField(fieldName, direction, nullsOrder));
        }

        int tupleCount = readUInt8(buf);
        if (tupleCount != sortCount) {
            throw invalid("Sort tuple length does not match sort definition");
        }
        List<SortValue> lastSortTuple = new ArrayList<>(tupleCount);
        for (int i = 0; i < tupleCount; i++) {
            lastSortTuple.add(readSortValue(buf));
        }

        String immutableUniqueResourceId = readBoundedNonBlankUtf8String(buf, MAX_RESOURCE_ID_BYTES);
        Instant issuedAt = readInstant(buf);
        Instant expiresAt = readInstant(buf);

        if (buf.hasRemaining()) {
            throw invalid("Trailing bytes in cursor payload");
        }

        if (!expiresAt.isAfter(issuedAt)) {
            throw invalid("Cursor lifetime is invalid");
        }
        final Duration lifetime;
        try {
            lifetime = Duration.between(issuedAt, expiresAt);
        } catch (ArithmeticException ex) {
            throw invalid("Cursor lifetime is out of bounds");
        }
        if (lifetime.compareTo(maxTtl) > 0) {
            throw invalid("Cursor TTL exceeds configured maximum");
        }

        return new DecodedCursor(
                schemaVersion,
                cursorMode,
                queryOperationId,
                tenantId,
                normalizedFilterFingerprint,
                authorizedResourceScopeFingerprint,
                sortDefinition,
                lastSortTuple,
                immutableUniqueResourceId,
                issuedAt,
                expiresAt);
    }

    private static boolean matchesContext(DecodedCursor decoded, CursorContext expected) {
        boolean tenantMatch = constantTimeEqualsUtf8(decoded.tenantId(), expected.tenantId(), MAX_TENANT_ID_BYTES);
        boolean scopeMatch = constantTimeEqualsUtf8(
                decoded.authorizedResourceScopeFingerprint(),
                expected.authorizedResourceScopeFingerprint(),
                MAX_FINGERPRINT_BYTES);
        boolean operationMatch = constantTimeEqualsUtf8(
                decoded.queryOperationId(), expected.queryOperationId(), MAX_OPERATION_ID_BYTES);
        boolean filterMatch = constantTimeEqualsUtf8(
                decoded.normalizedFilterFingerprint(),
                expected.normalizedFilterFingerprint(),
                MAX_FINGERPRINT_BYTES);
        boolean sortMatch = decoded.sortDefinition().equals(expected.sortDefinition());
        return tenantMatch & scopeMatch & operationMatch & filterMatch & sortMatch;
    }

    private static boolean constantTimeEqualsUtf8(String left, String right, int maxBytes) {
        return MessageDigest.isEqual(
                encodeStrictUtf8(left, maxBytes),
                encodeStrictUtf8(right, maxBytes));
    }

    private static void writeSortValue(ByteArrayOutputStream out, SortValue value) {
        switch (value) {
            case SortValue.NullValue ignored -> writeUInt8(out, SortValueType.NULL.wireTag());
            case SortValue.StringValue sv -> {
                writeUInt8(out, SortValueType.STRING.wireTag());
                writeBoundedUtf8String(out, sv.value(), 0, MAX_SORT_STRING_BYTES, "sortValue.string");
            }
            case SortValue.Int64Value iv -> {
                writeUInt8(out, SortValueType.INT64.wireTag());
                writeInt64(out, iv.value());
            }
            case SortValue.Float64Value fv -> {
                writeUInt8(out, SortValueType.FLOAT64.wireTag());
                writeInt64(out, Double.doubleToLongBits(fv.value()));
            }
            case SortValue.DecimalValue dv -> {
                BigDecimal dec = dv.value();
                byte[] unscaled = dec.unscaledValue().toByteArray();
                writeUInt8(out, SortValueType.DECIMAL.wireTag());
                writeInt32(out, dec.scale());
                writeUInt16(out, unscaled.length);
                out.writeBytes(unscaled);
            }
            case SortValue.BooleanValue bv -> {
                writeUInt8(out, SortValueType.BOOLEAN.wireTag());
                writeUInt8(out, bv.value() ? 1 : 0);
            }
            case SortValue.InstantValue inv -> {
                writeUInt8(out, SortValueType.INSTANT.wireTag());
                writeInstant(out, inv.value());
            }
            case SortValue.UuidValue uv -> {
                writeUInt8(out, SortValueType.UUID.wireTag());
                writeInt64(out, uv.value().getMostSignificantBits());
                writeInt64(out, uv.value().getLeastSignificantBits());
            }
        }
    }

    private static SortValue readSortValue(ByteBuffer buf) {
        int tag = readUInt8(buf);
        SortValueType type = SortValueType.fromWireTag(tag);
        return switch (type) {
            case NULL -> SortValue.ofNull();
            case STRING -> SortValue.ofString(readBoundedUtf8String(buf, 0, MAX_SORT_STRING_BYTES));
            case INT64 -> {
                if (buf.remaining() < 8) {
                    throw invalid("Truncated int64 sort value");
                }
                yield SortValue.ofInt64(buf.getLong());
            }
            case FLOAT64 -> {
                if (buf.remaining() < 8) {
                    throw invalid("Truncated float64 sort value");
                }
                double d = Double.longBitsToDouble(buf.getLong());
                if (!Double.isFinite(d)) {
                    throw invalid("Non-finite float64 sort value");
                }
                yield SortValue.ofFloat64(d);
            }
            case DECIMAL -> {
                if (buf.remaining() < 4) {
                    throw invalid("Truncated decimal scale");
                }
                int scale = buf.getInt();
                if (scale < MIN_DECIMAL_SCALE || scale > MAX_DECIMAL_SCALE) {
                    throw invalid("Decimal scale out of bounds");
                }
                int len = readUInt16(buf);
                if (len < 1 || len > MAX_DECIMAL_UNSCALED_BYTES || buf.remaining() < len) {
                    throw invalid("Invalid decimal unscaled byte length");
                }
                byte[] unscaledBytes = new byte[len];
                buf.get(unscaledBytes);
                BigInteger unscaled = new BigInteger(unscaledBytes);
                if (!Arrays.equals(unscaled.toByteArray(), unscaledBytes)) {
                    throw invalid("Non-canonical decimal encoding");
                }
                yield SortValue.ofDecimal(new BigDecimal(unscaled, scale));
            }
            case BOOLEAN -> {
                int b = readUInt8(buf);
                if (b == 0) {
                    yield SortValue.ofBoolean(false);
                }
                if (b == 1) {
                    yield SortValue.ofBoolean(true);
                }
                throw invalid("Invalid boolean sort value");
            }
            case INSTANT -> SortValue.ofInstant(readInstant(buf));
            case UUID -> {
                if (buf.remaining() < 16) {
                    throw invalid("Truncated UUID sort value");
                }
                yield SortValue.ofUuid(new UUID(buf.getLong(), buf.getLong()));
            }
        };
    }

    private static void writeBoundedUtf8String(
            ByteArrayOutputStream out,
            String value,
            int minBytes,
            int maxBytes,
            String fieldName) {
        byte[] bytes = encodeStrictUtf8(value, maxBytes);
        if (bytes.length < minBytes || bytes.length > maxBytes) {
            throw new IllegalArgumentException(fieldName + " UTF-8 byte length out of bounds");
        }
        writeUInt16(out, bytes.length);
        out.writeBytes(bytes);
    }

    private static String readBoundedNonBlankUtf8String(ByteBuffer buf, int maxBytes) {
        String value = readBoundedUtf8String(buf, 1, maxBytes);
        if (value.isBlank()) {
            throw invalid("Blank required field in cursor payload");
        }
        return value;
    }

    private static String readBoundedUtf8String(ByteBuffer buf, int minBytes, int maxBytes) {
        int len = readUInt16(buf);
        if (len < minBytes || len > maxBytes || buf.remaining() < len) {
            throw invalid("Invalid string field length in cursor payload");
        }
        byte[] bytes = new byte[len];
        buf.get(bytes);
        return decodeStrictUtf8(bytes);
    }

    private static byte[] encodeStrictUtf8(String value, int maxBytes) {
        // UTF-8 needs at least one byte per UTF-16 code unit.
        if (value.length() > maxBytes) {
            throw new IllegalArgumentException("Cursor string exceeds maximum UTF-8 byte length of " + maxBytes);
        }
        try {
            ByteBuffer encoded = StandardCharsets.UTF_8.newEncoder()
                    .onMalformedInput(CodingErrorAction.REPORT)
                    .onUnmappableCharacter(CodingErrorAction.REPORT)
                    .encode(CharBuffer.wrap(value));
            byte[] bytes = new byte[encoded.remaining()];
            encoded.get(bytes);
            return bytes;
        } catch (CharacterCodingException ex) {
            throw new IllegalArgumentException("Malformed UTF-16 in cursor string", ex);
        }
    }

    private static String decodeStrictUtf8(byte[] bytes) {
        try {
            CharBuffer decoded = StandardCharsets.UTF_8.newDecoder()
                    .onMalformedInput(CodingErrorAction.REPORT)
                    .onUnmappableCharacter(CodingErrorAction.REPORT)
                    .decode(ByteBuffer.wrap(bytes));
            return decoded.toString();
        } catch (CharacterCodingException ex) {
            throw invalid("Malformed UTF-8 in cursor payload");
        }
    }

    private static void writeInstant(ByteArrayOutputStream out, Instant instant) {
        writeInt64(out, instant.getEpochSecond());
        writeInt32(out, instant.getNano());
    }

    private static Instant readInstant(ByteBuffer buf) {
        if (buf.remaining() < 12) {
            throw invalid("Truncated timestamp in cursor payload");
        }
        long epochSecond = buf.getLong();
        int nano = buf.getInt();
        if (nano < 0 || nano > 999_999_999) {
            throw invalid("Invalid timestamp nanoseconds in cursor payload");
        }
        if (epochSecond < Instant.MIN.getEpochSecond() || epochSecond > Instant.MAX.getEpochSecond()) {
            throw invalid("Invalid timestamp seconds in cursor payload");
        }
        try {
            return Instant.ofEpochSecond(epochSecond, nano);
        } catch (DateTimeException ex) {
            throw invalid("Invalid timestamp in cursor payload");
        }
    }

    private static void writeUInt8(ByteArrayOutputStream out, int value) {
        if (value < 0 || value > 0xFF) {
            throw new IllegalArgumentException("uint8 value out of range: " + value);
        }
        out.write(value & 0xFF);
    }

    private static int readUInt8(ByteBuffer buf) {
        if (buf.remaining() < 1) {
            throw invalid("Truncated cursor payload");
        }
        return Byte.toUnsignedInt(buf.get());
    }

    private static void writeUInt16(ByteArrayOutputStream out, int value) {
        if (value < 0 || value > 0xFFFF) {
            throw new IllegalArgumentException("uint16 value out of range: " + value);
        }
        out.write((value >>> 8) & 0xFF);
        out.write(value & 0xFF);
    }

    private static int readUInt16(ByteBuffer buf) {
        if (buf.remaining() < 2) {
            throw invalid("Truncated cursor payload");
        }
        return Short.toUnsignedInt(buf.getShort());
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

    private static void validateBoundedNonBlankString(String value, int maxBytes, String fieldName) {
        if (value == null || value.isBlank()) {
            throw new IllegalArgumentException(fieldName + " must not be null or blank");
        }
        int utf8Length = encodeStrictUtf8(value, maxBytes).length;
        if (utf8Length > maxBytes) {
            throw new IllegalArgumentException(fieldName + " exceeds maximum UTF-8 byte length of " + maxBytes);
        }
    }

    private static CursorException invalid(String message) {
        return new CursorException(ErrorCode.CURSOR_INVALID, message);
    }

    public enum ErrorCode {
        CURSOR_INVALID,
        CURSOR_EXPIRED,
        CURSOR_SCOPE_CHANGED
    }

    public static final class CursorException extends RuntimeException {

        private static final long serialVersionUID = 1L;
        private final ErrorCode errorCode;

        public CursorException(ErrorCode errorCode, String message) {
            super(Objects.requireNonNull(message, "message must not be null"));
            this.errorCode = Objects.requireNonNull(errorCode, "errorCode must not be null");
        }

        public ErrorCode getErrorCode() {
            return errorCode;
        }
    }

    public enum SortDirection {
        ASC(0x01),
        DESC(0x02);

        private final int wireCode;

        SortDirection(int wireCode) {
            this.wireCode = wireCode;
        }

        int wireCode() {
            return wireCode;
        }

        static SortDirection fromWireCode(int code) {
            return switch (code) {
                case 0x01 -> ASC;
                case 0x02 -> DESC;
                default -> throw invalid("Invalid sort direction code");
            };
        }
    }

    public enum NullsOrder {
        FIRST(0x01),
        LAST(0x02);

        private final int wireCode;

        NullsOrder(int wireCode) {
            this.wireCode = wireCode;
        }

        int wireCode() {
            return wireCode;
        }

        static NullsOrder fromWireCode(int code) {
            return switch (code) {
                case 0x01 -> FIRST;
                case 0x02 -> LAST;
                default -> throw invalid("Invalid nulls order code");
            };
        }
    }

    public record SortField(String field, SortDirection direction, NullsOrder nullsOrder) {

        public SortField {
            validateBoundedNonBlankString(field, MAX_SORT_FIELD_NAME_BYTES, "sortField.field");
            if (direction == null) {
                throw new IllegalArgumentException("sortField.direction must not be null");
            }
            if (nullsOrder == null) {
                throw new IllegalArgumentException("sortField.nullsOrder must not be null");
            }
        }

        public static SortField asc(String field) {
            return new SortField(field, SortDirection.ASC, NullsOrder.LAST);
        }

        public static SortField desc(String field) {
            return new SortField(field, SortDirection.DESC, NullsOrder.LAST);
        }

        public static SortField of(String field, SortDirection direction, NullsOrder nullsOrder) {
            return new SortField(field, direction, nullsOrder);
        }
    }

    public record CursorContext(
            String queryOperationId,
            String tenantId,
            String normalizedFilterFingerprint,
            String authorizedResourceScopeFingerprint,
            List<SortField> sortDefinition) {

        public CursorContext {
            validateBoundedNonBlankString(queryOperationId, MAX_OPERATION_ID_BYTES, "queryOperationId");
            validateBoundedNonBlankString(tenantId, MAX_TENANT_ID_BYTES, "tenantId");
            validateBoundedNonBlankString(
                    normalizedFilterFingerprint,
                    MAX_FINGERPRINT_BYTES,
                    "normalizedFilterFingerprint");
            validateBoundedNonBlankString(
                    authorizedResourceScopeFingerprint,
                    MAX_FINGERPRINT_BYTES,
                    "authorizedResourceScopeFingerprint");
            if (sortDefinition == null || sortDefinition.isEmpty()) {
                throw new IllegalArgumentException("sortDefinition must not be null or empty");
            }
            if (sortDefinition.size() > MAX_SORT_FIELDS) {
                throw new IllegalArgumentException("sortDefinition exceeds maximum field count of " + MAX_SORT_FIELDS);
            }
            List<SortField> copy = new ArrayList<>(sortDefinition.size());
            Set<String> seenFields = new HashSet<>(sortDefinition.size());
            for (SortField field : sortDefinition) {
                if (field == null) {
                    throw new IllegalArgumentException("sortDefinition must not contain null entries");
                }
                if (!seenFields.add(field.field())) {
                    throw new IllegalArgumentException("sortDefinition must not contain duplicate field names");
                }
                copy.add(field);
            }
            sortDefinition = List.copyOf(copy);
        }
    }

    public enum SortValueType {
        NULL(0x00),
        STRING(0x01),
        INT64(0x02),
        FLOAT64(0x03),
        DECIMAL(0x04),
        BOOLEAN(0x05),
        INSTANT(0x06),
        UUID(0x07);

        private final int wireTag;

        SortValueType(int wireTag) {
            this.wireTag = wireTag;
        }

        int wireTag() {
            return wireTag;
        }

        static SortValueType fromWireTag(int tag) {
            return switch (tag) {
                case 0x00 -> NULL;
                case 0x01 -> STRING;
                case 0x02 -> INT64;
                case 0x03 -> FLOAT64;
                case 0x04 -> DECIMAL;
                case 0x05 -> BOOLEAN;
                case 0x06 -> INSTANT;
                case 0x07 -> UUID;
                default -> throw invalid("Unsupported sort value type tag");
            };
        }
    }

    public sealed interface SortValue permits
            SortValue.NullValue,
            SortValue.StringValue,
            SortValue.Int64Value,
            SortValue.Float64Value,
            SortValue.DecimalValue,
            SortValue.BooleanValue,
            SortValue.InstantValue,
            SortValue.UuidValue {

        SortValueType type();

        static NullValue ofNull() {
            return NullValue.INSTANCE;
        }

        static StringValue ofString(String value) {
            return new StringValue(value);
        }

        static Int64Value ofInt64(long value) {
            return new Int64Value(value);
        }

        static Float64Value ofFloat64(double value) {
            return new Float64Value(value);
        }

        static DecimalValue ofDecimal(BigDecimal value) {
            return new DecimalValue(value);
        }

        static BooleanValue ofBoolean(boolean value) {
            return new BooleanValue(value);
        }

        static InstantValue ofInstant(Instant value) {
            return new InstantValue(value);
        }

        static UuidValue ofUuid(UUID value) {
            return new UuidValue(value);
        }

        record NullValue() implements SortValue {
            public static final NullValue INSTANCE = new NullValue();

            @Override
            public SortValueType type() {
                return SortValueType.NULL;
            }
        }

        record StringValue(String value) implements SortValue {
            public StringValue {
                if (value == null) {
                    throw new IllegalArgumentException("StringValue must not be null; use SortValue.ofNull()");
                }
                int utf8Len = encodeStrictUtf8(value, MAX_SORT_STRING_BYTES).length;
                if (utf8Len > MAX_SORT_STRING_BYTES) {
                    throw new IllegalArgumentException(
                            "StringValue UTF-8 byte length exceeds maximum of " + MAX_SORT_STRING_BYTES);
                }
            }

            @Override
            public SortValueType type() {
                return SortValueType.STRING;
            }
        }

        record Int64Value(long value) implements SortValue {
            @Override
            public SortValueType type() {
                return SortValueType.INT64;
            }
        }

        record Float64Value(double value) implements SortValue {
            public Float64Value {
                if (!Double.isFinite(value)) {
                    throw new IllegalArgumentException("Float64Value must be finite");
                }
            }

            @Override
            public SortValueType type() {
                return SortValueType.FLOAT64;
            }
        }

        record DecimalValue(BigDecimal value) implements SortValue {
            public DecimalValue {
                if (value == null) {
                    throw new IllegalArgumentException("DecimalValue must not be null; use SortValue.ofNull()");
                }
                int scale = value.scale();
                if (scale < MIN_DECIMAL_SCALE || scale > MAX_DECIMAL_SCALE) {
                    throw new IllegalArgumentException("DecimalValue scale out of bounds");
                }
                int unscaledLen = value.unscaledValue().toByteArray().length;
                if (unscaledLen > MAX_DECIMAL_UNSCALED_BYTES) {
                    throw new IllegalArgumentException(
                            "DecimalValue unscaled byte length exceeds maximum of " + MAX_DECIMAL_UNSCALED_BYTES);
                }
            }

            @Override
            public SortValueType type() {
                return SortValueType.DECIMAL;
            }
        }

        record BooleanValue(boolean value) implements SortValue {
            @Override
            public SortValueType type() {
                return SortValueType.BOOLEAN;
            }
        }

        record InstantValue(Instant value) implements SortValue {
            public InstantValue {
                if (value == null) {
                    throw new IllegalArgumentException("InstantValue must not be null; use SortValue.ofNull()");
                }
            }

            @Override
            public SortValueType type() {
                return SortValueType.INSTANT;
            }
        }

        record UuidValue(UUID value) implements SortValue {
            public UuidValue {
                if (value == null) {
                    throw new IllegalArgumentException("UuidValue must not be null; use SortValue.ofNull()");
                }
            }

            @Override
            public SortValueType type() {
                return SortValueType.UUID;
            }
        }
    }

    public record DecodedCursor(
            String schemaVersion,
            String cursorMode,
            String queryOperationId,
            String tenantId,
            String normalizedFilterFingerprint,
            String authorizedResourceScopeFingerprint,
            List<SortField> sortDefinition,
            List<SortValue> lastSortTuple,
            String immutableUniqueResourceId,
            Instant issuedAt,
            Instant expiresAt) {

        public DecodedCursor {
            if (!SCHEMA_VERSION.equals(schemaVersion)) {
                throw new IllegalArgumentException("Unsupported schemaVersion");
            }
            if (!CURSOR_MODE.equals(cursorMode)) {
                throw new IllegalArgumentException("Unsupported cursorMode");
            }
            validateBoundedNonBlankString(queryOperationId, MAX_OPERATION_ID_BYTES, "queryOperationId");
            validateBoundedNonBlankString(tenantId, MAX_TENANT_ID_BYTES, "tenantId");
            validateBoundedNonBlankString(
                    normalizedFilterFingerprint,
                    MAX_FINGERPRINT_BYTES,
                    "normalizedFilterFingerprint");
            validateBoundedNonBlankString(
                    authorizedResourceScopeFingerprint,
                    MAX_FINGERPRINT_BYTES,
                    "authorizedResourceScopeFingerprint");
            if (sortDefinition == null || sortDefinition.isEmpty() || sortDefinition.size() > MAX_SORT_FIELDS) {
                throw new IllegalArgumentException("Invalid sortDefinition");
            }
            if (lastSortTuple == null || lastSortTuple.size() != sortDefinition.size()) {
                throw new IllegalArgumentException("lastSortTuple size must match sortDefinition size");
            }
            sortDefinition = List.copyOf(sortDefinition);
            lastSortTuple = List.copyOf(lastSortTuple);
            validateBoundedNonBlankString(immutableUniqueResourceId, MAX_RESOURCE_ID_BYTES, "immutableUniqueResourceId");
            if (issuedAt == null || expiresAt == null || !expiresAt.isAfter(issuedAt)) {
                throw new IllegalArgumentException("expiresAt must be strictly after issuedAt");
            }
        }

        public CursorContext context() {
            return new CursorContext(
                    queryOperationId,
                    tenantId,
                    normalizedFilterFingerprint,
                    authorizedResourceScopeFingerprint,
                    sortDefinition);
        }
    }
}
