create extension if not exists pgcrypto;

do $$ begin
  create type event_type as enum ('GAMEPLAY', 'SPORT', 'PARTY');
exception when duplicate_object then null; end $$;

do $$ begin
  create type rsvp_status as enum ('YES', 'NO', 'MAYBE', 'RESERVE');
exception when duplicate_object then null; end $$;

create table if not exists users (
  id uuid primary key default gen_random_uuid(),
  username text not null unique,
  displayname text not null,
  avatar_url text,
  created_at timestamptz not null default now()
);

create table if not exists user_credentials (
  user_id uuid primary key references users(id) on delete cascade,
  email text unique,
  password_hash text not null,
  roles text not null default 'USER', -- CSV: USER,ADMIN...
  created_at timestamptz not null default now()
);

create table if not exists groups (
  id uuid primary key default gen_random_uuid(),
  name text not null,
  invite_code text not null unique,
  image_url text,
  owner_id uuid not null references users(id) on delete restrict,
  created_at timestamptz not null default now()
);

create table if not exists group_members (
  id uuid primary key default gen_random_uuid(),
  group_id uuid not null references groups(id) on delete cascade,
  user_id uuid not null references users(id) on delete cascade,
  constraint group_members_unique unique (group_id, user_id)
);

create table if not exists events (
  id uuid primary key default gen_random_uuid(),
  event_type text not null,
  group_id uuid references groups(id) on delete set null,
  is_instant boolean not null default false,
  owner_id uuid not null references users(id) on delete restrict,
  scheduled_datetime timestamptz not null,
  name text not null,
  description text,
  invite_code text not null unique,
  created_at timestamptz not null default now(),
  constraint events_instant_group_ck check (
    (is_instant = true and group_id is null) or (is_instant = false)
  )
);

create table if not exists gameplay_events (
  event_id uuid primary key references events(id) on delete cascade,
  game_id text,
  game_name text not null
);

create table if not exists sport_events (
  event_id uuid primary key references events(id) on delete cascade,
  duration_minutes integer not null check (duration_minutes > 0),
  arena text,
  price_per_player numeric(12,2) not null default 0 check (price_per_player >= 0),
  max_players integer check (max_players is null or max_players > 0),
  accept_reserve boolean not null default false
);

create table if not exists event_rsvps (
  event_id uuid not null references events(id) on delete cascade,
  user_id uuid not null references users(id) on delete cascade,
  status text not null,
  is_paid boolean not null default false,
  obs text,
  created_at timestamptz not null default now(),
  primary key (event_id, user_id)
);

create table if not exists event_teams (
  id uuid primary key default gen_random_uuid(),
  event_id uuid not null references events(id) on delete cascade,
  "order" integer not null default 0,
  name text not null,
  color text,
  constraint event_teams_event_order_unique unique (event_id, "order"),
  constraint event_teams_event_name_unique unique (event_id, name)
);

create table if not exists team_players (
  team_id uuid not null references event_teams(id) on delete cascade,
  user_id uuid not null references users(id) on delete cascade,
  role text,
  primary key (team_id, user_id)
);
