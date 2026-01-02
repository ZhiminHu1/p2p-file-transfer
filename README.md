# P2P File Transfer

[English](README.md) | [简体中文](README.zh-CN.md)

A robust peer-to-peer (P2P) file transfer system implemented in Go. This system enables efficient file sharing and chunk-based data transfer between peers, utilizing a central server for peer discovery and mDNS for automatic service discovery.


## Features

- **mDNS Service Discovery**: Automatic discovery of the central server on local network
- **Peer Discovery**: Central server manages dynamic peer registration and tracking
- **Chunk-Based File Transfer**: Files divided into chunks for efficient parallel transfer
- **Concurrent Transfers**: Worker pool pattern for optimized download speed
- **File Integrity Verification**: Hash-based verification ensures data integrity
- **Interactive CLI**: User-friendly command-line interface with auto-completion
- **Heartbeat Mechanism**: Automatic peer health monitoring
- **Configurable Network**: Support for custom bind addresses and ports

## Installation

### Prerequisites

- Go 1.22.4 or higher

### Build from Source

```bash
# Clone the repository
git clone https://github.com/yourusername/p2p-file-transfer.git
cd p2p-file-transfer

# Build the executable
go build -o p2p-transfer ./cmd/p2p-transfer

# Or use make
make build
```

## Quick Start

### 1. Start the Central Server

```bash
# Default: listens on 0.0.0.0:8000
./p2p-transfer server

# Custom address
./p2p-transfer server --addr 192.168.1.100:8000

# Interactive mode with CLI shell
./p2p-transfer server --addr 0.0.0.0:8000 --interactive
```

**Server CLI Commands** (interactive mode):

| Command | Description |
|:--------|:-----------|
| `status` | Show server status and connected peers |
| `list peers` | List all connected peer addresses |
| `exit` | Stop server and exit |
| `help` | Show available commands |

### 2. Start Peer Nodes

```bash
# Terminal 1 - Peer with default settings
./p2p-transfer peer

# Terminal 2 - Another peer with custom address
./p2p-transfer peer --addr 127.0.0.1:8002

# With auto-discovery enabled (finds central server via mDNS)
./p2p-transfer peer --auto-discover

# Specify custom central server address
./p2p-transfer peer --server 192.168.1.100:8000

# Interactive mode
./p2p-transfer peer --interactive
```

**Peer CLI Commands** (interactive mode):

| Command | Description |
|:--------|:-----------|
| `status` | Show peer status and connection info |
| `list files` | List all registered files |
| `register <file>` | Register a file for sharing |
| `download <file-id>` | Download a file by ID |
| `exit` | Exit peer |

### 3. Register and Transfer Files

```bash
# Register a file for sharing (from peer CLI)
register /path/to/your/file.txt

# Download a file by its hash ID
download abc123def456 /path/to/output/
```

Or use command-line flags:

```bash
# Register file on startup
./p2p-transfer peer --register /path/to/file.txt

# Download file on startup
./p2p-transfer peer --download <file-id>
```

## Command Reference

### Global Options

| Flag | Short | Default | Description |
|:-----|:-----|:-------|:-----------|
| `--help` | `-h` | - | Show help message |

### Server Command

```bash
p2p-transfer server [flags]
```

| Flag | Short | Default | Description |
|:-----|:-----|:-------|:-----------|
| `--addr` | `-p` | `0.0.0.0:8000` | Address to listen on |
| `--interactive` | `-i` | `false` | Start in interactive mode |

### Peer Command

```bash
p2p-transfer peer [flags]
```

| Flag | Short | Default | Description |
|:-----|:-----|:-------|:-----------|
| `--addr` | `-a` | `127.0.0.1:8001` | Address for this peer to listen on |
| `--server` | `-s` | `127.0.0.1:8000` | Address of the Central Server |
| `--auto-discover` | - | `false` | Automatically discover Central Server via mDNS |
| `--register` | `-r` | - | Path to a file to register/seed immediately |
| `--download` | `-d` | - | File Hash ID to download immediately |
| `--interactive` | `-i` | `false` | Start in interactive mode |

## Usage Examples

### Scenario 1: Local Testing (Single Machine)

```bash
# Terminal 1: Start central server
./p2p-transfer server

# Terminal 2: Start first peer
./p2p-transfer peer --addr 127.0.0.1:8001

# Terminal 3: Start second peer
./p2p-transfer peer --addr 127.0.0.1:8002

# In Terminal 2, register a file
register ~/testfile.txt
# Note the file-id returned

# In Terminal 3, download the file
download <file-id> ~/downloads/
```

### Scenario 2: LAN Deployment

```bash
# Machine A (192.168.1.100): Start central server
./p2p-transfer server --addr 0.0.0.0:8000

# Machine B (192.168.1.101): Start peer with auto-discovery
./p2p-transfer peer --auto-discover

# Machine C (192.168.1.102): Start peer with manual server address
./p2p-transfer peer --server 192.168.1.100:8000
```

### Scenario 3: Using Interactive Mode

```bash
# Start peer in interactive mode
./p2p-transfer peer -i

# Inside the interactive shell:
> status
Peer Server Running on: 127.0.0.1:8001
Central Server: 127.0.0.1:8000 (Connected)
Connected Peers: 0

> register ./myvideo.mp4
[INFO] File registered successfully. File ID: a1b2c3d4e5f6...

> download a1b2c3d4e5f6 /home/user/downloads/
[INFO] Starting download: a1b2c3d4e5f6
```

## How It Works

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         P2P Network                             │
│                                                                  │
│   ┌──────────────┐      ┌──────────────┐      ┌──────────────┐ │
│   │    Peer A    │ ───> │ Central Srv  │ <─── │    Peer B    │ │
│   │  (Seeder)    │ <─── │  (Index)     │ ───> │  (Leecher)   │ │
│   └──────────────┘      └──────────────┘      └──────────────┘ │
│         │                                              │        │
│         └────────────── Direct P2P Transfer ──────────┘        │
│                     (Chunk-based)                              │
└─────────────────────────────────────────────────────────────────┘
```

### Service Discovery Flow

1. **Central Server** starts and broadcasts via mDNS
2. **Peer** with `--auto-discover` scans for the central server
3. **Peer** connects to central server and registers itself
4. **Peer** sends heartbeat every 5 seconds to maintain connection
5. **Peer** registers files with central server
6. **Peer** queries central server for file locations
7. **Peer** downloads chunks directly from other peers

### File Transfer Process

```
1. Seeder registers file
   └→ File split into chunks (4MB each)
   └→ Each chunk hashed and stored
   └→ Metadata sent to Central Server

2. Leecher requests file
   └→ Query Central Server for File ID
   └→ Get list of peers hosting chunks
   └→ Download chunks in parallel using worker pool

3. Chunks assembled
   └→ Verify file integrity
   └→ Reassemble original file
```

## Project Structure

```
p2p-file-transfer/
├── cmd/
│   └── p2p-transfer/          # Main CLI application
│       ├── main.go            # Entry point
│       ├── root.go            # Root command
│       ├── server.go          # Server command
│       └── peer.go            # Peer command
├── central-server/            # Central server implementation
│   └── cserver.go             # Server logic with mDNS advertising
├── peer/                      # Peer implementation
│   ├── peer.go                # Peer server logic
│   ├── logic.go               # File handling logic
│   ├── workerpool.go          # Concurrent download workers
│   └── store.go               # Chunk storage management
├── pkg/
│   ├── discovery/             # mDNS service discovery
│   │   ├── discovery.go       # Advertiser and Resolver
│   │   └── discovery_test.go  # Unit tests
│   ├── logger/                # Logging utilities
│   ├── protocol/              # Message definitions
│   ├── storage/               # File storage and chunking
│   └── transport/             # Network transport layer
│       ├── tcp_transport.go   # TCP implementation
│       └── transport.go       # Transport interface
└── README.md
```

## Configuration

### Default Ports

| Component | Default Port |
|:----------|:------------|
| Central Server | 8000 |
| Peer | 8001 |

### mDNS Configuration

- **Service Type**: `_p2p-transfer._tcp`
- **Domain**: `local.`
- **Discovery Timeout**: 5 seconds

## Troubleshooting

### Peer cannot connect to Central Server

1. Check if central server is running:
   ```bash
   # Use netstat or lsof to check if port is listening
   netstat -an | grep 8000
   ```

2. Verify firewall settings allow traffic on the port

3. Try using `--auto-discover` flag to find the server via mDNS

### mDNS discovery not working

- Ensure all devices are on the same network segment
- Check if multicast is enabled on your router
- Use manual `--server` address as fallback

### File download fails

1. Verify the peer hosting the file is online
2. Check available disk space
3. Review logs for error messages

## Development

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./pkg/discovery/
```

### Build for Different Platforms

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o p2p-transfer-linux ./cmd/p2p-transfer

# macOS
GOOS=darwin GOARCH=amd64 go build -o p2p-transfer-mac ./cmd/p2p-transfer

# Windows
GOOS=windows GOARCH=amd64 go build -o p2p-transfer.exe ./cmd/p2p-transfer
```

## Roadmap

- [ ] Peer-to-peer service discovery (mDNS between peers)
- [ ] Relay server for cross-network discovery
- [ ] Resume interrupted downloads
- [ ] End-to-end encryption
- [ ] Directory transfer support
- [ ] Web UI
- [ ] Mobile applications

## Contributing

Contributions are welcome! Please follow these steps:

1. Fork the repository
2. Create a new branch: `git checkout -b feature-branch-name`
3. Make your changes and commit them: `git commit -m 'Add some feature'`
4. Push to the branch: `git push origin feature-branch-name`
5. Submit a pull request

## License

This project is licensed under the MIT License.

## Acknowledgments

This project is inspired by and based on [tarunkavi/p2p-file-transfer](https://github.com/tarunkavi/p2p-file-transfer).

Special thanks to the following open-source projects:

- [github.com/grandcat/zeroconf](https://github.com/grandcat/zeroconf) - mDNS service discovery library
- [github.com/spf13/cobra](https://github.com/spf13/cobra) - CLI framework
- [github.com/c-bata/go-prompt](https://github.com/c-bata/go-prompt) - Interactive prompt library
