-- multi-app isolation reference (source_account_id wall)
-- Tables: public.notes, public.projects (per-app data), public.app_members (server-controlled)
-- Safe to re-run: every statement is IF NOT EXISTS.

SET lock_timeout = '5s';
SET statement_timeout = '60s';

CREATE TABLE IF NOT EXISTS public.notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    title TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS public.projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    title TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Who may act in which app (one app per user). Written with the admin secret
-- only; the Hasura `user` role has no permission on this table.
CREATE TABLE IF NOT EXISTS public.app_members (
    user_id UUID PRIMARY KEY REFERENCES auth.users(id) ON DELETE CASCADE,
    source_account_id TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS notes_source_account_id_idx ON public.notes (source_account_id);
CREATE INDEX IF NOT EXISTS projects_source_account_id_idx ON public.projects (source_account_id);
CREATE INDEX IF NOT EXISTS app_members_source_account_id_idx ON public.app_members (source_account_id);

RESET statement_timeout;
RESET lock_timeout;
