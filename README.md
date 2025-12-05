# AoxEngine Go Port

This project is a high-performance Go implementation of the AoxEngine Real-Time Location System (RTLS) core. It processes UWB and BLE sensor data to calculate tag positions using an Extended Kalman Filter (EKF) and broadcasts results to external systems (RBC) and a modern web visualization interface.

## Features

* **Core Engine:**
  * High-throughput UDP Server for receiving sensor data (UNIB protocol).
  * Fusion Pipeline implementing EKF for accurate positioning.
  * Support for TWR (UWB) and RSSI (BLE) measurements.
  * Protocol parsing for various frame types (TWR, RSSI, IMU, LoRa Uplink).
* **Data Management:**
  * **PCAP Support:** Read and write compatible `AoxEngine` binary log (PCAP) files.
  * **Replay Mode:** Simulate historical data at configurable speeds (1x, 2x, 100x, etc.) while maintaining physics integrity.
* **Interfaces:**
  * **RBC Output:** Broadcasts position and status updates to external platforms via UDP/TCP.
  * **Web Interface:** Built-in HTTP server with a WebSocket API streaming real-time positions.
  * **3D Visualization:** React + Three.js frontend for real-time monitoring on floor plans.

## Directory Structure

* `cmd/`: Entry points for executables.
  * `udp_server`: Main engine application.
  * `replay`: Standalone UDP packet replayer.
  * `verify_pcap`: Tool to compare PCAP binary content.
  * `rbc_sender`: Test tool for RBC protocol.
* `fusion/`: Core algorithms (EKF, Layer Manager, RSSI model).
* `server/`: UDP server logic and protocol parsing.
* `rbc/`: External interface (Remote Broadcast) logic.
* `web/`: HTTP and WebSocket handlers.
* `binlog/`: PCAP parsing and writing.
* `frontend/`: React/Vite web application source.

## Build Instructions

### Prerequisites

* Go 1.22+
* Node.js & npm (for frontend)

### 1. Build Frontend

```bash
cd frontend
npm install
npm run build
cd ..
```

### 2. Build Backend

```bash
# Install dependencies
go mod tidy

# Build main server
go build -o udp_server ./cmd/udp_server

# Build tools (optional)
go build -o replay_tool ./cmd/replay
go build -o verify_pcap ./cmd/verify_pcap
```

## Usage

### Main Server (`udp_server`)

The server can run in **Live Mode** (listening for UDP packets) or **Replay Mode** (processing a PCAP file).

#### Flags

* `-project <path>`: Path to `project.xml` (required).
* `-wogi <path>`: Path to `wogi.xml` (required).
* `-port <int>`: UDP listen port (default 44333).
* `-http <int>`: HTTP/WebSocket port (e.g., 8080). Set to 0 to disable.
* `-web-root <path>`: Path to frontend static files (default "frontend/dist").
* `-replay <path>`: Path to input PCAP file for simulation.
* `-speed <float>`: Replay speed multiplier (default 1.0).
* `-loop <bool>`: Loop replay indefinitely (default false).
* `-pcap <path>`: Output path to record valid packets to a new PCAP file.

#### Examples

**1. Run Live Server with Web UI:**

```bash
./udp_server -project config/project.xml -wogi config/wogi.xml -http 8080
```

* Access the dashboard at `http://localhost:8080`.
* WebSocket stream available at `ws://localhost:8080/ws`.

**2. Replay PCAP with Web UI (Looping Indefinitely):**
This command runs the replay at 4x speed and serves the frontend interface, continuously looping the PCAP file.

```bash
./udp_server \
  -project ../ruigao/project.xml -wogi ../ruigao/wogi.xml \
  -http 8080 \
  -web-root frontend/dist \
  -replay ../binlog/PKTSBIN_20251204103122343_1105_1121.pcap \
  -speed 4.0 \
  -loop
```

**3. Record Live Traffic:**

```bash
./udp_server -project config/project.xml -wogi config/wogi.xml -pcap captures/
```

*(If a directory is provided, a timestamped filename is generated automatically)*

---

### Ancillary Tools

#### `replay` (Standalone)

Reads a PCAP file and sends raw UDP packets to a target destination. Unlike the server's internal replay, this sends packets over the network loopback.

```bash
./replay_tool -pcap input.pcap -dest 127.0.0.1:44333 -speed 2.0
```

#### `verify_pcap`

Compares the payload of two PCAP files to verify data integrity (ignoring timestamps).

```bash
./verify_pcap -1 original.pcap -2 recorded.pcap
```

#### `rbc_sender`

Test utility to generate RBC protocol messages.

```bash
./rbc_sender -udp 127.0.0.1:5000 -tcp 127.0.0.1:6000
```

## Configuration

The engine relies on `project.xml` and `wogi.xml` for:

* **Anchor Coordinates:** Defined in `<anchorlist>`.
* **Layer/Map Info:** Defined in `<viewerSettings>` and `<groups>`.
* **RBC Destinations:** Defined in `<txlist>`.

The server automatically parses `<txlist>` to forward position data to configured UDP/TCP endpoints (e.g., "RBCC" type).
