# Changelog

## v1.0.0

### Features
- TCP file transfer with streaming progress output.
- mDNS LAN discovery (`recv` advertise + `list` browse + peer-id send resolution).
- End-to-end integrity verification on completion.
- Crash-safe resume with `.partial` + atomic metadata.
- Session guard and local lock files to prevent conflicting concurrent writes.

### Operational behavior
- Single-writer safety via `*.partial.lock`.
- Resume guarded by session identity.
- Safe finalize only after integrity verification.

### Limitations
- Single file transfers only.
- No encryption/authentication.
- No parallel chunking.
- Discovery assumes same-subnet multicast reachability.
