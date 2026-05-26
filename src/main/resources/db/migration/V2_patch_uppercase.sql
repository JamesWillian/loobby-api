-- ============================================================================
-- Patch para rodar no Supabase
--
-- Alinha os valores de event_rsvps.source e event_rsvps.verification_level
-- com a convenção UPPERCASE do projeto (Hibernate EnumType.STRING).
--
-- Rodar APENAS se você já aplicou a V2 anterior com valores em lowercase.
-- Pula esse arquivo se sua base ainda não tem essas colunas — nesse caso, use
-- diretamente o V2__public_rsvp_link.sql atual (já corrigido).
-- ============================================================================

-- 1. Derruba as check constraints antigas
alter table event_rsvps drop constraint if exists event_rsvps_source_ck;
alter table event_rsvps drop constraint if exists event_rsvps_verification_level_ck;

-- 2. Atualiza dados existentes para UPPERCASE
update event_rsvps set source = upper(source)
    where source in ('app', 'web_link', 'whatsapp_bot');

update event_rsvps set verification_level = upper(verification_level)
    where verification_level in ('unverified', 'phone_verified', 'app_authenticated');

-- 3. Recoloca check constraints com valores UPPERCASE
alter table event_rsvps
    add constraint event_rsvps_source_ck
        check (source in ('APP', 'WEB_LINK', 'WHATSAPP_BOT'));

alter table event_rsvps
    add constraint event_rsvps_verification_level_ck
        check (verification_level in ('UNVERIFIED', 'PHONE_VERIFIED', 'APP_AUTHENTICATED'));

-- 4. Ajusta os defaults das colunas
alter table event_rsvps
    alter column source set default 'APP';

alter table event_rsvps
    alter column verification_level set default 'APP_AUTHENTICATED';
