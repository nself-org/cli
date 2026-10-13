-- Authored capture of cluster-level facts for `nself db image switch`.
-- Run with psql -X -q -At on the old cluster (before) and on the restored cluster (after); the
-- two single-line JSON documents must be byte-identical. No pg_stat counters: reads move them.
SELECT jsonb_build_object(
 'major', current_setting('server_version_num')::int / 10000,
 'bootstrap', (SELECT rolname FROM pg_authid WHERE oid = 10),
 'templates', (SELECT coalesce(jsonb_agg(datname ORDER BY datname COLLATE "C"), '[]'::jsonb) FROM pg_database WHERE datistemplate AND datname NOT IN ('template0', 'template1')),
 'tablespaces', (SELECT coalesce(jsonb_agg(spcname ORDER BY spcname COLLATE "C"), '[]'::jsonb) FROM pg_tablespace WHERE spcname NOT IN ('pg_default', 'pg_global')),
 'roles', (SELECT coalesce(jsonb_agg(jsonb_build_array(rolname, rolsuper, rolinherit, rolcreaterole, rolcreatedb, rolcanlogin, rolreplication, rolbypassrls, rolconnlimit, rolvaliduntil::text, md5(coalesce(rolpassword, ''))) ORDER BY rolname COLLATE "C"), '[]'::jsonb) FROM pg_authid),
 'memberships', (SELECT coalesce(jsonb_agg(jsonb_build_array(r.rolname, m.rolname, g.rolname, a.admin_option, to_jsonb(a) -> 'inherit_option', to_jsonb(a) -> 'set_option') ORDER BY r.rolname COLLATE "C", m.rolname COLLATE "C"), '[]'::jsonb)
    FROM pg_auth_members a JOIN pg_roles r ON r.oid = a.roleid JOIN pg_roles m ON m.oid = a.member JOIN pg_roles g ON g.oid = a.grantor),
 'role_settings', (SELECT coalesce(jsonb_agg(jsonb_build_array(coalesce(r.rolname, ''), s.setconfig) ORDER BY coalesce(r.rolname, '') COLLATE "C"), '[]'::jsonb)
    FROM pg_db_role_setting s LEFT JOIN pg_roles r ON r.oid = s.setrole WHERE s.setdatabase = 0),
 'databases', (SELECT coalesce(jsonb_agg(jsonb_build_object(
      'name', d.datname,
      'owner', pg_get_userbyid(d.datdba),
      'provider', coalesce(to_jsonb(d) ->> 'datlocprovider', 'c'),
      'allow_connections', d.datallowconn,
      'collate', d.datcollate,
      'ctype', d.datctype,
      'create', format('CREATE DATABASE %I WITH TEMPLATE = template0 OWNER = %I ENCODING = %L LC_COLLATE = %L LC_CTYPE = %L CONNECTION LIMIT = %s', d.datname, pg_get_userbyid(d.datdba), pg_encoding_to_char(d.encoding), d.datcollate, d.datctype, d.datconnlimit),
      'explicit_acl', d.datacl IS NOT NULL,
      'acl', (SELECT coalesce(jsonb_agg(format('GRANT %s ON DATABASE %I TO %s%s', x.privilege_type, d.datname, CASE WHEN x.grantee = 0 THEN 'PUBLIC' ELSE quote_ident(pg_get_userbyid(x.grantee)) END, CASE WHEN x.is_grantable THEN ' WITH GRANT OPTION' ELSE '' END) ORDER BY x.grantee, x.privilege_type), '[]'::jsonb) FROM aclexplode(d.datacl) x),
      -- default_transaction_read_only is the quiesce setting this command adds; it is never carried over.
      'settings', (SELECT coalesce(jsonb_agg(jsonb_build_array(coalesce(r.rolname, ''), c) ORDER BY coalesce(r.rolname, '') COLLATE "C", c COLLATE "C"), '[]'::jsonb)
          FROM pg_db_role_setting s LEFT JOIN pg_roles r ON r.oid = s.setrole CROSS JOIN LATERAL unnest(s.setconfig) c
          WHERE s.setdatabase = d.oid AND c !~ '^default_transaction_read_only=')
    ) ORDER BY d.datname COLLATE "C"), '[]'::jsonb) FROM pg_database d WHERE NOT d.datistemplate)
)::text
