CREATE TABLE status_incidents (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 title VARCHAR(160) NOT NULL,
 message TEXT NOT NULL,
 impact TEXT NOT NULL CHECK(impact IN ('minor','major','maintenance')),
 status TEXT NOT NULL CHECK(status IN ('investigating','identified','monitoring','resolved')),
 created_by UUID NOT NULL REFERENCES accounts(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 resolved_at TIMESTAMPTZ
);
CREATE INDEX status_incidents_recent ON status_incidents(created_at DESC);
CREATE TABLE status_incident_updates (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 incident_id UUID NOT NULL REFERENCES status_incidents(id),
 message TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('investigating','identified','monitoring','resolved')),
 created_by UUID NOT NULL REFERENCES accounts(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
