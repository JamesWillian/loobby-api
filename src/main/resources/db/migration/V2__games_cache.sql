-- Cache local do catálogo do RAWG (https://rawg.io).
--
-- Decisões de design:
--  * PK = id do RAWG (numérico, armazenado como text para tolerar slugs/ids futuros).
--  * NÃO há FK rígida com gameplay_events.game_id de propósito: o cache pode ser
--    podado/recriado sem riscar o histórico de eventos, e gameplay_events.game_name
--    continua denormalizado para jogos digitados à mão (sem id RAWG) ou removidos do cache.
--  * raw_payload guarda a resposta completa do RAWG em jsonb -> expor campos novos
--    no app no futuro não exige nova migração.
--  * refreshed_at decide o TTL; cached_at é só auditoria ("quando entrou no sistema").
--
-- Flyway está desabilitado (flyway.enabled=false) e jpa.ddl-auto=validate.
-- APLIQUE ESTE SCRIPT MANUALMENTE NO BANCO ANTES DE SUBIR A APLICAÇÃO,
-- senão o Hibernate validate vai falhar no boot ao não encontrar a tabela "games".

create extension if not exists pg_trgm;

create table if not exists games (
    id               text primary key,            -- id do RAWG
    slug             text unique,
    name             text        not null,
    background_image text,
    released         date,
    rating           numeric(4, 2),
    metacritic       integer,
    description_raw  text,
    genres           jsonb,
    platforms        jsonb,
    raw_payload      jsonb,                        -- resposta completa do RAWG
    cached_at        timestamptz not null default now(),
    refreshed_at     timestamptz not null default now()
);

-- Índice trigram para busca local rápida por nome no futuro
-- (só vai ao RAWG quando o cache local devolver pouca coisa).
create index if not exists games_name_trgm_idx
    on games using gin (name gin_trgm_ops);
