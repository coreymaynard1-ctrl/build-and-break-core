# Development session status — 2026-10-02

Feature work stopped at the user's request. This document distinguishes verified behavior from remaining work.

## Implemented

- Preserved the pre-existing uncommitted provisioning queue/agent changes and existing enrollment.
- Canonical SQL migrations in `database/migrations`; runtime identity migration 007 relocated from the untracked root folder without losing its contents, made safe for the already-existing column/index.
- Additive lifecycle and sandbox billing migrations 008/009; checksum ledger and migration runner.
- Persistent runtime identity; authenticated create results; transactional job/service state updates and idempotent result replay.
- Owned-container start/stop/restart/delete jobs with managed + exact service/server label checks, immutable-ID targeting, resource limits, no privileged containers/host networks/host mounts/socket mounts.
- Service reconciliation, legacy identity recovery, stale revision protection, provisioning retry using deterministic names and ownership verification.
- Node/resource/runtime listing; server/service/job/template APIs; validation, transactional allocation limits, per-node port reservations, per-service active job constraint.
- Customer create-server transaction: allocation + first service + provisioning job.
- Fixed Alpine and Minecraft Java templates. Minecraft has a named world volume, configured TCP port, EULA validation and memory headroom for the JVM.
- Basic website, game/app create flow, My Servers list, per-server service panel, lifecycle controls, retry and job feedback, infrastructure details, reusable Core client and components.
- Sandbox orders, immutable price/resource snapshots, invoices, unique simulated payments, idempotent requests, atomic payment/fulfillment; basic billing page. **No live payment provider or recurring billing.**
- Empty server allocation archival in source/built Core; audit history retained and reserved capacity released.

## Passed verification

- `gofmt`, `go build ./...`, Go unit/runtime ownership tests.
- PostgreSQL integration tests in isolated schemas: every migration on a fresh schema, transactional/idempotent results, stale observation protection, concurrent service resource limits, lifecycle request conflicts, billing idempotency and fulfillment rollback.
- Applied migrations to the inspected existing development database; a second migration run made no changes.
- Frontend dependency installation, production build and configured lint; dependency audit reported zero vulnerabilities after updating Vite to 6.4.3.
- Real Alpine lifecycle: Core → PostgreSQL → authenticated agent → Docker → create/running, stop, start, restart, delete. Identity remained stable and all five jobs completed.
- Existing managed Alpine container was not recreated or stopped. Its runtime identity/state was recovered from the known create-job name with label checks.
- Real sandbox billing: unpaid invoice, duplicate order replay, conflicting-key rejection, repeated simulated-payment replay, one real service created and running, then stopped through the lifecycle queue.
- Local website HTTP and same-origin Vite proxy to Core configuration/templates.
- Minecraft: free image pulled, owned container created; its first Docker create request timed out after 30 seconds even though Docker created the container. After extending the timeout and adding retry/recovery, the same owned container was reused, started, answered a real Minecraft **1.21.1** protocol handshake, and was stopped through Core. The named world volume remains.

## Unverified or incomplete

- Full browser smoke test could not launch Chromium because `libnspr4.so` is missing. No browser pass is claimed. Playwright test is included for a properly configured development machine. Free browser binaries were downloaded to `/tmp/bnb-playwright`; no more system packages were installed after the stop request.
- The Minecraft test script originally failed when formatting a string description as a dictionary, after successful game readiness. That formatting issue is fixed. Minecraft-specific start/restart/delete after readiness were not completed today; generic Alpine lifecycle coverage passed.
- Final server archival route is compiled but was added after the last running Core restart. Restart Core from the built binary before testing that route.
- No dedicated admin page yet; only infrastructure details within My Servers.
- Public create still uses explicit development resource selection and direct local provisioning; the sandbox order/checkout workflow is a separate Billing page. A unified plan/location/checkout wizard remains frontend/product work.
- No customer authentication, tenant authorization, accounts, VPS/hypervisor integration, consoles/log streaming, file manager/SFTP, backups, scheduler, mods/plugins/player controls, subscription renewal, refunds or tax workflow.
- No filesystem disk quotas, durable agent result journal, or automatic recovery of jobs left processing after agent/process loss. An unrelated old test job already in processing was left untouched.
- This is a local development platform, not a fully working Server.pro/WHMCS/Paymenter replacement.

## Review artifacts

Existing preserved node: `c6076de5-90d4-4842-b903-df48d453637c`.
Existing preserved allocation: `1d1029df-f543-467f-84c4-20d7759a0622`.
Existing preserved service: `d2bbe608-8644-44c7-9ddd-18615c6834c4`.
Existing preserved container: `bnb-first-provisioned-service`.

New stopped sandbox-billed Alpine service: `b2ad5491-6413-4636-81dc-1448d1b8b958`.
Sandbox invoice: `c3476a43-06b9-4067-b673-a6e2752afea7`.
New stopped Minecraft service: `2ef5b261-21a1-4c3d-8f7a-46b73272bfa3`.
Minecraft allocation ID is available in the local service listing or `/tmp/bnb-minecraft-e2e.json`.
Minecraft test connection port: `25575`.

No secrets were printed, node re-enrollment performed, unrelated containers deleted, or existing PostgreSQL data destroyed. Test-generated service/allocation/job history remains for review.
