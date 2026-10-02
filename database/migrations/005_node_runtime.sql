ALTER TABLE nodes
ADD COLUMN runtime_available BOOLEAN NOT NULL DEFAULT FALSE,
ADD COLUMN runtime_name VARCHAR(50),
ADD COLUMN runtime_version VARCHAR(100),
ADD COLUMN runtime_api_version VARCHAR(100);
