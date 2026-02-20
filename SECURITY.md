# Security Policy

SnapSync currently does **not** provide transport encryption or peer authentication.

Current security model:
- LAN-oriented usage.
- Integrity verification at transfer completion.
- Local filesystem safeguards (locks, resume metadata, partial file isolation).

Use SnapSync only on trusted networks until encryption/authentication are introduced.
