create extension if not exists pgcrypto;

do $$ begin
  create type event_type as enum ('GAME_ONLINE', 'SPORTS', 'PARTY');
exception when duplicate_object then null; end $$;

do $$ begin
  create type rsvp_status as enum ('YES', 'NO', 'MAYBE_RESERVE');
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