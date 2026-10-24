# CI-only Day-02 PostgreSQL harness; not a product/runtime image.
# Official Docker CLI 27.5.1 static binary (Apache-2.0), source/provenance:
# https://github.com/docker-library/docker/blob/3e8d8decef166523d5440e8dfb546af7542ce84f/27/cli/Dockerfile
# Index pin selects the matching target architecture; copy no daemon or CLI plugins.
ARG MAVEN_IMAGE=maven:3.9.9-eclipse-temurin-21-alpine@sha256:5a8b906c4faa11d33f6c74758f67db8eac25441e14b0729f6d50ff78992be58a
FROM docker:27.5.1-cli@sha256:851f91d241214e7c6db86513b270d58776379aacc5eb9c4a87e5b47115e3065c AS docker-cli
FROM ${MAVEN_IMAGE}
COPY --from=docker-cli /usr/local/bin/docker /usr/local/bin/docker
# Execute in the final Maven userspace so incompatible architecture/ABI fails the build.
RUN uname -m && docker --version | grep -F 'Docker version 27.5.1,'
