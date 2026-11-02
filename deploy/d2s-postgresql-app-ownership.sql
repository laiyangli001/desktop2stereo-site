DO $$
DECLARE
    object_record record;
    object_kind text;
BEGIN
    FOR object_record IN
        SELECT n.nspname, c.relname, c.relkind
        FROM pg_class AS c
        JOIN pg_namespace AS n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public'
          AND c.relkind IN ('r', 'p', 'v', 'm', 'S')
          AND NOT (
              c.relkind = 'S'
              AND EXISTS (
                  SELECT 1 FROM pg_depend AS sequence_dependency
                  WHERE sequence_dependency.classid = 'pg_class'::regclass
                    AND sequence_dependency.objid = c.oid
                    AND sequence_dependency.refclassid = 'pg_class'::regclass
                    AND sequence_dependency.deptype IN ('a', 'i')
              )
          )
          AND c.relname NOT IN (
              'd2s_user_action_logs',
              'd2s_registration_records',
              'd2s_admin_operation_audits',
              'd2s_audit_retention_runs'
          )
          AND NOT EXISTS (
              SELECT 1 FROM pg_depend AS dependency
              WHERE dependency.classid = 'pg_class'::regclass
                AND dependency.objid = c.oid
                AND dependency.deptype = 'e'
          )
        ORDER BY c.relkind, c.relname
    LOOP
        object_kind := CASE object_record.relkind
            WHEN 'S' THEN 'SEQUENCE'
            WHEN 'v' THEN 'VIEW'
            WHEN 'm' THEN 'MATERIALIZED VIEW'
            ELSE 'TABLE'
        END;
        EXECUTE format(
            'ALTER %s %I.%I OWNER TO d2s_app',
            object_kind,
            object_record.nspname,
            object_record.relname
        );
    END LOOP;
END
$$;

GRANT USAGE, CREATE ON SCHEMA public TO d2s_app;
