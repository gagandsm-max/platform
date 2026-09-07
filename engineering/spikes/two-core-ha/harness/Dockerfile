FROM python:3.12-slim

WORKDIR /workspace

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    iproute2 \
    procps \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir \
    kafka-python==3.0.11 \
    requests==2.32.3 \
    psycopg2-binary==2.9.12

COPY . /workspace/harness/

ENV PYTHONPATH=/workspace
CMD ["python", "/workspace/harness/run_spike_tests.py"]
