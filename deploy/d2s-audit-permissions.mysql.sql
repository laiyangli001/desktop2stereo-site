-- Replace the database and account-host placeholders before execution.
-- Run as a database administrator after the audit tables have been migrated.
-- Create fresh d2s_app and d2s_audit_cleanup accounts with no global, database-wide,
-- or inherited role privileges that grant UPDATE/DELETE on these audit tables.
-- Grant the app account its non-audit table privileges separately and table by table.
GRANT SELECT, INSERT ON `__D2S_DATABASE__`.`d2s_user_action_logs` TO 'd2s_app'@'__D2S_APP_HOST__';
GRANT SELECT, INSERT ON `__D2S_DATABASE__`.`d2s_registration_records` TO 'd2s_app'@'__D2S_APP_HOST__';
GRANT SELECT, INSERT ON `__D2S_DATABASE__`.`d2s_admin_operation_audits` TO 'd2s_app'@'__D2S_APP_HOST__';
GRANT SELECT, INSERT ON `__D2S_DATABASE__`.`d2s_audit_retention_runs` TO 'd2s_app'@'__D2S_APP_HOST__';

GRANT SELECT, DELETE ON `__D2S_DATABASE__`.`d2s_user_action_logs` TO 'd2s_audit_cleanup'@'__D2S_CLEANUP_HOST__';
GRANT SELECT, DELETE ON `__D2S_DATABASE__`.`d2s_registration_records` TO 'd2s_audit_cleanup'@'__D2S_CLEANUP_HOST__';
GRANT SELECT, DELETE ON `__D2S_DATABASE__`.`d2s_admin_operation_audits` TO 'd2s_audit_cleanup'@'__D2S_CLEANUP_HOST__';
GRANT SELECT, INSERT ON `__D2S_DATABASE__`.`d2s_audit_retention_runs` TO 'd2s_audit_cleanup'@'__D2S_CLEANUP_HOST__';

SHOW GRANTS FOR 'd2s_app'@'__D2S_APP_HOST__';
SHOW GRANTS FOR 'd2s_audit_cleanup'@'__D2S_CLEANUP_HOST__';
