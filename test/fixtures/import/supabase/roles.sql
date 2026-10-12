-- Gives the service roles the fixture password (as the self-hosted Supabase compose does).
\set pgpass `echo "$POSTGRES_PASSWORD"`
ALTER USER supabase_auth_admin WITH PASSWORD :'pgpass';
ALTER USER supabase_storage_admin WITH PASSWORD :'pgpass';
ALTER USER authenticator WITH PASSWORD :'pgpass';
