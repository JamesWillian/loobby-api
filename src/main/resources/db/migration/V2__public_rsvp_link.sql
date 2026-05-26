-- ============================================================================
-- V2: Public RSVP via signed link (Phase 1)
--
-- Permite que convidados confirmem presença em um evento sem autenticar no app,
-- via URL https://loobby.app/c/<token>. Quando a verificação por WhatsApp entrar
-- (Phase 2), a feature flag passa a usar event_rsvp_pending; até lá a tabela
-- existe vazia para evitar nova migration depois.
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 1. users: telefone como identificador secundário
-- ----------------------------------------------------------------------------
-- Nullable porque usuários existentes não têm telefone.
-- Índice unique parcial: só aplica unicidade quando o telefone existe.
-- Formato esperado: E.164 (ex: +5511999998888).
alter table users
    add column if not exists phone_e164 text;

create unique index if not exists ix_users_phone_e164
    on users (phone_e164)
    where phone_e164 is not null;

-- ----------------------------------------------------------------------------
-- 2. event_rsvps: origem e nível de verificação
-- ----------------------------------------------------------------------------
-- source: por onde a confirmação entrou.
--   'APP'           → fluxo padrão autenticado
--   'WEB_LINK'      → confirmou pela URL pública
--   'WHATSAPP_BOT'  → confirmou respondendo no WhatsApp
--
-- verification_level: o quanto a identidade dessa RSVP é confiável.
--   'UNVERIFIED'        → digitou telefone+nome no link, sem prova de posse
--   'PHONE_VERIFIED'    → confirmou no WhatsApp (Phase 2)
--   'APP_AUTHENTICATED' → usuário logado no app
--
-- Valores em UPPERCASE para casar com @Enumerated(EnumType.STRING) do Hibernate,
-- seguindo o padrão de RsvpStatus.
--
-- Defaults garantem que as RSVPs existentes (todas vindas do app) ficam
-- consistentes sem backfill.
alter table event_rsvps
    add column if not exists source text not null default 'APP',
    add column if not exists verification_level text not null default 'APP_AUTHENTICATED';

alter table event_rsvps
    drop constraint if exists event_rsvps_source_ck;
alter table event_rsvps
    add constraint event_rsvps_source_ck
        check (source in ('APP', 'WEB_LINK', 'WHATSAPP_BOT'));

alter table event_rsvps
    drop constraint if exists event_rsvps_verification_level_ck;
alter table event_rsvps
    add constraint event_rsvps_verification_level_ck
        check (verification_level in ('UNVERIFIED', 'PHONE_VERIFIED', 'APP_AUTHENTICATED'));

-- ----------------------------------------------------------------------------
-- 3. event_link_tokens: tokens públicos compartilháveis por evento
-- ----------------------------------------------------------------------------
-- O token bruto NUNCA é armazenado. Guardamos só sha256(token) em token_hash.
-- Validação: cliente envia o token na URL → servidor hasheia → busca aqui.
--
-- expires_at sugerido pela aplicação: data_do_evento + 24h.
-- revoked_at: nullable; quando preenchido, o token deixa de ser aceito.
create table if not exists event_link_tokens (
    id                 uuid        primary key default gen_random_uuid(),
    event_id           uuid        not null references events(id) on delete cascade,
    token_hash         text        not null unique,
    created_by_user_id uuid        not null references users(id) on delete restrict,
    expires_at         timestamptz not null,
    revoked_at         timestamptz,
    created_at         timestamptz not null default now()
);

create index if not exists ix_event_link_tokens_event
    on event_link_tokens (event_id);

-- ----------------------------------------------------------------------------
-- 4. event_rsvp_pending: confirmações aguardando verificação por WhatsApp
-- ----------------------------------------------------------------------------
-- Existe a partir da V2 mas só é populada na Phase 2 (WhatsApp ligado).
-- Diferente de event_rsvps, NÃO referencia users — guardamos phone+nome crus.
-- O usuário só é criado/vinculado no momento da promoção pra event_rsvps,
-- evitando poluir users com gente que nunca confirmou.
--
-- Unicidade por (event_id, phone_e164): mesmo telefone não pode ter duas
-- confirmações pendentes pro mesmo evento. Reenviar OTP é UPDATE, não INSERT.
create table if not exists event_rsvp_pending (
    id              uuid        primary key default gen_random_uuid(),
    event_id        uuid        not null references events(id) on delete cascade,
    phone_e164      text        not null,
    display_name    text        not null,
    whatsapp_msg_id text,
    otp_sent_at     timestamptz,
    otp_attempts    integer     not null default 0,
    expires_at      timestamptz not null,
    created_at      timestamptz not null default now(),
    updated_at      timestamptz not null default now(),
    constraint event_rsvp_pending_event_phone_unique unique (event_id, phone_e164)
);

create index if not exists ix_event_rsvp_pending_phone
    on event_rsvp_pending (phone_e164);

create index if not exists ix_event_rsvp_pending_whatsapp_msg
    on event_rsvp_pending (whatsapp_msg_id)
    where whatsapp_msg_id is not null;

create index if not exists ix_event_rsvp_pending_expires
    on event_rsvp_pending (expires_at);
