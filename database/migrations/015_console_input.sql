ALTER TABLE services ADD COLUMN console_input_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE services ADD COLUMN runtime_metrics JSONB;
UPDATE services s SET console_input_enabled=true WHERE EXISTS(SELECT 1 FROM provisioning_jobs j WHERE j.service_id=s.id AND j.status='completed' AND j.job_type IN ('create_container','configure_minecraft') AND j.payload->'environment' @> '["CREATE_CONSOLE_IN_PIPE=true"]'::jsonb);
