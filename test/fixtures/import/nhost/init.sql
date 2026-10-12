-- hasura-auth and hasura-storage migrate inside their own schemas and expect them to exist, as in
-- the nhost postgres image. Extensions are created with them for the same reason.
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;
CREATE SCHEMA IF NOT EXISTS auth;
CREATE SCHEMA IF NOT EXISTS storage;
