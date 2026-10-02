CREATE TABLE node_credentials (
    node_id UUID PRIMARY KEY
        REFERENCES nodes(id)
        ON DELETE CASCADE,

    token_hash VARCHAR(64) NOT NULL UNIQUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    rotated_at TIMESTAMPTZ
);
