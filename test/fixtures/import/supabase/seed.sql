-- Run as the postgres role after gotrue has migrated auth and storage-api has migrated storage.
-- Two tables, two RLS policies: one translatable (auth.uid() = user_id), one not (true).
CREATE TABLE public.notes (
  id uuid PRIMARY KEY DEFAULT extensions.uuid_generate_v4(),
  user_id uuid NOT NULL DEFAULT auth.uid() REFERENCES auth.users(id),
  title text NOT NULL,
  body jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE public.announcements (
  id bigint PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
  message text NOT NULL
);
ALTER TABLE public.notes ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.announcements ENABLE ROW LEVEL SECURITY;
CREATE POLICY notes_owner ON public.notes FOR ALL TO authenticated USING (auth.uid() = user_id) WITH CHECK (auth.uid() = user_id);
CREATE POLICY announcements_read ON public.announcements FOR SELECT TO anon, authenticated USING (true);
INSERT INTO public.notes (user_id, title, body)
  SELECT id, 'note for ' || email, jsonb_build_object('n', row_number() OVER (ORDER BY email)) FROM auth.users;
INSERT INTO public.announcements (message) VALUES ('welcome'), ('maintenance window');
-- A storage rule and a profile trigger on auth.users: neither is moved, both must be reported.
CREATE POLICY storage_avatars ON storage.objects FOR SELECT TO authenticated USING (bucket_id = 'avatars');
CREATE FUNCTION public.handle_new_user() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$;
CREATE TRIGGER on_auth_user_created AFTER INSERT ON auth.users FOR EACH ROW EXECUTE FUNCTION public.handle_new_user();
