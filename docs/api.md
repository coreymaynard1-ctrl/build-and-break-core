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
| GET | `/billing/config` | Sandbox mode/enabled flag; no money collected |
| GET | `/billing/plans` | Active demo plans; requires sandbox enabled |
| GET | `/billing/invoices` | Latest 100 local sandbox invoices |
| POST | `/billing/orders` | Snapshot plan and create unpaid invoice; 201, replay 200 |
| POST | `/billing/invoices/{id}/pay-sandbox` | Simulate payment, allocate server/service and enqueue provisioning atomically; 200 |

These browser-facing endpoints are currently for the local development operator and have no customer authentication/tenant scoping. Agent endpoints require permanent node credentials.

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

For `dev-minecraft`, add `port` and `accept_eula:true`. Returns `order_id`, `invoice_id`, `status:"unpaid"`. The same normalized request/key returns the original invoice; a changed request/key returns 409. Invoices snapshot price/resources, use GBP minor units and a unique sequential number. Simulated payments are unique per invoice. Payment replay returns the same server/service. Capacity failures roll back payment and leave the invoice unpaid. No provider, recurring billing, tax, cancellation or refund workflow is enabled.

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
