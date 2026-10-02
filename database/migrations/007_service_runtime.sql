ALTER TABLE services
ADD COLUMN IF NOT EXISTS runtime_id VARCHAR(128);

CREATE UNIQUE INDEX IF NOT EXISTS idx_services_runtime_id
ON services(runtime_id)
WHERE runtime_id IS NOT NULL;
