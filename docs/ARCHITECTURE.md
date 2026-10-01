# RedWolf System Architecture

## 1. Component Overview

The **RedWolf** platform is organized around a modular architecture separating network orchestration, discovery runtime, core scheduling, and user interface:

```text
+-----------------------------------------------------------------------------------+
|                                  REDWOLF CORE                                     |
|                                                                                   |
|  +--------------------+   +---------------------+   +--------------------------+  |
|  |    DHCP Service    |   |     TFTP Service    |   |     HTTP Boot / Assets   |  |
|  | (Provisioning Net) |   |    (iPXE Kernel)    |   | (vmlinuz, initramfs, OS) |  |
|  +--------------------+   +---------------------+   +--------------------------+  |
|                                                                                   |
|  +-----------------------------------------------------------------------------+  |
|  |                               Core API Engine                               |  |
|  |  - Hardware Inventory Repository                                            |  |
|  |  - Node Finite State Machine (Discovered -> Ready -> Provisioning -> Active)|  |
|  |  - Cloud-Init Generator (user-data, meta-data, network-config v2)           |  |
|  +-----------------------------------------------------------------------------+  |
+----------------------------------------|------------------------------------------+
                                         |
                       +-----------------+-----------------+
                       |                                   |
                       v                                   v
        +----------------------------+      +----------------------------+
        |        REDWOLF UI          |      |     REDWOLF DISCOVERY      |
        |      (Web Dashboard)       |      |     (In-Memory Agent)      |
        | - Node table & live state  |      | - dmidecode, lscpu, ip     |
        | - Hardware visualization   |      | - ipmitool KCS (iDRAC conf)|
        | - OS Provisioning Wizard   |      | - JSON Telemetry Reporter  |
        +----------------------------+      +----------------------------+
```

---

## 2. Responsibilities & Data Flow

### 2.1 `redwolf-core`
- **DHCP / TFTP Orchestrator:** Controls the isolated provisioning network domain. Automatically identifies new nodes and serves network boot files.
- **Node Finite State Machine (FSM):**
  - `DISCOVERED`: Network boot detected on the provisioning segment.
  - `COLLECTING_TELEMETRY`: In-memory discovery agent running; collecting hardware data and configuring BMC.
  - `READY_FOR_PROVISIONING`: Hardware cataloged, BMC secured; awaiting operator deployment parameters.
  - `PROVISIONING`: Target storage partitioned; OS image deployed and Cloud-Init injected.
  - `ACTIVE`: Deployment finalized, server booted into production OS.
  - `FAILED`: Provisioning encountered an error.

### 2.2 `redwolf-discovery`
- Ultra-lightweight in-memory Linux micro-OS (Alpine/Buildroot kernel + minimal userspace tools).
- Tools: `ipmitool`, `dmidecode`, `ethtool`, `util-linux`, `curl`, `jq`.
- Agent script execution cycle:
  1. Loads hardware IPMI kernel drivers (`modprobe ipmi_si`, `modprobe ipmi_devintf`).
  2. Extracts system hardware inventory (CPU, RAM, platform DMI, NIC MACs, storage drives).
  3. Configures BMC administrative credentials locally via KCS.
  4. Enables DHCP on the BMC management NIC and queries the assigned IP.
  5. Posts full JSON telemetry to RedWolf Core API and transitions to listening mode.

### 2.3 `redwolf-ui`
- Modern, responsive web application for data center technicians and infrastructure engineers.
- Designed with enterprise-grade dark theme by default, featuring official RedWolf SVG branding.
- Real-time device matrix displaying live nodes, power states, and deployment progress.
- Server detail view:
  - Deep telemetry visualizer (CPU topology, memory channel utilization, NIC table).
  - Direct deep-link to iDRAC web console (`https://<BMC_IP>`).
  - Streamlined deployment wizard for AlmaLinux (8/9/10) and Debian (12/13).
