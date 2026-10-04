ALTER TABLE services ADD COLUMN minecraft_config JSONB;
ALTER TABLE services ADD COLUMN minecraft_applied_config JSONB;
ALTER TABLE services ADD COLUMN console_tail TEXT;
ALTER TABLE services ADD COLUMN console_updated_at TIMESTAMPTZ;
