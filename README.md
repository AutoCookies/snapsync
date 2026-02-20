# SnapSync

SnapSync is a LAN file transfer CLI for reliable single-file transfers over TCP with discovery, resume, and end-to-end integrity checks.

## Requirements

- Go 1.22+

## Build

```bash
make build
```

## Quick Start (2 machines)

Machine A (receiver):

```bash
./bin/snapsync recv --out ./downloads --accept
```

Machine B (sender):

```bash
./bin/snapsync list --timeout 2s
./bin/snapsync send ./20GB.iso --to <peer-id>
```

## Commands

- `snapsync recv --listen :45999 --out <dir> [--accept] [--overwrite] [--no-discovery] [--no-resume] [--keep-partial] [--force-restart] [--break-lock]`
- `snapsync list [--timeout 2s] [--json]`
- `snapsync send <path> --to <peer-id|host:port> [--timeout 2s] [--name <override>] [--no-resume]`
- `snapsync version`

## Discovery (Phase 2)

Receivers advertise on `_snapsync._tcp.local` while running. `snapsync list` shows discovered peers with ID, name, addresses, port, and age.

## Integrity Verification (Phase 3)

SnapSync verifies transfer integrity before finalizing output.

Corrupted transfers fail and incomplete outputs are removed.

## Resume Transfers (Phase 4)

If a transfer is interrupted, SnapSync resumes automatically.

Partial transfers are stored as `*.partial` with a metadata file `*.partial.snapsync`.

On completion, SnapSync verifies integrity before finalizing the file.

## Troubleshooting

- **Discovery not working**: verify both hosts are on same subnet and multicast DNS is allowed by firewall.
- **Connection failures**: ensure receiver port is open and reachable.
- **Lock busy errors**: another transfer is using the same target; retry or use `--break-lock` if you are sure it is stale.
- **Integrity failures**: transfer was corrupted in transit or on disk; rerun send.

## Known Limitations

- No folder transfer yet (files only).
- No encryption/authentication yet.
- Discovery is intended for same-subnet LAN environments.
