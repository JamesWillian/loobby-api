--
-- PostgreSQL database dump
--

\restrict IVIf9hGbzNu4p1ozCTrmhgLXu4T3LYqJBgDmkaexfhUVK0LCbFdLuxu4tQlT5ED

-- Dumped from database version 17.6
-- Dumped by pg_dump version 18.6

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: public; Type: SCHEMA; Schema: -; Owner: -
--

CREATE SCHEMA public;


--
-- Name: SCHEMA public; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON SCHEMA public IS 'standard public schema';


--
-- Name: rsvp_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.rsvp_status AS ENUM (
    'YES',
    'NO',
    'MAYBE_RESERVE'
);


--
-- Name: add_owner_to_group_members(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.add_owner_to_group_members() RETURNS trigger
    LANGUAGE plpgsql
    AS $$begin
  insert into group_members (id, group_id, user_id)
  values (gen_random_uuid(), NEW.id, NEW.owner_id)
  on conflict (group_id, user_id) do nothing;

  return NEW;
end;$$;


--
-- Name: fn_auto_rsvp_owner(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.fn_auto_rsvp_owner() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
begin
  insert into event_rsvps (event_id, user_id, status)
  values (new.id, new.owner_id, 'YES');
  return new;
end;
$$;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: device_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_tokens (
    token text NOT NULL,
    user_id uuid NOT NULL,
    platform text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT device_tokens_platform_check CHECK ((platform = ANY (ARRAY['ANDROID'::text, 'IOS'::text])))
);


--
-- Name: event_rsvps; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.event_rsvps (
    event_id uuid NOT NULL,
    user_id uuid NOT NULL,
    status text NOT NULL,
    is_paid boolean DEFAULT false NOT NULL,
    obs text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: event_teams; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.event_teams (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    event_id uuid NOT NULL,
    "order" integer DEFAULT 0 NOT NULL,
    name text NOT NULL,
    color text
);


--
-- Name: events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    event_type text NOT NULL,
    group_id uuid,
    is_instant boolean DEFAULT false NOT NULL,
    owner_id uuid NOT NULL,
    scheduled_datetime timestamp with time zone NOT NULL,
    name text NOT NULL,
    description text,
    invite_code text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT events_instant_group_ck CHECK ((((is_instant = true) AND (group_id IS NULL)) OR (is_instant = false)))
);


--
-- Name: gameplay_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gameplay_events (
    event_id uuid NOT NULL,
    game_id text,
    game_name text NOT NULL
);


--
-- Name: games; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.games (
    id text NOT NULL,
    slug text,
    name text NOT NULL,
    background_image text,
    released date,
    rating numeric(4,2),
    metacritic integer,
    description_raw text,
    genres jsonb,
    platforms jsonb,
    raw_payload jsonb,
    cached_at timestamp with time zone DEFAULT now() NOT NULL,
    refreshed_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: group_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.group_members (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    group_id uuid NOT NULL,
    user_id uuid NOT NULL
);


--
-- Name: groups; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.groups (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    invite_code text NOT NULL,
    image_url text,
    owner_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: notification_dispatch_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notification_dispatch_log (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    notification_type text NOT NULL,
    reference_id uuid NOT NULL,
    dispatched_date date NOT NULL,
    dispatched_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: sport_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sport_events (
    event_id uuid NOT NULL,
    duration_minutes integer NOT NULL,
    arena text,
    price_per_player numeric(12,2) DEFAULT 0 NOT NULL,
    max_players integer,
    accept_reserve boolean DEFAULT false NOT NULL,
    CONSTRAINT sport_events_duration_minutes_check CHECK ((duration_minutes > 0)),
    CONSTRAINT sport_events_max_players_check CHECK (((max_players IS NULL) OR (max_players > 0))),
    CONSTRAINT sport_events_price_per_player_check CHECK ((price_per_player >= (0)::numeric))
);


--
-- Name: team_players; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.team_players (
    team_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text
);


--
-- Name: user_credentials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_credentials (
    user_id uuid NOT NULL,
    password_hash text NOT NULL,
    roles text DEFAULT 'USER'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    email text NOT NULL,
    email_verified boolean DEFAULT false NOT NULL,
    email_verification_token text,
    email_verification_expires_at timestamp with time zone,
    password_reset_token text,
    password_reset_expires_at timestamp with time zone
);


--
-- Name: user_feed; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.user_feed WITH (security_invoker='on') AS
 SELECT user_id,
    id,
    name,
    image_url,
    entry_type,
    is_finished,
    scheduled_datetime
   FROM ( SELECT gm.user_id,
            g.id,
            g.name,
            g.image_url,
            'GROUP'::text AS entry_type,
            false AS is_finished,
            NULL::timestamp with time zone AS scheduled_datetime,
            2 AS sort_bucket
           FROM (public.group_members gm
             JOIN public.groups g ON ((g.id = gm.group_id)))
        UNION ALL
         SELECT r.user_id,
            e.id,
            e.name,
            NULL::text AS image_url,
            'EVENT'::text AS entry_type,
                CASE
                    WHEN (e.event_type = 'SPORT'::text) THEN ((e.scheduled_datetime + ((COALESCE(se.duration_minutes, 0) || ' minutes'::text))::interval) < now())
                    ELSE (e.scheduled_datetime < now())
                END AS is_finished,
            e.scheduled_datetime,
                CASE
                    WHEN
                    CASE
                        WHEN (e.event_type = 'SPORT'::text) THEN ((e.scheduled_datetime + ((COALESCE(se.duration_minutes, 0) || ' minutes'::text))::interval) < now())
                        ELSE (e.scheduled_datetime < now())
                    END THEN 3
                    ELSE 1
                END AS sort_bucket
           FROM ((public.events e
             JOIN public.event_rsvps r ON ((r.event_id = e.id)))
             LEFT JOIN public.sport_events se ON ((se.event_id = e.id)))
          WHERE (e.is_instant = true)) feed
  ORDER BY sort_bucket, scheduled_datetime;


--
-- Name: user_google_credentials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_google_credentials (
    user_id uuid NOT NULL,
    google_id text NOT NULL,
    email text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    username text NOT NULL,
    displayname text NOT NULL,
    avatar_url text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    auth_provider integer DEFAULT 0 NOT NULL
);


--
-- Name: device_tokens device_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_tokens
    ADD CONSTRAINT device_tokens_pkey PRIMARY KEY (token);


--
-- Name: event_rsvps event_rsvps_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_rsvps
    ADD CONSTRAINT event_rsvps_pkey PRIMARY KEY (event_id, user_id);


--
-- Name: event_teams event_teams_event_name_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_teams
    ADD CONSTRAINT event_teams_event_name_unique UNIQUE (event_id, name);


--
-- Name: event_teams event_teams_event_order_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_teams
    ADD CONSTRAINT event_teams_event_order_unique UNIQUE (event_id, "order");


--
-- Name: event_teams event_teams_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_teams
    ADD CONSTRAINT event_teams_pkey PRIMARY KEY (id);


--
-- Name: events events_invite_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events
    ADD CONSTRAINT events_invite_code_key UNIQUE (invite_code);


--
-- Name: events events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events
    ADD CONSTRAINT events_pkey PRIMARY KEY (id);


--
-- Name: gameplay_events gameplay_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gameplay_events
    ADD CONSTRAINT gameplay_events_pkey PRIMARY KEY (event_id);


--
-- Name: games games_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.games
    ADD CONSTRAINT games_pkey PRIMARY KEY (id);


--
-- Name: games games_slug_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.games
    ADD CONSTRAINT games_slug_key UNIQUE (slug);


--
-- Name: group_members group_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.group_members
    ADD CONSTRAINT group_members_pkey PRIMARY KEY (id);


--
-- Name: group_members group_members_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.group_members
    ADD CONSTRAINT group_members_unique UNIQUE (group_id, user_id);


--
-- Name: groups groups_invite_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.groups
    ADD CONSTRAINT groups_invite_code_key UNIQUE (invite_code);


--
-- Name: groups groups_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.groups
    ADD CONSTRAINT groups_pkey PRIMARY KEY (id);


--
-- Name: notification_dispatch_log notification_dispatch_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notification_dispatch_log
    ADD CONSTRAINT notification_dispatch_log_pkey PRIMARY KEY (id);


--
-- Name: notification_dispatch_log notification_dispatch_log_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notification_dispatch_log
    ADD CONSTRAINT notification_dispatch_log_unique UNIQUE (user_id, notification_type, reference_id, dispatched_date);


--
-- Name: sport_events sport_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sport_events
    ADD CONSTRAINT sport_events_pkey PRIMARY KEY (event_id);


--
-- Name: team_players team_players_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_players
    ADD CONSTRAINT team_players_pkey PRIMARY KEY (team_id, user_id);


--
-- Name: user_credentials user_credentials_email_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_credentials
    ADD CONSTRAINT user_credentials_email_key UNIQUE (email);


--
-- Name: user_credentials user_credentials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_credentials
    ADD CONSTRAINT user_credentials_pkey PRIMARY KEY (user_id);


--
-- Name: user_google_credentials user_google_credentials_google_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_google_credentials
    ADD CONSTRAINT user_google_credentials_google_id_key UNIQUE (google_id);


--
-- Name: user_google_credentials user_google_credentials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_google_credentials
    ADD CONSTRAINT user_google_credentials_pkey PRIMARY KEY (user_id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: users users_username_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_username_key UNIQUE (username);


--
-- Name: games_name_trgm_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX games_name_trgm_idx ON public.games USING gin (name public.gin_trgm_ops);


--
-- Name: idx_device_tokens_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_device_tokens_user_id ON public.device_tokens USING btree (user_id);


--
-- Name: idx_dispatch_log_user_type_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dispatch_log_user_type_date ON public.notification_dispatch_log USING btree (user_id, notification_type, dispatched_date);


--
-- Name: groups trg_add_owner_to_group_members; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_add_owner_to_group_members AFTER INSERT ON public.groups FOR EACH ROW EXECUTE FUNCTION public.add_owner_to_group_members();


--
-- Name: events trg_auto_rsvp_owner; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_auto_rsvp_owner AFTER INSERT ON public.events FOR EACH ROW EXECUTE FUNCTION public.fn_auto_rsvp_owner();


--
-- Name: device_tokens device_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_tokens
    ADD CONSTRAINT device_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: event_rsvps event_rsvps_event_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_rsvps
    ADD CONSTRAINT event_rsvps_event_id_fkey FOREIGN KEY (event_id) REFERENCES public.events(id) ON DELETE CASCADE;


--
-- Name: event_rsvps event_rsvps_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_rsvps
    ADD CONSTRAINT event_rsvps_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: event_teams event_teams_event_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_teams
    ADD CONSTRAINT event_teams_event_id_fkey FOREIGN KEY (event_id) REFERENCES public.events(id) ON DELETE CASCADE;


--
-- Name: events events_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events
    ADD CONSTRAINT events_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.groups(id) ON DELETE CASCADE;


--
-- Name: events events_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events
    ADD CONSTRAINT events_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE RESTRICT;


--
-- Name: gameplay_events gameplay_events_event_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gameplay_events
    ADD CONSTRAINT gameplay_events_event_id_fkey FOREIGN KEY (event_id) REFERENCES public.events(id) ON DELETE CASCADE;


--
-- Name: group_members group_members_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.group_members
    ADD CONSTRAINT group_members_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.groups(id) ON DELETE CASCADE;


--
-- Name: group_members group_members_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.group_members
    ADD CONSTRAINT group_members_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: groups groups_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.groups
    ADD CONSTRAINT groups_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE RESTRICT;


--
-- Name: notification_dispatch_log notification_dispatch_log_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notification_dispatch_log
    ADD CONSTRAINT notification_dispatch_log_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: sport_events sport_events_event_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sport_events
    ADD CONSTRAINT sport_events_event_id_fkey FOREIGN KEY (event_id) REFERENCES public.events(id) ON DELETE CASCADE;


--
-- Name: team_players team_players_team_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_players
    ADD CONSTRAINT team_players_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.event_teams(id) ON DELETE CASCADE;


--
-- Name: team_players team_players_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_players
    ADD CONSTRAINT team_players_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_credentials user_credentials_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_credentials
    ADD CONSTRAINT user_credentials_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_google_credentials user_google_credentials_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_google_credentials
    ADD CONSTRAINT user_google_credentials_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: device_tokens; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.device_tokens ENABLE ROW LEVEL SECURITY;

--
-- Name: event_rsvps; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.event_rsvps ENABLE ROW LEVEL SECURITY;

--
-- Name: event_teams; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.event_teams ENABLE ROW LEVEL SECURITY;

--
-- Name: events; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.events ENABLE ROW LEVEL SECURITY;

--
-- Name: gameplay_events; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.gameplay_events ENABLE ROW LEVEL SECURITY;

--
-- Name: games; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.games ENABLE ROW LEVEL SECURITY;

--
-- Name: group_members; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.group_members ENABLE ROW LEVEL SECURITY;

--
-- Name: groups; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.groups ENABLE ROW LEVEL SECURITY;

--
-- Name: notification_dispatch_log; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.notification_dispatch_log ENABLE ROW LEVEL SECURITY;

--
-- Name: sport_events; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.sport_events ENABLE ROW LEVEL SECURITY;

--
-- Name: team_players; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.team_players ENABLE ROW LEVEL SECURITY;

--
-- Name: user_credentials; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.user_credentials ENABLE ROW LEVEL SECURITY;

--
-- Name: user_google_credentials; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.user_google_credentials ENABLE ROW LEVEL SECURITY;

--
-- Name: users; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.users ENABLE ROW LEVEL SECURITY;

--
-- PostgreSQL database dump complete
--

\unrestrict IVIf9hGbzNu4p1ozCTrmhgLXu4T3LYqJBgDmkaexfhUVK0LCbFdLuxu4tQlT5ED

