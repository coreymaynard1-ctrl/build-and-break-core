# Development verification

Snapshot: 4 October 2026. See [developer handoff](handoff.md) for setup and remaining work.

Verified against the development Core, PostgreSQL, agent and Docker:

- Authentication, customer isolation, role checks and sandbox invoice fulfillment.
- Node enrollment and node-scoped job/observation authentication.
- Owned lifecycle operations, allocation budgets and persistent game volumes.
- Paper plugin installation/removal, Fabric mods and a Modrinth pack.
- Distinct per-service console content, command submission and correct allocated join ports.
- Concrete startup defaults, custom startup environment and restoration of the default.
- Bedrock primary TCP plus sixteen UDP reservations, status readiness and persistent restart.
- Bedrock behavior-pack upload/install/enable/disable, including readiness with the pack enabled.
- File list/write/read/rename/delete on stopped Java and Bedrock test services.
- Browser account/checkout/support/status and Minecraft management flows.

The development frontend is http://172.24.50.244:5173 and Core serves the built frontend at http://172.24.50.244:8080. These are private addresses and may change with WSL networking. The local node reports 7896 MiB RAM; existing reservations can prevent another 4 GiB purchase from being placed.

External merchant delivery, authenticated Bedrock client joining, Purpur/Spigot/Quilt deployments, most catalogue installers and the production operation requirements remain unverified or unfinished. Runtime readiness does not imply complete product parity.
