--
-- PostgreSQL database dump (schema only, `pg_restore --schema-only -f -`)
--

SET statement_timeout = 0;
SET client_encoding = 'UTF8';

CREATE TABLE public.messages (
    id bigint NOT NULL,
    owner text NOT NULL
);

ALTER TABLE public.messages ENABLE ROW LEVEL SECURITY;

--
-- Name: messages messages_mux_read; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY messages_mux_read ON public.messages FOR SELECT TO mux_user USING ((owner = CURRENT_USER));

CREATE POLICY messages_mux_write ON public.messages AS RESTRICTIVE FOR INSERT TO mux_user, CURRENT_USER WITH CHECK ((owner = 'x TO "Not Valid"'::text));

CREATE POLICY messages_public ON public.messages FOR SELECT TO PUBLIC USING (true);

CREATE POLICY "to TO ""quoted"" policy" ON public."TO" USING (true);

-- CREATE POLICY commented_out ON public.messages TO ghost_role;

COMMENT ON POLICY messages_public ON public.messages IS '
CREATE POLICY in_a_string ON public.messages TO "Quoted Invalid";';

CREATE FUNCTION public.f() RETURNS void LANGUAGE sql AS $$ SELECT 1 $$;
