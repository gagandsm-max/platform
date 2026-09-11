package com.platform.core.support;

import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.TimeUnit;
import java.util.stream.Collectors;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Regression: ephemeral Day-02 Postgres must not create anonymous Docker volumes.
 *
 * <p>Docker 29 may list tmpfs under {@code HostConfig.Tmpfs} with empty {@code Mounts}.
 */
@Tag("postgres")
class PostgresTestSupportVolumeLifecycleTest {

    @Test
    void successPathUsesTmpfsAndLeavesNoContainer() throws Exception {
        PostgresTestSupport pg = new PostgresTestSupport();
        try {
            pg.start();
            assertTmpfsPostgresData(pg.containerName());
            assertNoNamedVolumeMounts(pg.containerName());
            pg.jdbc().queryForObject("SELECT 1", Integer.class);
        } finally {
            pg.stop();
        }
        assertFalse(containerExists(pg.containerName()));
        assertNoVolumesLabeledForRun(pg.testRunId());
    }

    @Test
    void failurePathStillCleansContainerAndVolumes() throws Exception {
        PostgresTestSupport pg = new PostgresTestSupport();
        try {
            pg.start();
            assertTmpfsPostgresData(pg.containerName());
            assertNoNamedVolumeMounts(pg.containerName());
            throw new RuntimeException("induced failure after start");
        } catch (RuntimeException ex) {
            assertEquals("induced failure after start", ex.getMessage());
        } finally {
            pg.stop();
        }
        assertFalse(containerExists(pg.containerName()));
        assertNoVolumesLabeledForRun(pg.testRunId());
    }

    @Test
    void stopIsIdempotentWhenContainerAlreadyGone() {
        PostgresTestSupport pg = new PostgresTestSupport();
        pg.stop();
        pg.stop();
    }

    private static void assertTmpfsPostgresData(String container) throws Exception {
        String tmpfsJson = capture(20, "docker", "inspect",
                "--format", "{{json .HostConfig.Tmpfs}}",
                container).trim();
        assertTrue(tmpfsJson.contains("/var/lib/postgresql/data"),
                "expected HostConfig.Tmpfs for /var/lib/postgresql/data, got: " + tmpfsJson);
        assertTrue(tmpfsJson.contains("size="),
                "tmpfs options should include size=, got: " + tmpfsJson);
    }

    private static void assertNoNamedVolumeMounts(String container) throws Exception {
        String volumeNames = capture(20, "docker", "inspect",
                "--format", "{{range .Mounts}}{{if eq .Type \"volume\"}}{{.Name}};{{end}}{{end}}",
                container).trim();
        assertTrue(volumeNames.isEmpty(),
                "container must not create Docker volumes, found: " + volumeNames);
    }

    private static void assertNoVolumesLabeledForRun(String testRunId) throws Exception {
        String out = capture(60, "docker", "volume", "ls", "-q",
                "--filter", "label=com.platform.test-run=" + testRunId).trim();
        assertTrue(out.isEmpty(),
                "no leftover volumes may carry test-run label " + testRunId + ", found: " + out);
    }

    private static boolean containerExists(String name) throws Exception {
        Process p = new ProcessBuilder("docker", "inspect", name)
                .redirectErrorStream(true)
                .start();
        boolean finished = p.waitFor(10, TimeUnit.SECONDS);
        return finished && p.exitValue() == 0;
    }

    private static String capture(int timeoutSeconds, String... command) throws Exception {
        Process p = new ProcessBuilder(command).redirectErrorStream(true).start();
        boolean finished = p.waitFor(timeoutSeconds, TimeUnit.SECONDS);
        String out;
        try (BufferedReader reader = new BufferedReader(
                new InputStreamReader(p.getInputStream(), StandardCharsets.UTF_8))) {
            out = reader.lines().collect(Collectors.joining("\n"));
        }
        if (!finished) {
            p.destroyForcibly();
            throw new IllegalStateException("Command timed out: " + String.join(" ", command));
        }
        if (p.exitValue() != 0) {
            throw new IllegalStateException(
                    "Command failed (" + p.exitValue() + "): " + String.join(" ", command) + "\n" + out);
        }
        return out;
    }
}
