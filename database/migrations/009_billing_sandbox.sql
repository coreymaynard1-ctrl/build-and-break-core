-- A development ledger. Prices are demonstration values, not a live offer.
CREATE TABLE billing_plans (
 id TEXT PRIMARY KEY,
 name TEXT NOT NULL,
 currency VARCHAR(3) NOT NULL CHECK (currency='GBP'),
 amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
 memory_mb INTEGER NOT NULL CHECK (memory_mb > 0),
 disk_mb INTEGER NOT NULL CHECK (disk_mb > 0),
 cpu_threads INTEGER NOT NULL CHECK (cpu_threads > 0),
 template TEXT NOT NULL,
 active BOOLEAN NOT NULL DEFAULT true
);
INSERT INTO billing_plans VALUES
 ('dev-alpine','Development Alpine','GBP',100,128,128,1,'alpine-test',true),
 ('dev-minecraft','Development Minecraft','GBP',500,2048,4096,2,'minecraft-java',true);
CREATE TABLE billing_orders (
 id UUID PRIMARY KEY,
 idempotency_key TEXT NOT NULL UNIQUE,
 request JSONB NOT NULL,
 plan_id TEXT NOT NULL REFERENCES billing_plans(id),
 plan_snapshot JSONB NOT NULL,
 customer_email TEXT NOT NULL,
 name VARCHAR(100) NOT NULL,
 node_id UUID NOT NULL REFERENCES nodes(id),
 port INTEGER,
 status TEXT NOT NULL DEFAULT 'unpaid' CHECK(status IN ('unpaid','paid','cancelled')),
 server_id UUID REFERENCES servers(id),
 service_id UUID REFERENCES services(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE billing_invoices (
 id UUID PRIMARY KEY,
 number BIGINT GENERATED ALWAYS AS IDENTITY UNIQUE,
 order_id UUID NOT NULL UNIQUE REFERENCES billing_orders(id),
 currency VARCHAR(3) NOT NULL CHECK(currency='GBP'),
 amount_minor BIGINT NOT NULL CHECK(amount_minor > 0),
 status TEXT NOT NULL DEFAULT 'unpaid' CHECK(status IN ('unpaid','paid','void')),
 mode TEXT NOT NULL DEFAULT 'sandbox' CHECK(mode='sandbox'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 paid_at TIMESTAMPTZ
);
CREATE TABLE billing_payments (
 id UUID PRIMARY KEY,
 invoice_id UUID NOT NULL UNIQUE REFERENCES billing_invoices(id),
 provider TEXT NOT NULL CHECK(provider='sandbox'),
 provider_reference TEXT NOT NULL UNIQUE,
 currency VARCHAR(3) NOT NULL,
 amount_minor BIGINT NOT NULL CHECK(amount_minor > 0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
