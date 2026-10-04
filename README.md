# Build & Break

Local development hosting platform: React → Go Core → PostgreSQL jobs → authenticated bnb-agent → Docker/Moby.

A **Node** is a physical machine. A **Server** is a customer resource allocation on a node. A **Service** is a workload inside that allocation. Containers are isolated per service and share an enforced allocation budget in Core. This is not yet a VPS/hypervisor implementation.

## Developer handoff

Start with [handoff notes](docs/handoff.md) and the [frontend guide](docs/react-handoff.md). For a fresh checkout, copy `.env.example` to `.env`, set a local database password in both database settings, then run `docker compose --env-file .env -f compose.dev.yml up -d`. This creates a new development database; do not replace an existing environment. Database contents, game volumes and node identities are not stored in Git.

## Run locally

Run commands from the repository root. Requires Go matching `go.mod`, PostgreSQL, Docker on the agent machine, Node 18+ and npm. The existing enrolled agent reads its permanent identity from `~/.config/build-and-break/agent.json`. **Do not re-enroll the development node.** Do not print that file or `.env`.

Build and migrate:

```bash
export GOCACHE=/tmp/bnb-go-cache
set -a
source .env
set +a
go build -o bin/bnb-migrate ./cmd/migrate
./bin/bnb-migrate
go build -o bin/bnb-core ./apps/api
npm ci --prefix apps/web
bash scripts/build.sh
```

Existing development migrations have already been applied and recorded. On a **fresh empty database**, the migration command applies 001 onward. On a legacy database without `schema_migrations`, inspect its schema before using `--baseline-through 006`. That flag records older files without executing them; never baseline an empty database. Migrations live in **database/migrations**, not the old root migrations folder. Each new migration executes in a transaction and its checksum is recorded.

Terminal 1 (Core):

```bash
set -a
source .env
set +a
BNB_BILLING_SANDBOX=1 ./bin/bnb-core
```

Terminal 2 (existing enrolled agent):

```bash
./bin/bnb-agent
```

Terminal 3 (React development):

```bash
npm run dev --prefix apps/web
```

Open **http://127.0.0.1:5173/**. Alternatively, after the frontend build, Core serves the built site at **http://127.0.0.1:8080/**. Vite proxies `/api` to Core. The browser never talks to the agent or Docker. Core defaults to `127.0.0.1:8080`; `BNB_CORE_ADDR` overrides its listener. Agent health is local-only on `127.0.0.1:9090`.

Do not run multiple copies of Core/agent on the same ports. Processes left running from development may need to be stopped first using their exact PIDs. Never stop unrelated services or containers.

## Accounts and live development

Development snapshot: 2026-10-04. See [product delivery](docs/product-delivery.md) for
current requirements and outstanding work. The live development frontend is
http://172.24.50.244:5173; Core and its built frontend are at
http://172.24.50.244:8080. These are private network addresses.

Register through the website. Customer resources and invoices are scoped to the
signed-in account. Direct development provisioning, infrastructure details and
sandbox payment simulation require administrator access. To grant a registered
operator account admin access, load `.env` and run:

```bash
go run ./cmd/account-admin --email your-registered-email
```

Existing development resources remain unassigned and accessible only to admins.
Frontend mutations send a session CSRF token. API scripts must authenticate through
`/api/v1/auth/login`, preserve its HttpOnly cookie and send `X-CSRF-Token` on mutations.
Lifecycle scripts use `BNB_TEST_EMAIL` and `BNB_TEST_PASSWORD` for a test admin.
Set `BNB_WEB_ORIGIN` to the exact Vite origin(s), separated by commas, when accessing Vite from another
machine. Set `BNB_SECURE_COOKIES=1` when Core is served over HTTPS.

## Use the platform

1. Home → **Create server** → choose Minecraft Java, Bedrock or Alpine, plan, name and location.
2. Review the order and create an unpaid invoice. In sandbox mode, an admin uses **Billing → Simulate payment and provision** to create the allocation, first service and job. Configured Stripe/PayPal payments commit first, then a durable worker provisions the owned allocation.
3. The agent creates/starts the owned container. Core records its runtime identity and state.
4. **My servers → Manage server** shows resources and services. Start/Stop/Restart/Delete all enqueue real jobs.
5. Add more services under a server while memory/CPU budgets allow it. Each service reserves CPU/memory even while stopped.
6. Failed provisioning has a **Retry provisioning** button. It reuses the original deterministic container name only after matching all ownership labels.
7. The create wizard records a **sandbox order**; an admin then uses **Billing → Simulate payment and provision**. The ledger records an invoice/payment and queues a real service. Repeated payment requests do not create duplicate allocations.

Minecraft defaults to Vanilla **1.21.1**. The management workspace supports Vanilla, Paper, Purpur, Spigot, Fabric, Quilt, Forge, NeoForge and Modrinth packs with publisher version feeds and resolved Java runtimes. It uses the managed `itzg/minecraft-server` image, explicit EULA acceptance, a validated/reserved host TCP port, and a managed named volume `bnb-service-<service-id>` mounted at `/data`. The image tag is mutable; pin a reviewed digest before production use. Connect a matching Minecraft Java client to the node's address and the selected port. `running` describes the container, not game readiness; initial downloads/world generation take longer. Deleting a service retains its world volume. No automatic volume deletion is implemented.

Alpine is a fixed `alpine:3.20` development workload. Arbitrary images, privileged mode, host networks, host mounts and Docker socket mounts are not accepted through the public API.

## Tests

```bash
gofmt -w apps/api cmd internal services/agent
go build ./...
go test ./...
# Opt-in integration tests apply every migration in isolated temporary schemas.
# They remove only their own test schemas afterwards.
BNB_TEST_DATABASE_URL="$DATABASE_URL" go test ./...
npm run build --prefix apps/web
npm run lint --prefix apps/web
# Optional browser smoke test (requires Chromium's system libraries):
# npx --prefix apps/web playwright install --with-deps chromium
# npm run test:e2e --prefix apps/web
# Operator-only combined smoke test: creates and revokes its temporary test admin.
python3 scripts/authenticated_e2e.py
# Alternatively, set BNB_TEST_EMAIL and BNB_TEST_PASSWORD for a test admin:
python3 scripts/lifecycle_e2e.py
python3 scripts/billing_e2e.py
python3 scripts/minecraft_e2e.py
```

Integration and mock-runtime tests need permission to open local sockets. End-to-end scripts need Core, agent, Docker and PostgreSQL running. They create their own development allocations. Alpine lifecycle tests delete only their own service container and retain audit history. Billing tests leave a stopped Alpine service; Minecraft tests leave a stopped Minecraft service and its world volume. `scripts/minecraft_e2e.py` uses port **25575** and writes its service IDs to `/tmp/bnb-minecraft-e2e.json`; `BNB_MINECRAFT_RESUME=1 python3 scripts/minecraft_e2e.py` retries that same failed test service.

## Development scope and remaining work

This is a **local development control plane**, not a production customer launch. Email verification, password recovery, MFA, full admin/account management, subuser permissions, realtime log streaming, SFTP, backups, scheduled tasks, VPS provisioning, subscriptions/renewals, live payment collection, refunds/tax handling and production operational recovery are not implemented. Disk budgets are recorded but filesystem quotas are not enforced. The workspace shows observed CPU and memory samples and bounded console snapshots. Minecraft commands use a managed input pipe. Disk and player measurements remain incomplete. Node connection addresses are configurable in Administration; join addresses use the allocated host port, rather than the container listener port.

The running preview uses `BNB_BILLING_SANDBOX=1`, demo GBP integer prices, and no external payment requests. Stripe/PayPal adapters require explicit test/live configuration; see [payment setup](docs/payment-setup.md). Test mode cannot collect real money. Live mode excludes demo plans. Sandbox payment and fulfillment are atomic; provider-confirmed payments commit separately and provisioning retries durably. Recurring billing is still unimplemented. The development preview now has customer/admin authorization. Live provider credentials and a public HTTPS deployment are still unconfigured.

Jobs serialize per service and results retry safely/idempotently. There is no durable agent result journal or automatic lease recovery after an agent dies while executing a job. Such jobs can remain `processing`; inspect the owned runtime and job before any manual recovery. No automatic requeue is enabled because a timed-out mutation may have already taken effect. An old unrelated `test` job was already processing before this work and was left untouched.

See [API contract](docs/api.md), [React handoff](docs/react-handoff.md), and [verified session status](docs/session-status.md).

Additional Linux hosts: see [node setup](docs/node-setup.md) for admin enrollment and persistent agent installation.

## Management tools

Files support browsing, text editing, upload/download, renaming, folders and deletion on owned Java/Bedrock volumes, including stopped servers. Individual files are limited to 24 KiB and folder listings to 100 entries. The agent requires `bnb-fs` beside its executable. Bedrock supports validated `.mcpack`/`.mcaddon` uploads up to 16 MiB, installation, and enable/disable for the default world. Encrypted packs, pack upgrades and removal are not implemented.

Shared resource plans start at 4 GiB and accept any supported workload that fits. Existing allocations and invoice snapshots retain their original resources. Startup shows the resolved Java launch command and supports container-scoped overrides. Bedrock uses the current NetherNet transport with one primary TCP port and sixteen reserved UDP ports. Status readiness and persistent restarts were verified; authenticated Bedrock client joining remains a separate acceptance check.
