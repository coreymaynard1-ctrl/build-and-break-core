ALTER TABLE nodes
ADD COLUMN last_seen_at TIMESTAMPTZ;

CREATE INDEX idx_nodes_last_seen_at
ON nodes(last_seen_at);
