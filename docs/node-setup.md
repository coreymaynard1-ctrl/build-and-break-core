# Adding Linux nodes

Core manages multiple independent agents. Install one agent on each Linux host
with its own Docker daemon. Never copy an enrolled agent's credentials to another
host. Core routes jobs by node ID, and every claim, result, observation and
heartbeat requires that node's credential.

Build the agent on a compatible Linux machine with Go:

```bash
go build -o bin/bnb-agent ./services/agent
CGO_ENABLED=0 go build -o bin/bnb-fs ./cmd/server-files
```

Transfer both binaries to the same directory on the new host. The file manager runs the static `bnb-fs` helper inside a temporary restricted container with only the selected service volume mounted. Docker must be installed and accessible to
the agent service account. In Administration, create an enrollment token using
the new node's name and location. The token expires in 30 minutes and works once.
On the new host, enter it without putting it in shell history:

```bash
read -r -s -p 'Enrollment token: ' BNB_ENROLLMENT_TOKEN
export BNB_ENROLLMENT_TOKEN
./bnb-agent enroll --core https://YOUR-CORE-DOMAIN
unset BNB_ENROLLMENT_TOKEN
./bnb-agent
```

Use the reachable development Core URL for local testing. A remote host cannot
reach a private WSL address without network routing. For remote deployment, Core
needs a reachable HTTPS endpoint; no additional remote hosts have been provided
or deployed yet.

The private identity is saved at `~/.config/build-and-break/agent.json` with mode
0600. Enrollment refuses to overwrite an existing configuration. Optional
`BNB_AGENT_CONFIG` selects a different identity path and `BNB_AGENT_ADDR` selects
the health listener (default `127.0.0.1:9090`). These settings do not create separate
physical resources when multiple processes share a Docker daemon.

For persistent operation, install the binaries at `/usr/local/bin/bnb-agent` and `/usr/local/bin/bnb-fs` and
configure a systemd service under the same service account used for enrollment:

```ini
[Unit]
Description=Build & Break node agent
After=network-online.target docker.service
Wants=network-online.target
Requires=docker.service

[Service]
User=YOUR-AGENT-USER
ExecStart=/usr/local/bin/bnb-agent
Restart=on-failure
RestartSec=5
UMask=0077

[Install]
WantedBy=multi-user.target
```

Replace the user, ensure it can access Docker, and enable the service on that
host. Node heartbeat and runtime readiness determine location availability.
Deployments reserve resources and ports on the selected node. An unavailable
node keeps paid fulfillment queued for retry.
