CREATE TABLE provisioning_jobs (
    id UUID PRIMARY KEY,

    node_id UUID NOT NULL
        REFERENCES nodes(id)
        ON DELETE CASCADE,

    server_id UUID
        REFERENCES servers(id)
        ON DELETE CASCADE,

    service_id UUID
        REFERENCES services(id)
        ON DELETE CASCADE,

    job_type VARCHAR(50) NOT NULL,

    status VARCHAR(30) NOT NULL DEFAULT 'pending',

    payload JSONB NOT NULL DEFAULT '{}'::jsonb,

    error_message TEXT,

    attempts INTEGER NOT NULL DEFAULT 0,

    claimed_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_provisioning_jobs_node_status
ON provisioning_jobs(node_id, status);

CREATE INDEX idx_provisioning_jobs_created_at
ON provisioning_jobs(created_at);
