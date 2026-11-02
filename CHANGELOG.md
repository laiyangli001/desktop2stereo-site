# Changelog

## Unreleased

- Standardized production API, device approval, reconciliation, and email-link examples on the verified `d2s.site` hostname.

- Added a purchase quote confirmation step. Order creation now validates the confirmed amount, currency, and region and returns `409 quote_changed` without creating or debiting an order when the server quote changes.
- Made matching idempotent order retries return the original order even if current prices have changed; mismatched retries remain rejected.
- Added English and Chinese screen-reader status feedback while the purchase quote is loading.
- Serialized manual-unbind request creation and approval on the user profile row to enforce the lifetime one-approval limit under concurrent requests.
- Expanded the production drill with per-platform Windows/Linux/macOS authorization acceptance flows and evidence requirements.
- Imported new-api baseline `3a9f41ee85cc369f5b8d7fe6e62ff4e7bf3a9ec8` while retaining upstream attribution and AGPL-3.0 terms.
- Added Desktop2Stereo device-code login, 30-day trial, multi-license device binding, online leases, offline ES256 entitlements, permanent binding, revoke workflows, region-aware orders, balances, invite rewards, chargeback handling, withdrawals, and admin APIs.
- Added source-build Docker Compose configuration, deployment guidance, API documentation, and SQLite/MySQL/PostgreSQL verification tests.
- Split the client and server implementation plans, consolidated production deployment into `docs/d2s.site.md`, made logout release Desktop2Stereo online leases, and added hourly cleanup for expired device codes and leases.
