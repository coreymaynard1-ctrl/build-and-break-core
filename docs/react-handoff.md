# React developer handoff

## Current structure

- `apps/web/src/api.js`: reusable same-origin Core client and API errors.
- `App.jsx`: route/hash state, 5-second polling, landing page, server list/detail and lifecycle controls.
- `CreateServer.jsx`: workload → resources → location → atomic real provisioning flow.
- `Billing.jsx`: explicitly sandbox order/invoice/payment flow.
- `components.jsx`: reusable status, field, panel and empty state.
- `style.css`: intentionally small basic stylesheet.
- `vite.config.js`: development/preview proxy; Core serves the built frontend on the same origin.

Use `/api/v1` only. No Docker or agent URLs/tokens belong in React. Keep service controls asynchronous: 202 means queued, not finished. Use `job_status`, `status`, `error_message`, `observed_status`, `observed_at`, and `node_status`. Show container-running separately from Minecraft readiness. Delete requires user confirmation and preserves world volumes.

## Intended product direction

Server.pro's public pages describe multiple services inside a server and a game/app → plan → resources/location → payment flow, with unified management, file access, backups and sharing. These are reference workflows, not evidence about its private implementation:

- https://server.pro/
- https://server.pro/create/ubuntu
- https://server.pro/support

Build & Break should own the Core, agent, data and UI. The current create page gets the customer to a real server with one service; a server page groups its services. The raw node view is tucked under infrastructure. A future public checkout should connect to owned billing/entitlements and a configured payment provider rather than minting unpaid services.

## Next frontend work

Add dedicated customer account/auth flows and an admin area after Core authorization/tenant ownership exist. Use a shared shell for website, customer game/app panels, future VPS panels and billing. Replace hash routes with the project's preferred router; extract hooks/forms/cards as useful. Add plan cards, location cards and a proper checkout stage when billing provider and subscription contracts are ready. Retain actual state/error feedback; do not substitute mock services or simulated lifecycle controls.

No dedicated admin page, console, files, player controls, backups, subscription management or VPS features currently exist. Do not present them as functioning controls. Sandbox billing is local testing and cannot collect money. See `api.md` for the backend contract and `session-status.md` for actual verification results.
