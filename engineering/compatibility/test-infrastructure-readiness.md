# Platform v1.0 — Commercial GA Test Infrastructure Readiness Matrix

**Document ID:** PLATFORM-COMPAT-INFRA-001  
**Status:** **ACTIVE / DAY 01 AUDIT**  
**Governing Documents:** `AGENTS.md`, `Platform_14_Test_Performance_and_Release_Quality.md`, `Platform_15_CI_CD_Packaging_and_Distribution.md`  

---

## 1. Classification Methodology

Every target environment and test dependency is classified into one of four states:
1. **AVAILABLE:** Fully available, functional, and verifiable on the current developer system or via local containerization without external dependency.
2. **NEEDS CREATION:** Can be created, simulated, or automated via local Docker Compose, mock containers, or open-source test harness scripts without purchasing hardware or external services.
3. **NEEDS USER/EXTERNAL ACCESS:** Requires bare-metal hardware, dedicated multi-node virtual machines, proprietary cloud accounts, enterprise hypervisors, or customer-provided access.
4. **NOT YET REQUIRED:** Sized for post-v1.0 or deferred beyond current milestone test gates.

---

## 2. Infrastructure Inventory & Readiness Matrix

| Test Target / Dependency | Classification | Current State / How Provided | Lab / User Action Required |
| :--- | :--- | :--- | :--- |
| **Local Docker Development Environment** | **AVAILABLE** | Linux host running Docker 29.7 and Compose 5.5 | None. Fully operational. |
| **PostgreSQL Target (Internal & Monitored)** | **AVAILABLE** | Containerized PostgreSQL 16 Alpine in Compose | None. |
| **ClickHouse Target (Internal & Monitored)** | **AVAILABLE** | Containerized ClickHouse 24.8 Alpine in Compose | None. |
| **Kafka Broker Target (Internal)** | **AVAILABLE** | Containerized Apache Kafka 3.8 KRaft in Compose | None. |
| **Valkey Cache Target (Internal)** | **AVAILABLE** | Containerized Valkey 7.2 Alpine in Compose | None. |
| **mTLS Sensor Transport Harness** | **AVAILABLE** | Development PKI generated in `deploy/docker/certs/` | None. |
| **SNMP v1/v2c Simulator** | **NEEDS CREATION** | Can deploy `snmpsim` container in test profile | Build test compose profile with `snmpsim` on Day 2. |
| **SNMP v3 Simulator (USM: Auth/Priv)** | **NEEDS CREATION** | Can configure `snmpsim` with SHA/AES users | Script users in test compose profile on Day 2. |
| **Real SNMP Physical Device Access** | **NEEDS USER/EXTERNAL ACCESS** | No physical switches/routers on developer laptop | User/Lab to provide lab switch or VPN access for end-to-end testing. |
| **NetFlow / sFlow / IPFIX Flow Generator**| **NEEDS CREATION** | Can deploy `flowgen` / `nflow-generator` container | Add flow generation service to test compose profile on Day 3. |
| **Syslog Generator (RFC 5424 / RFC 3164)** | **NEEDS CREATION** | Can deploy `loggen` container in test profile | Add syslog generator script on Day 3. |
| **SSH / Linux Device Simulator** | **NEEDS CREATION** | Mock OpenSSH container running on test bridge | Script SSH container with pre-seeded test accounts on Day 4. |
| **Webhook Receiver (PagerDuty / Slack mock)** | **NEEDS CREATION** | Local Go HTTP receiver in test harness | Implement mock receiver container on Day 8. |
| **SMTP Test Target** | **NEEDS CREATION** | Can deploy `mailpit` or `mailhog` container | Add lightweight mail receiver to test profile on Day 8. |
| **gNMI Simulator / Target** | **NEEDS CREATION** | Can deploy open-source `gnmi_target` container | Build mock gNMI target container on Day 7. |
| **NETCONF Target** | **NEEDS CREATION** | Can deploy `netopeer2` container in test network | Add NETCONF mock service on Day 7. |
| **MySQL / MariaDB Target** | **NEEDS CREATION** | Can deploy `mariadb:10.11` container in test profile | Add database monitoring target container on Day 9. |
| **MongoDB Target** | **NEEDS CREATION** | Can deploy `mongo:7.0` container in test profile | Add database monitoring target container on Day 9. |
| **Redis / Valkey Monitored Target** | **NEEDS CREATION** | Dedicated unauthenticated/standalone Valkey target | Add monitored target container on Day 9. |
| **Two-Core HA Testing (Active/Standby VIP)** | **NEEDS USER/EXTERNAL ACCESS** | Local Compose has profiles, but true VIP / ARP takeover requires 2 separate hosts/VMs | User/Lab to provide 2 networked Linux VMs for HA multi-node certification. |
| **Disaster Recovery (DR) Test Environment** | **NEEDS USER/EXTERNAL ACCESS** | Requires independent second site / network | User/Lab to provide secondary DR VM instance or isolated network namespace. |
| **2,500-Device Benchmark Environment** | **NEEDS USER/EXTERNAL ACCESS** | Requires 8C/16G dedicated VM/host compute | Dedicated benchmark instance required for 24h certified test run (Day 18). |
| **Ubuntu 24.x Clean VM (Package Install)** | **NEEDS USER/EXTERNAL ACCESS** | Requires bare-metal or KVM/VirtualBox VM for `.deb` install, systemd autostart, host reboot testing | User/Lab to provide clean Ubuntu 24.04 VM with root access for packaging verification. |
| **RHEL 9.x Clean VM (Package Install)** | **NEEDS USER/EXTERNAL ACCESS** | Requires RHEL 9 VM for `.rpm` install, SELinux enforcement, systemd verification | User/Lab to provide clean RHEL 9 VM for `.rpm` packaging certification. |
| **Windows VM / Test Runner** | **NEEDS USER/EXTERNAL ACCESS** | Windows Service / MSI install and SCM reboot testing cannot run inside Linux Docker | User/Lab to provide Windows Server 2022 / Windows 11 VM with WinRM / PowerShell access. |
| **WinRM Target** | **NEEDS USER/EXTERNAL ACCESS** | Requires Windows instance listening on 5985/5986 | Part of the Windows test VM noted above. |
| **Redfish / IPMI Target** | **NEEDS CREATION** | Open-source `sushy-tools` emulator container | Add Redfish mock emulator on Day 6. |
| **VMware vCenter / ESXi Target** | **NEEDS USER/EXTERNAL ACCESS** | Proprietary hypervisor API | User/Lab to provide vCenter test credentials or nested ESXi VM. |
| **Hyper-V Target** | **NEEDS USER/EXTERNAL ACCESS** | Proprietary Microsoft hypervisor | Part of Windows Server test VM noted above. |
| **Proxmox / Nutanix Target** | **NEEDS USER/EXTERNAL ACCESS** | Hypervisor test instance | Lab Proxmox VE test instance for API connector testing. |
| **Storage / SAN Target (SMI-S / NetApp)** | **NEEDS USER/EXTERNAL ACCESS** | Proprietary storage array or simulator | Lab storage simulator or simulated REST endpoint. |
| **AWS Test Account** | **NEEDS USER/EXTERNAL ACCESS** | Cloud API polling (CloudWatch, EC2, VPC) | User to provide dedicated sandboxed AWS IAM credentials (read-only monitoring). |
| **Azure Test Subscription** | **NEEDS USER/EXTERNAL ACCESS** | Cloud API polling (Azure Monitor, VMs) | User to provide dedicated sandboxed Azure Service Principal. |
| **GCP Test Project** | **NEEDS USER/EXTERNAL ACCESS** | Cloud API polling (Cloud Monitoring, Compute) | User to provide dedicated sandboxed GCP Service Account JSON. |
| **Kubernetes Test Cluster** | **NEEDS CREATION** | Can run lightweight local `k3s` / `kind` in container | Can be created locally via `kind` / `k3s` container when K8s monitoring is implemented. |
| **SQL Server Target** | **NEEDS CREATION** | Can deploy Microsoft SQL Server Linux container | Add `mcr.microsoft.com/mssql/server` test container on Day 9. |
| **Oracle Database Target** | **NEEDS USER/EXTERNAL ACCESS** | Oracle license / heavy container requirements | User to provide Oracle test instance if certified under v1. |
| **ITSM Test Environment (ServiceNow / Jira)**| **NEEDS CREATION** | Local mock REST endpoint simulating incident APIs | Implement mock ITSM container on Day 8. |
| **Air-Gap / Offline Test Environment** | **NEEDS CREATION** | Isolated network bridge with outbound DNS/HTTP blocked | Docker network with `--internal` flag can validate air-gap installers on Day 25. |

---

## 3. Summary of Readiness

* **AVAILABLE (Immediate):** 6 items (Development stack, Core, Postgres, ClickHouse, Kafka, Valkey, mTLS).
* **NEEDS CREATION (Local Container Mocking):** 16 items (SNMP simulators, Flow generator, Syslog generator, SSH mock, Webhook receiver, SMTP mock, gNMI target, NETCONF target, MySQL, MongoDB, Redis, Redfish emulator, K8s kind, SQL Server, ITSM mock, Air-gap bridge).
* **NEEDS USER/EXTERNAL ACCESS (Hardware / VMs / Cloud):** 17 items (Ubuntu 24 VM, RHEL 9 VM, Windows VM, 2-Core HA network, DR network, 2500-device host, real SNMP switches, WinRM, vCenter, Hyper-V, Proxmox, SAN, AWS, Azure, GCP, Oracle, Enterprise ServiceNow).
* **NOT YET REQUIRED:** 0 items (All v1.0 scope items accounted for).

### Critical External Blockers for Customer Certification
To exit Day 30 and pass commercial GA, the lab or user must supply:
1. **Packaging VMs:** Clean Ubuntu 24.04 and RHEL 9.x VMs for `.deb`/`.rpm` install, systemd, and reboot validation.
2. **Windows Runner:** Windows Server 2022 / Windows 11 VM for MSI, Windows Service, and code-signing validation.
3. **High-Availability Cluster:** Two networked Linux instances with L2 connectivity for VIP/fencing failover testing.
4. **Benchmark Host:** One 8C/16G host for the 24-hour SNWP-2500 performance certification.
