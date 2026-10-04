# Core API contract

Base: `/api/v1`. Browser requests go only to Core. JSON request bodies for the new endpoints reject unknown fields, trailing JSON and bodies over 64 KiB. New validation/operation errors use `{"error":"message"}`; existing enrollment/heartbeat routes may return plain-text errors. IDs are UUIDs. Memory and disk budgets use MiB; node total resources and create-container payload memory use bytes. Times are RFC3339 timestamps.

## Browser/development endpoints

| Method | Path | Behavior |
| --- | --- | --- |
| GET | `/health` | Core health |
| GET | `/nodes` | Nodes, calculated online state, runtime readiness and reserved capacity |
| GET | `/templates` | Fixed templates and required memory/EULA/port metadata |
| GET | `/servers` | Non-archived server allocations |
| POST | `/servers` | Allocate resources on an online/runtime-ready node; 201 |
| DELETE | `/servers/{id}` | Archive an empty allocation and release its budget; preserve history; 200 |
| GET | `/services?server_id=UUID` | Services, runtime identity, observed state/time, errors, latest job status; optional filter |
| POST | `/services` | Add a service to an existing allocation and enqueue creation; 202 |
| POST | `/deployments` | Atomically allocate server + first service + job; 202 |
| POST | `/services/{id}/start` | Enqueue `start_service`; 202 |
| POST | `/services/{id}/stop` | Enqueue `stop_service`; 202 |
| POST | `/services/{id}/restart` | Enqueue `restart_service`; 202 |
| POST | `/services/{id}/retry` | Retry failed provisioning using the original create payload; 202 |
| DELETE | `/services/{id}` | Enqueue owned-container stop/removal; 202 |
| POST | `/services/{id}/delete` | Alias for DELETE service; 202 |
| GET | `/jobs?service_id=UUID` | Latest 50 jobs for a service; no job payload secrets returned |
| GET | `/billing/config` | Mode and provider availability; no credentials |
| GET | `/billing/plans` | Active plans; live mode excludes demo prices |
| GET | `/billing/invoices` | Latest 100 owned invoices and fulfillment state |
| POST | `/billing/orders` | Snapshot plan and create unpaid invoice; 201, replay 200 |
| POST | `/billing/invoices/{id}/pay-sandbox` | Simulate payment, allocate server/service and enqueue provisioning atomically; 200 |

Customer resource and invoice endpoints require a session and enforce tenant ownership. Direct allocations/deployments, raw node infrastructure, simulated payments and admin endpoints require an administrator. Templates, locations, billing configuration/plans and service status are public. Agent endpoints use separate permanent node credentials.

### Create allocation

```json
{"node_id":"c6076de5-90d4-4842-b903-df48d453637c","name":"My server","memory_mb":2048,"disk_mb":4096,"cpu_threads":2}
```

Returns `{"id":"UUID","status":"active"}`. Name must be nonblank, at most 100 bytes. Memory 32–1048576 MiB, disk 128–1073741824 MiB, CPU 1–256; all must fit unallocated node capacity.

### Create service

```json
{"server_id":"UUID","name":"Minecraft","template":"minecraft-java","memory_mb":2048,"cpu_threads":2,"port":25565,"accept_eula":true}
```

Alpine uses `template:"alpine-test"`, minimum 32 MiB, and no EULA/port fields. Minecraft requires at least 1536 MiB, `accept_eula:true`, and port 1024–65535. Service budgets must fit the server; port reservations are unique per node. Legacy services with unknown resource limits conservatively consume the server budget.

Returns `{"id":"service UUID","job_id":"UUID","status":"installing"}`.

### Create server and first service

`POST /deployments` accepts `node_id`, `name`, `template`, `memory_mb`, `disk_mb`, `cpu_threads`, optional `port`, and optional `accept_eula`. Same validation as above. Returns:

```json
{"server_id":"UUID","service_id":"UUID","job_id":"UUID","status":"installing"}
```

### Lifecycle and states

Lifecycle requests have no JSON body. They return `{"id":"service UUID","job_id":"UUID","status":"starting|stopping|restarting|deleting|installing"}`. Poll services to observe completion. One pending/processing job per service; another request returns 409. Offline nodes/unavailable runtimes or missing runtime identity return 409. Unknown resources return 404. Invalid bodies/IDs return 400. Internal failures return 500.

Completed create/start/restart observe `running` or `stopped`; completed stop observes `stopped`; delete records `deleted`, clears runtime ID and releases port reservation. Deleted services disappear from active listing; database tombstones and jobs remain. Failed jobs set `error`, preserve available runtime identity and expose error text. Observations record `running`, `stopped`, `missing`, or `unknown`; failure details persist until an explicit operation succeeds. An offline node does not imply a stopped service. Compare `node_status` and `observed_at` before treating observations as current.

### Sandbox orders

`POST /billing/orders` requires `Idempotency-Key` of 8–128 characters. Body:

```json
{"plan_id":"dev-alpine","node_id":"c6076de5-90d4-4842-b903-df48d453637c","name":"Test order","email":"test@example.com"}
```

For `dev-minecraft`, add `accept_eula:true`; an omitted/zero port is assigned during fulfillment, or pass an explicit valid `port`. Returns `order_id`, `invoice_id`, `status:"unpaid"`. The same normalized request/key returns the original invoice; a changed request/key returns 409. Invoices snapshot price/resources, use GBP minor units and a unique sequential number. Simulated payments are unique per invoice. Payment replay returns the same server/service. Capacity failures roll back payment and leave the invoice unpaid. The live preview remains sandbox-only. Configurable Stripe/PayPal checkout is described below; recurring billing, tax, cancellation and refund workflows remain outstanding.

## Authenticated agent endpoints

| Method | Path | Behavior |
| --- | --- | --- |
| POST | `/nodes/{id}/heartbeat` | Existing permanent-credential authentication and runtime/resource reporting |
| POST | `/nodes/{id}/jobs/claim` | Claim oldest pending job with `FOR UPDATE SKIP LOCKED` |
| POST | `/nodes/{id}/jobs/{jobID}/result` | Record result and service state in one transaction |
| GET | `/nodes/{id}/services` | Node-scoped inventory; runtime ID or known create-job name plus revision timestamp |
| POST | `/nodes/{id}/services/observations` | At most 200 observations; rejects changed ownership/identity and ignores stale revisions/active operations |

All require `Authorization: Bearer <permanent node credential>`. Never send that credential to React. Enrollment remains available at `/nodes/enroll` for the established workflow, but the existing development node must not be re-enrolled.

Result body:

```json
{"status":"completed","runtime_id":"opaque runtime identity","service_status":"running"}
```

Failures use `status:"failed"` and `error_message`; include runtime identity if creation succeeded before a later step failed. Completed managed jobs require runtime identity and a valid observed service status. Repeated identical final results return 200; conflicting results return 409. Node/service/server assignment is verified transactionally.

Lifecycle payload: `{"runtime_id":"opaque ID"}`. Create payload is allowlisted (`name`, `image`, `memory_bytes`, `cpu_threads`, `command`, `environment`, optional `port` and `persistent_data`). Allocation ownership comes from authenticated job metadata, not payload fields. The agent checks the managed marker and exact service/server labels before mutations and uses the inspected immutable ID.

Observation body: `{"services":[{"service_id":"UUID","runtime_id":"opaque ID","status":"running","revision":"inventory updated_at timestamp"}]}`. Revision protection prevents an old inspection from overwriting a newer operation result.

## Account authentication (2026-10-04)

- `POST /api/v1/auth/register`: email, name, password (12–256 bytes); creates a customer.
- `POST /api/v1/auth/login`: email/password; returns user and `csrf_token`, sets HttpOnly session cookie.
- `GET /api/v1/auth/session`: returns user/token for a valid session or `{ "user": null }`.
- `POST /api/v1/auth/logout`: requires session and CSRF token; revokes the current session.
- `GET /api/v1/locations`: public location catalogue without internal hostnames or capacity.
- `GET /api/v1/admin/accounts`: administrator account list; excludes password hashes.

Protected mutations require `X-CSRF-Token` plus the session cookie. Session lifetime
is 24 hours. Cross-origin writes are rejected; an optional comma-separated list of exact `BNB_WEB_ORIGIN` values
supports the Vite development proxy. Authentication attempts are limited per peer
IP in PostgreSQL. Proxy forwarding headers are not trusted for identity or rate limits.

Server/service/job/invoice access is scoped to the account. Administrators can
inspect all resources including unassigned legacy resources. Direct deployment,
allocation creation, node infrastructure listing and simulated payment are admin-only.
Customers create owned orders; email is derived from the signed-in account and
idempotency keys are scoped per account. Registration cannot assign admin roles.
Public node agent routes retain their existing separate node-credential authentication.

Email verification/recovery, MFA and recurring billing are not implemented yet. The sandbox invoice endpoints cannot collect money.

## Provider payments, catalogue, support and status

- `GET /billing/config`: current mode (`disabled`, `sandbox`, `test`, `live`), enabled flag,
  `collects_money`, recurring capability and provider availability; never credentials.
- `GET /billing/plans`: active plans; live mode excludes `is_demo` plans.
- `POST /billing/invoices/{id}/checkout`: `{ "provider": "stripe" | "paypal" }`;
  returns a provider checkout URL. Uses the stored invoice amount/currency/account,
  deterministic remote idempotency keys and a persisted provider reservation.
- `POST /billing/invoices/{id}/capture-paypal`: captures the invoice's stored PayPal
  order after approval. Ignores browser-supplied amounts, tokens and payment claims.
- `POST /billing/webhooks/{provider}`: bounded raw body; Stripe HMAC verification or
  PayPal verification API. Only provider-confirmed completed payments matching the
  checkout, amount, currency and mode can mark an invoice paid.
- `GET /admin/plans`, `PUT /admin/plans/{planID}`: list all plans and create/update
  validated GBP pricing/resources/templates/publication. Invoice snapshots do not change.
- `GET /status`: real Core/database/node/runtime/queue checks and persisted incidents.
- `POST /admin/incidents`, `PATCH /admin/incidents/{id}`: publish incidents/updates;
  resolution preserves public history. Admin session/CSRF required.
- `GET /support/tickets`, `POST /support/tickets`: list owned tickets/create conversation.
- `GET /support/tickets/{id}`, `PATCH /support/tickets/{id}`: read/reply/close/reopen;
  customers access only their own tickets, admins have the support inbox.

Provider receipt/event recording and invoice payment status commit together.
Fulfillment runs in separate retriable transactions; an offline node cannot undo a
confirmed payment. Payment and event replays do not produce duplicate services.
Invoice listings expose `fulfillment_status`/`fulfillment_error` and the resulting
server/service IDs. Sandbox simulation stays admin-only and cannot pay test/live invoices.

Provider adapters and fulfillment have mock/integration coverage. Merchant credentials
and actual provider test-mode delivery are not configured. See [payment setup](payment-setup.md).

## Files, addons and startup

All routes below are under `/api/v1` and require an owned service (or administrator), with CSRF on mutations.

- `POST /services/{id}/files`: JSON `{operation,path,destination?,content?}`; operations `list`, `read`, `write`, `mkdir`, `rename`, `delete`. Content is base64. Returns HTTP 202 with `job_id`. Java/Bedrock only; 24 KiB per file, 100 directory entries.
- `GET /jobs/{id}`: scoped operation status/error/result. File result `data` is a JSON string, parsed after completion.
- `GET /services/{id}/bedrock/addons`: owned installed/requested addon state.
- `POST /services/{id}/bedrock/addons`: raw `.mcpack`/`.mcaddon` bytes, up to 16 MiB; queues installation disabled initially.
- `PUT /services/{id}/bedrock/addons/{addonID}`: `{enabled:true|false}` queues default-world pack configuration.
- Minecraft configuration exposes `egg.default_startup`, `startup_resolved` and variables. Submitted startup commands are validated and run inside the owned game container.

Billing orders select `template` independently from their resource plan. Optional initial Java fields are `game_type`, `game_version`, `game_build`, `game_java`, `game_modpack`; the validated selection is frozen in the invoice. New plans/orders require at least 4096 MiB RAM. Services expose allocated primary and extra transport ports; use the node's configured public address for connection display.
