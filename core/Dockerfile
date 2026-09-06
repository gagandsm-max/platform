# ==============================================================================
# Platform Core Dockerfile (Clean-room Multi-stage Build)
# ==============================================================================

FROM maven:3.9.9-eclipse-temurin-21-alpine AS builder
WORKDIR /build

# Cache Maven dependencies
COPY pom.xml .
RUN mvn dependency:go-offline -B

# Copy source code and build application jar
COPY src ./src
RUN mvn clean package -DskipTests -B

# ==============================================================================
# Runtime Stage
# ==============================================================================
FROM eclipse-temurin:21-jre-alpine
WORKDIR /app

RUN apk add --no-cache curl tzdata \
    && addgroup -S platform \
    && adduser -S platform -G platform

COPY --from=builder /build/target/platform-core-*.jar /app/platform-core.jar
RUN chown -R platform:platform /app

USER platform
EXPOSE 8443

HEALTHCHECK --interval=10s --timeout=5s --start-period=30s --retries=3 \
    CMD curl -k -f https://127.0.0.1:8443/actuator/health || exit 1

ENTRYPOINT ["java", "-XX:+UseContainerSupport", "-XX:MaxRAMPercentage=75.0", "-jar", "/app/platform-core.jar"]
