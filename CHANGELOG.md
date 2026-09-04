# Changelog

## Unreleased

- Imported new-api baseline `3a9f41ee85cc369f5b8d7fe6e62ff4e7bf3a9ec8` while retaining upstream attribution and AGPL-3.0 terms.
- Added Desktop2Stereo device-code login, 30-day trial, multi-license device binding, online leases, offline ES256 entitlements, permanent binding, revoke workflows, region-aware orders, balances, invite rewards, chargeback handling, withdrawals, and admin APIs.
- Added source-build Docker Compose configuration, deployment guidance, API documentation, and SQLite/MySQL/PostgreSQL verification tests.
- Split the client and server implementation plans, consolidated production deployment into `docs/d2s.site.md`, made logout release Desktop2Stereo online leases, and added hourly cleanup for expired device codes and leases.
