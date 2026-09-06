# Platform — Enterprise Network and Infrastructure Observability

Platform is a clean-room, carrier-grade Network and Infrastructure Monitoring system engineered for high-scale, multi-tenant hybrid environments.

---

## 1. Architectural Authority & Instructions

All development on Platform is strictly governed by the authoritative specification suite located in:
- `project_documentation/` (Mother-reference specifications `Platform_00` through `Platform_18`)
- `AGENTS.md` (Engineering authority chain and non-negotiable rules)

### Authority Chain Precedence:
1. `project_documentation/Platform_00_Document_Index.md`
2. Relevant BRD/PRD requirement (`Platform_01_BRD.md`, `Platform_02_PRD.md`)
3. `Platform_03_Architecture.md`
4. Relevant HLD/LLD/Dataflow sections (`Platform_04_HLD.md`, `Platform_05_LLD.md`, `Platform_06_Dataflow.md`)
5. Approved catalog: `Platform_07_OSS_Catalog.md`
6. Specialized masters (`Platform_09` through `Platform_18`)
7. `Platform_12_Decision_Source_Register.md` for resolving decision precedence

---

## 2. Non-Negotiable Engineering Principles

- **Product Name:** Strictly **Platform**.
- **Clean-room Implementation:** Zero inclusion or adaptation of legacy or proprietary competitor code.
- **Host Runtime Isolation:** Zero project-specific language runtimes or datastores installed directly on developer machines (no host Java, Go, Python virtualenvs, Node, Kafka, PostgreSQL, ClickHouse, or Valkey). All builds, runs, and tests execute inside Docker Compose.
- **Workstation Safety:** Safe, non-conflicting host port bindings (`20000–29999` on `127.0.0.1`). Foreign containers on the developer workstation are completely untouched. Never run global `docker prune`.
- **UI HTTPS-Only:** Web UI listens exclusively on TLS (container port 443, mapped to host port `23000`). No application listener on port 80.
- **Sensor Spool Protection:** All edge collectors implement disk spools guarded by a mandatory 80% storage threshold.
- **Internal Kafka:** Kafka KRaft streaming backbone is internal to Docker (`platform_net`) and is never exposed directly to the host.
- **Two-Node HA Target:** Core Active/Standby topology with failover target under 60 seconds.

---

## 3. Repository Directory Layout

```
├── AGENTS.md                  # Symlink to project_documentation/AGENTS.md
├── Makefile                   # Development automation contract
├── compose.yaml               # Docker Compose specifications with profiles
├── .env.example               # Reference environment variables
├── .env                       # Local developer environment configuration (untracked)
├── core/                      # Core Control Plane (Java 21, Spring Boot 3.3)
│   ├── src/                   # REST API, Actuator, JDBC, Kafka, Valkey, logging
│   ├── pom.xml                # Maven build definition
│   └── Dockerfile             # Multi-stage Java 21 build
├── sensor/                    # Sensor Engine (Go 1.23)
│   ├── cmd/sensor/            # Collector main daemon
│   ├── internal/              # Config, disk spool with 80% guard, Core client, health
│   ├── go.mod                 # Go module definition
│   └── Dockerfile             # Multi-stage Go build
├── agent/                     # Host Agents (Go 1.23)
│   ├── common/                # Shared identity, local queue, Core client, service harness
│   ├── linux/                 # Linux daemon wrapper
│   ├── windows/               # Windows service harness (cross-compiled)
│   ├── go.mod                 # Go module definition
│   └── Dockerfile             # Multi-stage Linux daemon / test harness
├── ui/                        # Web User Interface (React 18, TypeScript, Vite)
│   ├── src/                   # Components, navigation shell, health status, error boundary
│   ├── nginx.conf             # HTTPS-only reverse proxy configuration (No Port 80)
│   └── Dockerfile             # Multi-stage Vite build -> Nginx runtime
├── analytics/                 # Analytics Engine (Python 3.12)
│   ├── src/                   # Baseline analytics worker & health server
│   ├── requirements.txt       # OSS-approved numerical & data dependencies
│   └── Dockerfile             # Multi-stage Python 3.12 build
├── deploy/docker/             # Deployment configurations and dev assets
│   ├── certs/                 # Development Root CA and SAN certificates (untracked)
│   ├── postgres/init.sql      # PostgreSQL initialization schema & migrations
│   ├── clickhouse/init.sql    # ClickHouse initialization schema & telemetry tables
│   └── nginx/nginx.conf       # Master Nginx configuration
├── tests/                     # Verification test suites
│   ├── run-unit-tests.sh      # Containerized unit tests (Go, Java, TypeScript)
│   └── run-all-tests.sh       # 20-test automated acceptance verification suite
└── scripts/                   # Utility automation
    ├── generate-dev-certs.sh  # Development TLS certificate generator
    └── platform.sh            # CLI wrapper mirroring Makefile targets
```

---

## 4. Host Port Allocation

Platform services bind exclusively to `127.0.0.1` in the `20000–29999` range:

| Service | Container Port | Host Port (`127.0.0.1`) | Purpose | Protocol |
| :--- | :--- | :--- | :--- | :--- |
| **UI** | `443` | `23000` | HTTPS React SPA & API Gateway | HTTPS (TLS 1.2/1.3) |
| **Core** | `8443` | `28443` | Spring Boot REST API & Actuator | HTTP (Internal control) |
| **PostgreSQL** | `5432` | `25432` | Transactional Datastore | PostgreSQL |
| **ClickHouse HTTP** | `8123` | `28123` | Analytical Telemetry Query API | HTTP |
| **ClickHouse Native** | `9000` | `29000` | High-throughput ingestion | Native TCP |
| **Valkey** | `6379` | `26379` | Ephemeral Cache & State Store | Redis protocol |
| **Kafka** | `9092` | *None* | Streaming Backbone | Internal (`platform_net`) |
| **Sensors** | `8088` | *None* | Collector Spool & Probes | Internal (`platform_net`) |
| **Analytics** | `8090` | *None* | Compute Worker & Health | Internal (`platform_net`) |

---

## 5. Developer Quickstart

### Prerequisites
- Docker Engine 24+ with Compose v2+
- `make` utility
- `openssl` (for TLS dev certificates)

### Step 1: Initialize Environment
```bash
cp .env.example .env
make doctor
```

### Step 2: Build Containers
```bash
make dev-build
```

### Step 3: Start Services
```bash
make dev-up
```

### Step 4: Verify Status
```bash
make dev-status
```

Access the Web UI at:
👉 **`https://localhost:23000`** *(Accept the local dev self-signed TLS certificate)*

Core API endpoints:
- Core Health: `http://localhost:28443/actuator/health`
- Core Version: `http://localhost:28443/api/v1/version`
- Connectivity Check: `http://localhost:28443/api/v1/health/connectivity`
- Proxied via UI: `https://localhost:23000/api/v1/version`

---

## 6. Testing & Quality Gates

Platform provides two containerized testing tiers that require zero host language runtimes:

### 1. Containerized Unit Tests
Executes Go unit tests (`TestSpoolWriteAndDrain`, `TestWatermarkThreshold`, `TestIdentityGeneration`, `TestMemoryQueueOperations`), Java Spring Boot tests, and TypeScript compiler checks inside isolated build containers:
```bash
make test-unit
```

### 2. Full Integration Verification Suite (20 Acceptance Tests)
Executes the comprehensive automated 20-test verification matrix:
```bash
make test-integration
# or
make test
```

### The 20 Automated Acceptance Tests:
1. **Repository Integrity & Specification Traceability:** Verifies git metadata, `.gitignore`, `.env.example`, `AGENTS.md` symlink, and authoritative specifications.
2. **Environment Doctor Check:** Inspects Docker, Compose, TLS certificates, RAM, disk, and verifies absence of port collisions.
3. **Dev TLS Certificate & SAN Validation:** Validates Root CA and SAN entries (`localhost`, `core`, `ui`).
4. **Multi-stage Container Builds:** Verifies compilation and image assembly for Core, Sensor, UI, Analytics, and Agent.
5. **Compose Configuration Validation:** Validates compose syntax, profiles (`dev`, `test`, `ha`, `scale`, `security`, `dr`), and service definitions.
6. **Stack Startup & Health Convergence:** Launches dev stack and verifies all services reach healthy state deterministically.
7. **PostgreSQL Schema & Migrations:** Validates Postgres health and verifies bootstrap schema migration execution.
8. **ClickHouse Analytical Store:** Validates ClickHouse HTTP probe and verifies telemetry bootstrap tables.
9. **Kafka KRaft Streaming Backbone:** Validates Kafka internal broker health and enforces zero host port exposure.
10. **Valkey Fast Cache:** Validates password authentication, cluster ping, and SET/GET key verification.
11. **Core Actuator Health:** Probes `/actuator/health` and verifies status `UP`.
12. **Core API Version:** Probes `/api/v1/version` and verifies product identity `Platform`.
13. **Core Tri-Datastore Connectivity:** Probes `/api/v1/health/connectivity` and validates joint health of PostgreSQL, ClickHouse, and Valkey.
14. **Sensor Instances Startup & Health:** Verifies `sensor-net` and `sensor-infra` containers and HTTP health checks.
15. **Sensor 80% Storage Watermark Guard:** Verifies spool rejection logic when filesystem watermark exceeds 80%.
16. **UI HTTPS-Only Listener:** Verifies HTTPS on host port 23000 and verifies complete absence of port 80 listener.
17. **UI Reverse-Proxy Routing:** Verifies Nginx routing to Core `/api/v1/version` over HTTPS.
18. **Restart Persistence:** Executes `dev-down` followed by `dev-up` and verifies transactional and cache state persistence across reboot.
19. **Structured JSON Logging:** Validates JSON log format (`timestamp`, `level`, `service`, `message`) across Core, Sensor, and Analytics.
20. **Workstation Isolation:** Verifies foreign containers (`screensmart-*`) remain completely intact and zero host project runtimes exist in host `$PATH`.

---

## 7. Reboot Safety & Data Persistence

All stateful dev services (`postgres`, `clickhouse`, `kafka`, `valkey`, `sensor-net`, `sensor-infra`) use:
1. **Named Persistent Docker Volumes:**
   - `platform_pg_data`
   - `platform_ch_data`
   - `platform_kafka_data`
   - `platform_valkey_data`
   - `platform_sensor_net_spool`
   - `platform_sensor_infra_spool`
2. **Restart Policies:** `restart: unless-stopped` ensures automated supervised recovery after host reboots.
3. **Graceful Teardown:** `make dev-down` stops containers cleanly without touching persistent named volumes.

---

## 8. Reset & Clean Slate

To remove local build caches without touching database data:
```bash
make clean
```

To perform a clean-slate reset (destroys Platform containers and named volumes, requiring re-initialization):
```bash
make dev-reset CONFIRM=yes
```
*Note: `make dev-reset` is strictly bounded to `platform` resources and will NEVER touch unrelated containers or volumes on the developer workstation.*
