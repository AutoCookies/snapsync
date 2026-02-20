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

Start a receiver:

```bash
./bin/snapsync recv --listen :45999 --out ./downloads --accept
```

Send a file:

```bash
./bin/snapsync send ./big.iso --to 192.168.1.10:45999
```

Check version/build metadata:

```bash
./bin/snapsync version
```
