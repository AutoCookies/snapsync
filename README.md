# SnapSync

SnapSync is a LAN file transfer CLI focused on reliable local-network file sending with a simple sender/receiver workflow.

## Requirements

- Go 1.22+

## Build

```bash
make build
```

## Test

```bash
make test
make test-race
make lint
```

## Run

Start a receiver with discovery enabled (default):

```bash
./bin/snapsync recv --listen :45999 --out ./downloads --accept --name LivingRoomPC
```

Discover peers on the LAN:

```bash
./bin/snapsync list --timeout 2s
```

Send by discovered peer ID:

```bash
./bin/snapsync send ./movie.mkv --to a1b2c3d4e5 --timeout 3s
```

Direct host:port mode still works:

```bash
./bin/snapsync send ./movie.mkv --to 192.168.1.10:45999
```

Check version/build metadata:

```bash
./bin/snapsync version
```

## Integrity Verification (Phase 3)

SnapSync now verifies file integrity using BLAKE3.

All transfers are cryptographically validated before completion.

Corrupted files are automatically deleted.

## Resume Transfers (Phase 4)

If a transfer is interrupted, SnapSync resumes automatically.

Partial transfers are stored as `*.partial` with a small metadata file.

On completion, SnapSync verifies BLAKE3 integrity before finalizing the file.
