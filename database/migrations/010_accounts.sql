CREATE TABLE accounts (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 email TEXT NOT NULL UNIQUE CHECK(email=lower(email)),
 name VARCHAR(100) NOT NULL,
 password_hash TEXT NOT NULL,
 role TEXT NOT NULL DEFAULT 'customer' CHECK(role IN ('customer','admin')),
 disabled BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE account_sessions (
 token_hash TEXT PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 csrf_token TEXT NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX account_sessions_account ON account_sessions(account_id);
CREATE TABLE auth_rate_limits (
 bucket TEXT PRIMARY KEY,
 attempts INTEGER NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL
);
-- Existing development resources remain unassigned and visible only to admins.
ALTER TABLE servers ADD COLUMN account_id UUID REFERENCES accounts(id);
CREATE INDEX servers_account ON servers(account_id);
ALTER TABLE billing_orders ADD COLUMN account_id UUID REFERENCES accounts(id);
CREATE INDEX billing_orders_account ON billing_orders(account_id);
