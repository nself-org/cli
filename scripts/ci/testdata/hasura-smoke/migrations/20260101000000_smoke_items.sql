-- P7-LIVE-23 hasura-upgrade-smoke fixture: two tables with synthetic rows.
CREATE TABLE public.smoke_items (
  id serial PRIMARY KEY,
  name text NOT NULL,
  secret_note text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE public.smoke_tags (
  id serial PRIMARY KEY,
  item_id integer NOT NULL REFERENCES public.smoke_items (id),
  tag text NOT NULL
);
INSERT INTO public.smoke_items (name, secret_note) VALUES
  ('alpha', 'note-a'),
  ('beta', 'note-b'),
  ('gamma', 'note-c');
INSERT INTO public.smoke_tags (item_id, tag) VALUES
  (1, 'first'),
  (2, 'second'),
  (3, 'third');
