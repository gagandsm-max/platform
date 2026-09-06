# ==============================================================================
# Platform Analytics Dockerfile (Clean-room Multi-stage Build)
# ==============================================================================

FROM python:3.12-slim AS builder
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

RUN python -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# ==============================================================================
# Runtime Stage
# ==============================================================================
FROM python:3.12-slim
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r platform && useradd -r -g platform platform

COPY --from=builder /opt/venv /opt/venv
COPY src ./src

ENV PATH="/opt/venv/bin:$PATH" \
    PYTHONUNBUFFERED=1

USER platform
EXPOSE 8090

HEALTHCHECK --interval=10s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://127.0.0.1:8090/health || exit 1

ENTRYPOINT ["python", "src/main.py"]
