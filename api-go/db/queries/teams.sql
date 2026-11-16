-- Times de um evento. Porta de EventTeamRepository e TeamPlayerRepository.

-- name: InsertTeam :exec
INSERT INTO event_teams (id, event_id, "order", name, color)
VALUES (sqlc.arg(id), sqlc.arg(event_id), sqlc.arg(order_index), sqlc.arg(name), sqlc.narg(color));

-- name: FindTeam :one
SELECT * FROM event_teams WHERE id = sqlc.arg(id);

-- name: FindTeamsByEvent :many
SELECT * FROM event_teams WHERE event_id = sqlc.arg(event_id) ORDER BY "order" ASC;

-- name: UpdateTeam :exec
UPDATE event_teams SET name = sqlc.arg(name), color = sqlc.narg(color), "order" = sqlc.arg(order_index)
WHERE id = sqlc.arg(id);

-- name: DeleteTeam :exec
DELETE FROM event_teams WHERE id = sqlc.arg(id);

-- name: DeleteTeamsByEvent :exec
DELETE FROM event_teams WHERE event_id = sqlc.arg(event_id);

-- name: UpsertTeamPlayer :exec
-- O save() do JPA numa chave composta ja existente vira UPDATE: o papel e sobrescrito.
INSERT INTO team_players (team_id, user_id, role)
VALUES (sqlc.arg(team_id), sqlc.arg(user_id), sqlc.narg(role))
ON CONFLICT (team_id, user_id) DO UPDATE SET role = EXCLUDED.role;

-- name: FindTeamPlayer :one
SELECT * FROM team_players WHERE team_id = sqlc.arg(team_id) AND user_id = sqlc.arg(user_id);

-- name: FindPlayersByTeams :many
SELECT * FROM team_players WHERE team_id = ANY (sqlc.arg(team_ids)::uuid[]);

-- name: DeleteTeamPlayer :exec
DELETE FROM team_players WHERE team_id = sqlc.arg(team_id) AND user_id = sqlc.arg(user_id);
