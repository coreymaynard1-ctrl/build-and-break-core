ALTER TABLE service_ports DROP CONSTRAINT service_ports_service_id_key;
ALTER TABLE services ADD COLUMN extra_ports JSONB NOT NULL DEFAULT '[]';
