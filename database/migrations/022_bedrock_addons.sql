CREATE TABLE bedrock_artifacts (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 service_id UUID NOT NULL REFERENCES services(id),
 archive BYTEA NOT NULL,
 sha256 TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE bedrock_addons (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 service_id UUID NOT NULL REFERENCES services(id),
 artifact_id UUID NOT NULL REFERENCES bedrock_artifacts(id),
 pack_id UUID NOT NULL,
 name TEXT NOT NULL,
 version JSONB NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('behavior','resource')),
 state TEXT NOT NULL DEFAULT 'pending',
 enabled BOOLEAN NOT NULL DEFAULT false,
 applied_enabled BOOLEAN NOT NULL DEFAULT false,
 UNIQUE(service_id,pack_id)
);
