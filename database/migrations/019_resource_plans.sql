-- New catalogue copy is resource based. Invoice/order snapshots stay immutable.
UPDATE billing_plans SET name='Pocket resources · 128 MiB' WHERE id='dev-alpine';
UPDATE billing_plans SET name='Builder resources · 2 GiB' WHERE id='dev-minecraft';
UPDATE billing_plans SET name='Starter resources · 1 GiB' WHERE id='dev-bedrock';
