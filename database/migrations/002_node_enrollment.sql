CREATE TABLE node_enrollment_tokens (
    id UUID PRIMARY KEY,

    token_hash VARCHAR(64) NOT NULL UNIQUE,

    node_name VARCHAR(100) NOT NULL,
    location VARCHAR(100) NOT NULL,

    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_node_enrollment_tokens_token_hash
    ON node_enrollment_tokens(token_hash);
