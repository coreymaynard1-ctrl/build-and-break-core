ALTER TABLE services
    ADD COLUMN memory_mb INTEGER CHECK (memory_mb > 0),
    ADD COLUMN cpu_threads INTEGER CHECK (cpu_threads > 0),
    ADD COLUMN port INTEGER CHECK (port BETWEEN 1024 AND 65535),
    ADD COLUMN observed_status VARCHAR(30),
    ADD COLUMN observed_at TIMESTAMPTZ,
    ADD COLUMN error_message TEXT,
    ADD COLUMN deleted_at TIMESTAMPTZ;
ALTER TABLE provisioning_jobs ADD COLUMN result JSONB;
CREATE UNIQUE INDEX idx_jobs_active_service ON provisioning_jobs(service_id)
    WHERE service_id IS NOT NULL AND status IN ('pending', 'processing');
-- Port reservations are node-scoped; nullable ports do not reserve anything.
CREATE TABLE service_ports (
    node_id UUID NOT NULL REFERENCES nodes(id),
    port INTEGER NOT NULL CHECK (port BETWEEN 1024 AND 65535),
    service_id UUID NOT NULL UNIQUE REFERENCES services(id),
    PRIMARY KEY (node_id, port)
);
