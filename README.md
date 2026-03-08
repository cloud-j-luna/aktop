# aktop

A terminal UI for monitoring consensus and provider operations.

Displays real-time consensus data, validator voting status, and provider upgrade status with CPU/memory stats.

![DE1BDA77-C915-41D6-9018-E351D1204F58_1_105_c](https://github.com/user-attachments/assets/5e9aca6e-c4d1-47d2-b795-41e8417a1c3b)

## Features

- Real-time consensus state monitoring (height, round, step)
- Validator voting status visualization (prevotes/precommits)
- Provider version distribution and resource monitoring (CPU/memory)
- Smart caching for provider data with priority-based scheduling
- Multiple viewing tabs (Overview, Validators, Providers)

## Installation

### Quick Install (Linux/macOS)

```sh
curl -sSL https://raw.githubusercontent.com/cloud-j-luna/aktop/main/install.sh | sh
```

### Using Go

Requires Go 1.26 or later.

```sh
go install github.com/cloud-j-luna/aktop/cmd/aktop@latest
```

### From Source

```sh
git clone https://github.com/cloud-j-luna/aktop.git
cd aktop
go build -o aktop ./cmd/aktop
```

### Download Binary

Download pre-built binaries from the [releases page](https://github.com/cloud-j-luna/aktop/releases).

Available for:
- Linux (amd64, arm64)
- macOS (amd64, arm64)

## Usage

```sh
# Use default Akash RPC
aktop

# Use custom RPC endpoint
aktop https://akash-rpc.polkachu.com

# Fast polling mode (250ms interval)
aktop --fast

# Custom refresh interval
aktop --refresh 2s

# Clear cache and start fresh
aktop --clean-cache
```

### Command Line Options

| Flag | Description | Default |
|------|-------------|---------|
| `--rpc` | RPC endpoint URL | `https://rpc.akt.dev/rpc` |
| `--rest` | REST endpoint URL | `https://rpc.akt.dev/rest` |
| `--refresh` | Refresh interval | `1s` |
| `--fast` | Fast polling mode (250ms) | `false` |
| `--clean-cache` | Delete provider cache | `false` |

### Keyboard Controls

| Key | Action |
|-----|--------|
| `q`, `Ctrl+C` | Quit |
| `r` | Manual refresh |
| `Tab`, `1`, `2`, `3` | Switch between tabs |
| `j`/`k`, `Up`/`Down` | Scroll lists |
| `h`/`l`, `Left`/`Right` | Cycle through provider versions |
| `g`, `G` | Jump to top/bottom of list |

### Tabs

1. **Overview** - Consensus state, vote progress, validator grid
2. **Validators** - Detailed validator list with vote status
3. **Providers** - Provider version distribution and list

## Cache

Provider data is cached in `~/.aktop/providers.json` to improve startup time and reduce network requests.

The cache uses smart scheduling:
- Online providers: checked every 1 minute
- Recently offline providers: checked every 5 minutes
- Long-term offline providers: checked every 6 hours
- Chain sync: every 10 minutes

## Development

### Requirements

- Go 1.26 or later

### Build

```sh
go build -o aktop ./cmd/aktop
```

### Run

```sh
go run ./cmd/aktop
```

### Test

```sh
go test ./...
```
