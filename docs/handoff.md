# Developer handoff

Development snapshot: 4 October 2026. Build & Break is a working development hosting platform with a React frontend, Go control plane, PostgreSQL ledger/job queue and independent Go node agents using Docker. It is not ready for a public customer launch.

## Start here

Clone the repository, copy `.env.example` to `.env`, and set your own local database password in `DATABASE_URL` and `BNB_DEV_DB_PASSWORD`. Use Go matching `go.mod`, Node/npm, Docker and Docker Compose.

```bash
docker compose --env-file .env -f compose.dev.yml up -d
npm ci --prefix apps/web
bash scripts/build.sh
set -a
source .env
set +a
./bin/bnb-migrate
./bin/bnb-core
```

In another terminal, run `npm run dev --prefix apps/web`; open http://127.0.0.1:5173. Register an operator, then load `.env` and run `./bin/bnb-account-admin --email YOUR-REGISTERED-EMAIL`. Use Administration to enroll a fresh node following [node setup](node-setup.md). Start its agent with `bnb-fs` installed beside it. An existing enrolled node must keep its identity; do not enroll it again.

Core also serves the built frontend on port 8080. Vite proxies `/api` to Core. Database contents, game worlds, provider credentials and node identities are environment state and are excluded from the repository. A checkout alone does not reproduce the existing live servers.

## Working functionality

- Account registration, sessions, CSRF protection and customer/admin authorization.
- Shared resource plans with a 4 GiB purchase minimum, immutable invoices, administrator sandbox payment and durable fulfillment. Stripe/PayPal adapters exist; merchant delivery remains unverified.
- Independent node enrollment, resource/port reservation and owned container start/stop/restart/delete operations.
- Minecraft Java software/version settings, Modrinth plugin/mod/pack selection, console commands, observed CPU/memory and editable startup commands.
- Bedrock latest-version deployment with reserved NetherNet transport ports; validated addon installation and enable/disable.
- Java/Bedrock persistent-volume file browsing, editing, uploads/downloads, folders, rename and deletion, including stopped servers.
- Public measured status/incident history and customer/admin support conversations.

Paper, Fabric, Modrinth packs, console commands, startup overrides, Bedrock readiness/restart, addon toggles and file operations have real runtime checks. Purpur, Spigot and Quilt are connected to configuration/version flows but still need dedicated deployment acceptance tests. Bedrock authenticated client joining also needs acceptance testing.

## Contracts to preserve

A node is a machine; a server is a resource allocation; a service is a workload within it. Allocation resources remain reserved when services stop. New purchases use common plans and separately select a workload. The first workload currently consumes the plan resources; resizing and resource redistribution need further work.

The browser calls Core only. Mutations send the session CSRF token. Server ownership, billing prices, node credentials and Docker ownership labels are enforced by Go. A queued HTTP 202 response is not completion: poll the returned job and show its final error or result. Container running is distinct from game readiness. Deletion preserves game volumes.

File operations are limited to 24 KiB per file and 100 entries per folder. Addon uploads are limited to 16 MiB compressed and target the default Bedrock world. These limits are visible in the UI. No host paths, Docker credentials or arbitrary privileged workloads should enter frontend contracts.

## Next priorities

1. Expand file transfer limits with streaming/chunking and paginated folders; retain volume confinement and ownership checks.
2. Complete admin user management and service subuser permissions in Core before enabling access-sharing controls.
3. Add remaining game installers and game-specific management with real version feeds, port contracts and lifecycle tests. The catalogue marks planned entries explicitly.
4. Implement backup/scheduler/SFTP operations, durable agent result recovery and disk quotas.
5. Configure email and DNS/subdomain operations once domain/provider details are available.
6. Verify merchant test-mode callbacks, then add recurring billing, renewals and production operational controls.

Full Ubuntu VPS provisioning, plan changes, node migration, password recovery/MFA and the broader game catalogue remain unfinished. Disabled navigation entries must remain disabled until their backend operations work.

## Validation

Run `go test ./...`, `npm run lint --prefix apps/web` and `npm run build --prefix apps/web`. Set `BNB_TEST_DATABASE_URL` for isolated database integration tests. Browser tests use the actual API; management tests require dedicated test-service IDs. Runtime scripts in `scripts/` create or use dedicated development services and must not target customer workloads. See [frontend guide](react-handoff.md), [API](api.md) and [payment setup](payment-setup.md).
