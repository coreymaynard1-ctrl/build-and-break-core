#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p bin
CGO_ENABLED=0 go build -o bin/bnb-fs ./cmd/server-files
go build -o bin/bnb-core ./apps/api
go build -o bin/bnb-agent ./services/agent
go build -o bin/bnb-migrate ./cmd/migrate
go build -o bin/bnb-account-admin ./cmd/account-admin
npm run build --prefix apps/web
