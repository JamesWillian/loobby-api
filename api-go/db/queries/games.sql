-- Cache local do catalogo RAWG. Porta do GameRepository + GameService do Kotlin.

-- name: FindGame :one
-- rating chega como o texto do Postgres -- "0.00", como o Kotlin responde do
-- cache (medido em producao) -- por causa do db.RegisterTypes. genres/platforms
-- chegam como o texto do jsonb, na ordem de chaves do Postgres: a mesma que o
-- Hibernate le e o Jackson re-escreve.
SELECT id,
       slug,
       name,
       background_image,
       released,
       rating,
       metacritic,
       description_raw,
       genres,
       platforms,
       refreshed_at
FROM games
WHERE id = sqlc.arg(id);

-- name: UpsertGameFromSearch :one
-- GameService.upsertFromSearch. Busca nao traz descricao: description_raw fica
-- como esta. raw_payload so e gravado se ainda nao houver um (nao sobrescreve o
-- payload completo de um detalhe). name nulo = o RAWG nao mandou nome: mantem o
-- atual, ou "" num registro novo (como o GameEntity(name = "")).
INSERT INTO games (id, slug, name, background_image, released, rating, metacritic,
                   genres, platforms, raw_payload, cached_at, refreshed_at)
VALUES (sqlc.arg(id), sqlc.narg(slug), COALESCE(sqlc.narg(name)::text, ''),
        sqlc.narg(background_image), sqlc.narg(released),
        sqlc.narg(rating), sqlc.narg(metacritic),
        (sqlc.narg(genres)::text)::jsonb, (sqlc.narg(platforms)::text)::jsonb,
        (sqlc.narg(raw_payload)::text)::jsonb, sqlc.arg(now), sqlc.arg(now))
ON CONFLICT (id) DO UPDATE SET
    name             = COALESCE(sqlc.narg(name)::text, games.name),
    slug             = EXCLUDED.slug,
    background_image = EXCLUDED.background_image,
    released         = EXCLUDED.released,
    rating           = EXCLUDED.rating,
    metacritic       = EXCLUDED.metacritic,
    genres           = EXCLUDED.genres,
    platforms        = EXCLUDED.platforms,
    raw_payload      = COALESCE(games.raw_payload, EXCLUDED.raw_payload),
    refreshed_at     = EXCLUDED.refreshed_at
RETURNING name;

-- name: UpsertGameFromDetail :one
-- GameService.upsertFromDetail: tudo do detalhe, inclusive a descricao -- que
-- pode virar NULL se o RAWG nao mandar nenhuma das duas.
INSERT INTO games (id, slug, name, background_image, released, rating, metacritic,
                   description_raw, genres, platforms, raw_payload, cached_at, refreshed_at)
VALUES (sqlc.arg(id), sqlc.narg(slug), COALESCE(sqlc.narg(name)::text, ''),
        sqlc.narg(background_image), sqlc.narg(released),
        sqlc.narg(rating), sqlc.narg(metacritic), sqlc.narg(description_raw),
        (sqlc.narg(genres)::text)::jsonb, (sqlc.narg(platforms)::text)::jsonb,
        (sqlc.narg(raw_payload)::text)::jsonb, sqlc.arg(now), sqlc.arg(now))
ON CONFLICT (id) DO UPDATE SET
    name             = COALESCE(sqlc.narg(name)::text, games.name),
    slug             = EXCLUDED.slug,
    background_image = EXCLUDED.background_image,
    released         = EXCLUDED.released,
    rating           = EXCLUDED.rating,
    metacritic       = EXCLUDED.metacritic,
    description_raw  = EXCLUDED.description_raw,
    genres           = EXCLUDED.genres,
    platforms        = EXCLUDED.platforms,
    raw_payload      = EXCLUDED.raw_payload,
    refreshed_at     = EXCLUDED.refreshed_at
RETURNING name;
