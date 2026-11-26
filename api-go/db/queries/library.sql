-- Bibliotecas de jogos da 2.0 (internal/library): a da pessoa e a do grupo.
--
-- owned_platforms (jsonb) entra como texto, (sqlc.arg(x)::text)::jsonb, pelo
-- mesmo motivo dos jsonb de gameplays.sql: o simple_protocol da producao.
--
-- As listas saem pelo nome do jogo, sem diferenciar maiusculas. rawg_platforms
-- e o games.platforms, que o Go traduz para as 5 plataformas da 2.0.

-- name: GameInCatalog :one
SELECT EXISTS (SELECT 1 FROM games WHERE id = sqlc.arg(id));

-- name: ListUserLibrary :many
SELECT l.game_id, g.name, g.background_image AS cover_url, g.platforms AS rawg_platforms,
       l.owned_platforms, l.created_at, l.updated_at
FROM user_library l
JOIN games g ON g.id = l.game_id
WHERE l.user_id = sqlc.arg(user_id)
ORDER BY lower(g.name), l.game_id;

-- name: GetUserLibraryGame :one
SELECT l.game_id, g.name, g.background_image AS cover_url, g.platforms AS rawg_platforms,
       l.owned_platforms, l.created_at, l.updated_at
FROM user_library l
JOIN games g ON g.id = l.game_id
WHERE l.user_id = sqlc.arg(user_id) AND l.game_id = sqlc.arg(game_id);

-- name: UserLibraryHas :one
SELECT EXISTS (SELECT 1 FROM user_library WHERE user_id = sqlc.arg(user_id) AND game_id = sqlc.arg(game_id));

-- name: CountUserLibrary :one
SELECT count(*) FROM user_library WHERE user_id = sqlc.arg(user_id);

-- name: UpsertUserLibraryGame :exec
-- Adiciona o jogo, ou troca o "tenho o jogo" de quem ja tem.
INSERT INTO user_library (user_id, game_id, owned_platforms)
VALUES (sqlc.arg(user_id), sqlc.arg(game_id), (sqlc.arg(owned_platforms)::text)::jsonb)
ON CONFLICT (user_id, game_id) DO UPDATE SET owned_platforms = EXCLUDED.owned_platforms, updated_at = now();

-- name: DeleteUserLibraryGame :exec
DELETE FROM user_library WHERE user_id = sqlc.arg(user_id) AND game_id = sqlc.arg(game_id);

-- name: DeleteUserLibrary :exec
-- Exclusao de conta.
DELETE FROM user_library WHERE user_id = sqlc.arg(user_id);

-- name: ListLibraryOwners :many
-- O match (internal/gameplays): quais destes usuarios tem quais destes jogos
-- em pelo menos uma das plataformas.
SELECT user_id, game_id FROM user_library
WHERE user_id = ANY (sqlc.arg(user_ids)::uuid[])
  AND game_id = ANY (sqlc.arg(game_ids)::text[])
  AND owned_platforms ?| sqlc.arg(platforms)::text[];

-- name: ListGroupLibrary :many
SELECT l.game_id, g.name, g.background_image AS cover_url, g.platforms AS rawg_platforms,
       l.added_by, l.created_at
FROM group_library l
JOIN games g ON g.id = l.game_id
WHERE l.group_id = sqlc.arg(group_id)
ORDER BY lower(g.name), l.game_id;

-- name: GetGroupLibraryGame :one
SELECT l.game_id, g.name, g.background_image AS cover_url, g.platforms AS rawg_platforms,
       l.added_by, l.created_at
FROM group_library l
JOIN games g ON g.id = l.game_id
WHERE l.group_id = sqlc.arg(group_id) AND l.game_id = sqlc.arg(game_id);

-- name: CountGroupLibrary :one
SELECT count(*) FROM group_library WHERE group_id = sqlc.arg(group_id);

-- name: InsertGroupLibraryGame :exec
-- Repetir nao muda nada: quem adicionou primeiro continua no added_by.
INSERT INTO group_library (group_id, game_id, added_by)
VALUES (sqlc.arg(group_id), sqlc.arg(game_id), sqlc.arg(added_by))
ON CONFLICT (group_id, game_id) DO NOTHING;

-- name: DeleteGroupLibraryGame :exec
DELETE FROM group_library WHERE group_id = sqlc.arg(group_id) AND game_id = sqlc.arg(game_id);

-- name: ClearGroupLibraryAddedBy :exec
-- Exclusao de conta: o jogo fica com o grupo, sem quem o adicionou.
UPDATE group_library SET added_by = NULL WHERE added_by = sqlc.arg(user_id)::uuid;
