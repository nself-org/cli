-- Authored capture of one database for `nself db image switch`. Run with psql -X -q -At -d <db>.
-- The row count and hash are the definition of internal/postgres/tablehash: count(*) and
-- sum(hashtextextended(t::text, 0)::numeric) per table under tablehash.Settings. The settings
-- are applied here by value; a Go test pins them to tablehash.Settings.
SELECT set_config('TimeZone', 'UTC', false), set_config('DateStyle', 'ISO', false), set_config('IntervalStyle', 'postgres', false), set_config('extra_float_digits', '3', false), set_config('bytea_output', 'hex', false), set_config('lc_monetary', 'C', false), set_config('statement_timeout', '0', false), set_config('row_security', 'off', false), set_config('search_path', 'pg_catalog', false) \g /dev/null
SELECT jsonb_build_object(
 'encoding', current_setting('server_encoding'),
 'tables', (SELECT coalesce(jsonb_agg(jsonb_build_object('table', n.nspname || '.' || c.relname, 'count', x.c, 'hash', x.h) ORDER BY n.nspname COLLATE "C", c.relname COLLATE "C"), '[]'::jsonb)
   FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
   CROSS JOIN LATERAL (SELECT (xpath('/row/c/text()', q))[1]::text AS c, (xpath('/row/h/text()', q))[1]::text AS h
        FROM (SELECT query_to_xml(format('SELECT count(*) AS c, coalesce(sum(hashtextextended(t::text, 0)::numeric), 0)::text AS h FROM %I.%I AS t', n.nspname, c.relname), false, true, '') AS q) s) x
   WHERE c.relkind IN ('r', 'p') AND NOT c.relispartition AND n.nspname NOT IN ('pg_catalog', 'information_schema') AND n.nspname !~ '^pg_(toast|temp)'),
 'sequences', (SELECT coalesce(jsonb_agg(jsonb_build_array(schemaname, sequencename, last_value) ORDER BY schemaname COLLATE "C", sequencename COLLATE "C"), '[]'::jsonb) FROM pg_sequences),
 'extensions', (SELECT coalesce(jsonb_agg(jsonb_build_array(extname, extversion) ORDER BY extname COLLATE "C"), '[]'::jsonb) FROM pg_extension)
)::text
