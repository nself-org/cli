CREATE SCHEMA app;
CREATE SCHEMA auth;
CREATE TABLE auth.users (id uuid PRIMARY KEY, email text, email_verified boolean, phone_number text, display_name text, created_at timestamptz, disabled boolean, password_hash text, metadata jsonb, default_role text NOT NULL DEFAULT 'user');
CREATE TABLE auth.user_roles (user_id uuid REFERENCES auth.users, role text);
INSERT INTO auth.users VALUES ('11111111-1111-4111-8111-111111111111','fixture@example.invalid',false,NULL,'Fixture','2026-01-01',false,NULL,'{}');
INSERT INTO auth.user_roles VALUES ('11111111-1111-4111-8111-111111111111','user');
-- Two phone-only users: NULL email must stay NULL (hasura-auth keeps email unique).
INSERT INTO auth.users VALUES ('33333333-3333-4333-8333-333333333333',NULL,false,'+15550000001',NULL,'2026-01-02',false,NULL,'{}'),('44444444-4444-4444-8444-444444444444',NULL,false,'+15550000002',NULL,'2026-01-03',false,NULL,'{}');
CREATE TABLE public.notes (id bigint PRIMARY KEY, amount numeric(20,6), score double precision, happened timestamptz, body jsonb, tags text[], payload bytea);
INSERT INTO public.notes SELECT i, i::numeric/7, i::double precision/13, '2026-01-01'::timestamptz + i*interval '1 second', jsonb_build_object('n',i,'unicode','日'), ARRAY['one',i::text], decode('00ff10','hex') FROM generate_series(1,10000) i;
CREATE TABLE app.no_key (name text, amount numeric);
INSERT INTO app.no_key VALUES ('duplicate', 1.25), ('duplicate',1.25), (NULL,NULL);
CREATE TABLE app.parents (id integer PRIMARY KEY);
INSERT INTO app.parents VALUES (1),(2);
CREATE TABLE app.children (id integer PRIMARY KEY, parent_id integer REFERENCES app.parents);
INSERT INTO app.children VALUES (1,1),(2,2);
CREATE TABLE public.empty_table (id uuid PRIMARY KEY, owner uuid REFERENCES auth.users);
INSERT INTO public.empty_table VALUES ('22222222-2222-4222-8222-222222222222','11111111-1111-4111-8111-111111111111');
