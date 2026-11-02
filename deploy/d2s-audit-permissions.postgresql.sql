-- Run as a database administrator after the audit tables have been migrated.
-- Create d2s_migrator, d2s_app, and d2s_audit_cleanup roles separately.
-- The runtime and cleanup roles must not inherit privileges from d2s_migrator.
BEGIN;

GRANT USAGE ON SCHEMA public TO d2s_migrator, d2s_app, d2s_audit_cleanup;

ALTER TABLE public.d2s_user_action_logs OWNER TO d2s_migrator;
ALTER TABLE public.d2s_registration_records OWNER TO d2s_migrator;
ALTER TABLE public.d2s_admin_operation_audits OWNER TO d2s_migrator;
ALTER TABLE public.d2s_audit_retention_runs OWNER TO d2s_migrator;

REVOKE ALL PRIVILEGES ON TABLE
    public.d2s_user_action_logs,
    public.d2s_registration_records,
    public.d2s_admin_operation_audits,
    public.d2s_audit_retention_runs
FROM PUBLIC, d2s_app, d2s_audit_cleanup;

GRANT SELECT, INSERT ON TABLE
    public.d2s_user_action_logs,
    public.d2s_registration_records,
    public.d2s_admin_operation_audits,
    public.d2s_audit_retention_runs
TO d2s_app;

GRANT SELECT, DELETE ON TABLE
    public.d2s_user_action_logs,
    public.d2s_registration_records,
    public.d2s_admin_operation_audits
TO d2s_audit_cleanup;

GRANT SELECT, INSERT ON TABLE public.d2s_audit_retention_runs TO d2s_audit_cleanup;

COMMIT;

DO $$
DECLARE
    table_name text;
    table_ref text;
BEGIN
    FOREACH table_name IN ARRAY ARRAY[
        'd2s_user_action_logs',
        'd2s_registration_records',
        'd2s_admin_operation_audits',
        'd2s_audit_retention_runs'
    ] LOOP
        table_ref := format('public.%I', table_name);
        IF NOT has_table_privilege('d2s_app', table_ref, 'SELECT')
           OR NOT has_table_privilege('d2s_app', table_ref, 'INSERT')
           OR has_table_privilege('d2s_app', table_ref, 'UPDATE')
           OR has_any_column_privilege('d2s_app', table_ref, 'UPDATE')
           OR has_table_privilege('d2s_app', table_ref, 'DELETE')
           OR has_table_privilege('d2s_app', table_ref, 'TRUNCATE')
           OR has_table_privilege('d2s_app', table_ref, 'REFERENCES')
           OR has_table_privilege('d2s_app', table_ref, 'TRIGGER') THEN
            RAISE EXCEPTION 'invalid application privileges for %', table_ref;
        END IF;
    END LOOP;

    FOREACH table_name IN ARRAY ARRAY[
        'd2s_user_action_logs',
        'd2s_registration_records',
        'd2s_admin_operation_audits'
    ] LOOP
        table_ref := format('public.%I', table_name);
        IF NOT has_table_privilege('d2s_audit_cleanup', table_ref, 'SELECT')
           OR NOT has_table_privilege('d2s_audit_cleanup', table_ref, 'DELETE')
           OR has_table_privilege('d2s_audit_cleanup', table_ref, 'INSERT')
           OR has_table_privilege('d2s_audit_cleanup', table_ref, 'UPDATE')
           OR has_any_column_privilege('d2s_audit_cleanup', table_ref, 'UPDATE')
           OR has_table_privilege('d2s_audit_cleanup', table_ref, 'TRUNCATE')
           OR has_table_privilege('d2s_audit_cleanup', table_ref, 'REFERENCES')
           OR has_table_privilege('d2s_audit_cleanup', table_ref, 'TRIGGER') THEN
            RAISE EXCEPTION 'invalid cleanup privileges for %', table_ref;
        END IF;
    END LOOP;

    IF NOT has_table_privilege('d2s_audit_cleanup', 'public.d2s_audit_retention_runs', 'SELECT')
       OR NOT has_table_privilege('d2s_audit_cleanup', 'public.d2s_audit_retention_runs', 'INSERT')
       OR has_table_privilege('d2s_audit_cleanup', 'public.d2s_audit_retention_runs', 'UPDATE')
       OR has_any_column_privilege('d2s_audit_cleanup', 'public.d2s_audit_retention_runs', 'UPDATE')
       OR has_table_privilege('d2s_audit_cleanup', 'public.d2s_audit_retention_runs', 'DELETE')
       OR has_table_privilege('d2s_audit_cleanup', 'public.d2s_audit_retention_runs', 'TRUNCATE')
       OR has_table_privilege('d2s_audit_cleanup', 'public.d2s_audit_retention_runs', 'REFERENCES')
       OR has_table_privilege('d2s_audit_cleanup', 'public.d2s_audit_retention_runs', 'TRIGGER') THEN
        RAISE EXCEPTION 'invalid cleanup privileges for public.d2s_audit_retention_runs';
    END IF;
END
$$;
