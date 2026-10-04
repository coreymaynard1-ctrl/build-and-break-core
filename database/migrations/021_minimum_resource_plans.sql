UPDATE billing_plans SET memory_mb=GREATEST(memory_mb,4096),disk_mb=GREATEST(disk_mb,4096);
UPDATE billing_plans SET name='Starter resources · 4 GiB' WHERE id='dev-alpine';
UPDATE billing_plans SET name='Builder resources · 4 GiB' WHERE id='dev-minecraft';
UPDATE billing_plans SET name='Workshop resources · 4 GiB',cpu_threads=3 WHERE id='dev-bedrock';
INSERT INTO billing_plans(id,name,currency,amount_minor,memory_mb,disk_mb,cpu_threads,template,active,is_demo)
VALUES ('resources-8g','Studio resources · 8 GiB','GBP',1000,8192,16384,4,'resources',true,true),
('resources-16g','Workshop resources · 16 GiB','GBP',2000,16384,32768,6,'resources',true,true)
ON CONFLICT(id) DO NOTHING;
