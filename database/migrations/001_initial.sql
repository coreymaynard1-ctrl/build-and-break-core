CREATE TABLE nodes (
    id UUID PRIMARY KEY,

    name VARCHAR(100) NOT NULL,
    hostname VARCHAR(255) NOT NULL UNIQUE,
    location VARCHAR(100) NOT NULL,

    status VARCHAR(30) NOT NULL DEFAULT 'offline',

    cpu_threads INTEGER NOT NULL DEFAULT 0,
    total_memory BIGINT NOT NULL DEFAULT 0,
    total_disk BIGINT NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE servers (
    id UUID PRIMARY KEY,

    node_id UUID REFERENCES nodes(id),

    name VARCHAR(100) NOT NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'pending',

    memory_mb INTEGER NOT NULL,
    disk_mb INTEGER NOT NULL,
    cpu_threads INTEGER NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE services (
    id UUID PRIMARY KEY,

    server_id UUID NOT NULL
        REFERENCES servers(id)
        ON DELETE CASCADE,

    name VARCHAR(100) NOT NULL,
    template VARCHAR(100) NOT NULL,

    status VARCHAR(30) NOT NULL DEFAULT 'installing',

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_servers_node_id
    ON servers(node_id);

CREATE INDEX idx_services_server_id
    ON services(server_id);
