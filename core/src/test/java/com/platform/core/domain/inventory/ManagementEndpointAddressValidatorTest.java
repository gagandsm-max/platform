package com.platform.core.domain.inventory;

import com.platform.core.domain.DomainException;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class ManagementEndpointAddressValidatorTest {

    @Test
    void acceptsIpv4AndNormalizesLeadingZeros() {
        var n = ManagementEndpointAddressValidator.normalizeAndValidate(
                ManagementEndpointAddressType.IPV4, "192.168.001.010");
        assertEquals("192.168.1.10", n.address());
        assertEquals(ManagementEndpointAddressType.IPV4, n.type());
    }

    @Test
    void rejectsMalformedIpv4() {
        DomainException ex = assertThrows(DomainException.class,
                () -> ManagementEndpointAddressValidator.normalizeAndValidate(
                        ManagementEndpointAddressType.IPV4, "999.1.1.1"));
        assertEquals("VALIDATION_FAILED", ex.getCode());
    }

    @Test
    void acceptsIpv6AndNormalizes() {
        var n = ManagementEndpointAddressValidator.normalizeAndValidate(
                ManagementEndpointAddressType.IPV6, "2001:DB8::1");
        assertTrue(n.address().contains(":"));
        assertEquals(ManagementEndpointAddressType.IPV6, n.type());
    }

    @Test
    void rejectsMalformedIpv6() {
        assertThrows(DomainException.class,
                () -> ManagementEndpointAddressValidator.normalizeAndValidate(
                        ManagementEndpointAddressType.IPV6, "gggg::1"));
    }

    @Test
    void acceptsFqdnLowercases() {
        var n = ManagementEndpointAddressValidator.normalizeAndValidate(
                ManagementEndpointAddressType.FQDN, "Device.Example.COM.");
        assertEquals("device.example.com", n.address());
    }

    @Test
    void rejectsMalformedFqdn() {
        assertThrows(DomainException.class,
                () -> ManagementEndpointAddressValidator.normalizeAndValidate(
                        ManagementEndpointAddressType.FQDN, "-bad.example.com"));
    }

    @Test
    void rejectsHttpUrl() {
        DomainException ex = assertThrows(DomainException.class,
                () -> ManagementEndpointAddressValidator.normalizeAndValidate(
                        ManagementEndpointAddressType.FQDN, "https://device.example.com"));
        assertTrue(ex.getMessage().toLowerCase().contains("url") || ex.getMessage().contains("host"));
    }

    @Test
    void rejectsUserinfoPathQuery() {
        assertThrows(DomainException.class,
                () -> ManagementEndpointAddressValidator.normalizeAndValidate(
                        ManagementEndpointAddressType.FQDN, "user@host.example"));
        assertThrows(DomainException.class,
                () -> ManagementEndpointAddressValidator.normalizeAndValidate(
                        ManagementEndpointAddressType.FQDN, "host.example/path"));
        assertThrows(DomainException.class,
                () -> ManagementEndpointAddressValidator.normalizeAndValidate(
                        ManagementEndpointAddressType.FQDN, "host.example?q=1"));
        assertThrows(DomainException.class,
                () -> ManagementEndpointAddressValidator.normalizeAndValidate(
                        ManagementEndpointAddressType.FQDN, "host.example#frag"));
    }
}
