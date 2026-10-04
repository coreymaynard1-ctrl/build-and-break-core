INSERT INTO billing_plans(id,name,currency,amount_minor,memory_mb,disk_mb,cpu_threads,template,active,is_demo)
VALUES('dev-bedrock','Development Bedrock','GBP',500,1024,4096,1,'minecraft-bedrock',true,true)
ON CONFLICT(id) DO NOTHING;
