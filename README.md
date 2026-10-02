# Build & Break

Local development hosting platform: React → Go Core → PostgreSQL jobs → authenticated bnb-agent → Docker/Moby.

A **Node** is a physical machine. A **Server** is a customer resource allocation on a node. A **Service** is a workload inside that allocation. Containers are isolated per service and share an enforced allocation budget in Core. This is not yet a VPS/hypervisor implementation.

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
go build -o bin/bnb-agent ./services/agent
npm ci --prefix apps/web
npm run build --prefix apps/web
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

## Use the platform

1. Home → **Create server** → choose Minecraft Java or Alpine, name/resources/location.
2. Core atomically creates the server allocation, its first service and a provisioning job.
3. The agent creates/starts the owned container. Core records its runtime identity and state.
4. **My servers → Manage server** shows resources and services. Start/Stop/Restart/Delete all enqueue real jobs.
5. Add more services under a server while memory/CPU budgets allow it. Each service reserves CPU/memory even while stopped.
6. Failed provisioning has a **Retry provisioning** button. It reuses the original deterministic container name only after matching all ownership labels.
7. Billing → create a **sandbox order**, then **Simulate payment and provision**. The ledger records an invoice/payment and queues a real service. Repeated payment requests do not create duplicate allocations.

Minecraft is Vanilla **1.21.1**, using `itzg/minecraft-server:java21`, explicit EULA acceptance, a validated/reserved host TCP port, and a managed named volume `bnb-service-<service-id>` mounted at `/data`. The image tag is mutable; pin a reviewed digest before production use. Connect a matching Minecraft Java client to the node's address and the selected port. `running` describes the container, not game readiness; initial downloads/world generation take longer. Deleting a service retains its world volume. No automatic volume deletion is implemented.

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
python3 scripts/lifecycle_e2e.py
python3 scripts/billing_e2e.py
python3 scripts/minecraft_e2e.py
```

Integration and mock-runtime tests need permission to open local sockets. End-to-end scripts need Core, agent, Docker and PostgreSQL running. They create their own development allocations. Alpine lifecycle tests delete only their own service container and retain audit history. Billing tests leave a stopped Alpine service; Minecraft tests leave a stopped Minecraft service and its world volume. `scripts/minecraft_e2e.py` uses port **25575** and writes its service IDs to `/tmp/bnb-minecraft-e2e.json`; `BNB_MINECRAFT_RESUME=1 python3 scripts/minecraft_e2e.py` retries that same failed test service.

## Development scope and remaining work

This is a **local development control plane**, not a production customer launch. Customer authentication, tenant authorization, a dedicated admin page, account management, console/log streaming, file manager/SFTP, backups, scheduled tasks, VPS provisioning, subscriptions/renewals, live payment collection, refunds/tax handling and production operational recovery are not implemented. Disk budgets are recorded but filesystem quotas are not enforced. Node resources show totals/reserved allocations rather than live per-container usage.

Billing requires explicit `BNB_BILLING_SANDBOX=1`, uses demo GBP integer amounts and makes **no payment-provider requests**. It cannot collect money. Sandbox payment and fulfillment commit together or roll back together. It is a foundation for integration, not WHMCS/Paymenter feature parity. Keep Core/Vite on localhost until customer/admin authorization is implemented.

Jobs serialize per service and results retry safely/idempotently. There is no durable agent result journal or automatic lease recovery after an agent dies while executing a job. Such jobs can remain `processing`; inspect the owned runtime and job before any manual recovery. No automatic requeue is enabled because a timed-out mutation may have already taken effect. An old unrelated `test` job was already processing before this work and was left untouched.

See [API contract](docs/api.md), [React handoff](docs/react-handoff.md), and [verified session status](docs/session-status.md).
