# Build & Break product delivery

User direction, 2026-10-04: build a complete owned hosting platform, with the
complete customer hosting workflow, billing capabilities comparable to WHMCS /
Paymenter, and an owned Core/agent management backend comparable to Pterodactyl.
Both Stripe and PayPal are required. The full planned game and app catalogue (92 entries) and all Pterodactyl panel capabilities are explicit targets, alongside an Ubuntu base server and multiple services sharing the customer’s purchased allocation. Keep the development website running and
update it live. This is an ongoing product build; the foundation is not completion.

## Acceptance criteria

- Website: game/app catalogue, plans, locations, account, checkout, customer
  dashboard, billing, support, and responsive service management.
- Identity: customer/admin permissions, verified email, password recovery, MFA,
  session revocation, account management, team access and audit records.
- Billing: owned product catalogue, immutable price snapshots, Stripe and PayPal
  checkout and verified webhook handling, deduplication, recurring invoices,
  renewal, cancellation, overdue/grace policies, suspension/reactivation,
  upgrades/downgrades, credits/refunds, tax and invoice exports, and admin controls.
- Runtime: provisioning, stable ownership, lifecycle operations, live usage,
  console/logs and commands, safe file access/uploads, SFTP, backups/restores,
  schedules, configurable templates/game versions and mod/plugin support.
- Operations: durable execution results, safe interrupted-job recovery,
  enforced quotas, migrations, monitoring, deployment and restoration checks.
- Verify complete customer journeys with provider test environments and real
  managed containers before enabling real payment collection.

The implementation and branding belong to Build & Break.

## Current delivery

Implemented: accounts, password hashing, persisted expiring sessions, sign-up,
sign-in/sign-out, CSRF checks, authentication rate limits, customer resource and
invoice isolation, admin-only direct deployments/simulated payments, basic admin
account/infrastructure view, and a public location catalogue. Existing unassigned
development resources remain admin-only. No public admin auto-promotion exists.

All Go tests including isolated PostgreSQL integration and runtime tests passed.
Frontend production build and lint passed. Real Chromium verification passed for
registration, session persistence, customer isolation, status display, the complete
sandbox checkout wizard, invoice listing, support conversations and sign-out.

Status health checks and persisted incident/update history are implemented and live.
The public catalogue, locations, plans and marketing sections now use Core data.
Customer and admin support tickets support replies, closure and reopening.
Administrators can create/edit/deactivate plans; existing invoice snapshots remain
immutable and demo plans are excluded from live payment mode.

Stripe Checkout and PayPal Orders/capture adapters, Stripe signature checks and
PayPal remote webhook verification, receipt/event deduplication, and durable paid
fulfillment retries are implemented. Tests use local mock provider APIs; no merchant
credentials or public provider webhook delivery have been configured or verified.
Payments commit separately from provisioning so unavailable nodes retain paid
invoices and retry fulfillment. Minecraft billing can automatically allocate a game
port. Recurring subscriptions are still outstanding.

Real authenticated Alpine provisioning, start/stop/restart/delete and allocation
archival passed. Sandbox order/payment replay produced one running service and
left it stopped. The temporary test administrator was disabled and its session revoked.

Live development addresses:

- Frontend with Vite updates: http://172.24.50.244:5173
- Built frontend/Core: http://172.24.50.244:8080

These are private network addresses. Core, Vite and the existing enrolled agent
were started in this development session. No node was re-enrolled.

Multi-node onboarding now includes admin-issued, expiring, single-use enrollment
tokens and a working agent enrollment command. Wire-contract, credential file
permissions, overwrite refusal and cross-node job isolation tests passed. The
existing live agent identity is preserved. Additional physical hosts still need
to be supplied and installed; see [node setup](node-setup.md).

## Remaining and next

Merchant-account configuration and verification, recurring billing, the expanded
game installers and Ubuntu architecture, and the remaining acceptance criteria
above are outstanding. Live provider credentials,
webhook endpoints, business pricing, email delivery and a public HTTPS domain
are not configured. Never describe sandbox invoices as real payment collection.

Ubuntu scope awaits a user decision between an environment inside the allocation
and a full virtual machine, or both. See [game coverage](../internal/catalogue/games.json) for the
92-entry deployment checklist. It is an implementation roadmap, not a claim that all
92 games are launchable. Container allocations do not implement VM/hypervisor hosting.

Operator setup: register an account through the website, then explicitly grant
admin access with `go run ./cmd/account-admin --email your-registered-email` after
loading `.env`. Do not promote an arbitrary public signup automatically. End-to-end
lifecycle scripts now require `BNB_TEST_EMAIL` and `BNB_TEST_PASSWORD` for a test
admin account. Do not print passwords, session cookies, `.env`, or agent credentials.

## Management progress — 4 October 2026

The management workspace now includes concrete Java startup defaults and overrides, server-specific console commands, software/version controls, Modrinth plugin/mod/pack selection, owned-volume file operations and Bedrock addon installation/toggles. Bedrock deployment reserves its primary TCP port and sixteen UDP transport ports. Shared plans accept supported workloads independently and new purchases start at 4 GiB.

Files currently support 24 KiB per file and 100 entries per folder; broader transfers and pagination remain work. Purpur, Spigot and Quilt need runtime acceptance testing. Bedrock authenticated client joining remains unverified. See [handoff](handoff.md) for current contracts and the remaining delivery priorities.
