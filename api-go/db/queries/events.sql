-- Eventos, detalhes por tipo e RSVPs. Porta de EventRepository,
-- GameplayEventRepository, SportEventRepository e EventRsvpRepository.

-- name: InsertEvent :exec
-- O gatilho trg_auto_rsvp_owner cria o RSVP YES do dono.
INSERT INTO events (id, event_type, group_id, is_instant, owner_id, scheduled_datetime,
                    name, description, invite_code)
VALUES (sqlc.arg(id), sqlc.arg(event_type), sqlc.narg(group_id), sqlc.arg(is_instant),
        sqlc.arg(owner_id), sqlc.arg(scheduled_datetime), sqlc.arg(name),
        sqlc.narg(description), sqlc.arg(invite_code));

-- name: FindEvent :one
SELECT * FROM events WHERE id = sqlc.arg(id);

-- name: FindEventByInviteCode :one
SELECT * FROM events WHERE invite_code = sqlc.arg(invite_code);

-- name: EventInviteCodeExists :one
SELECT EXISTS (SELECT 1 FROM events WHERE invite_code = sqlc.arg(invite_code));

-- name: EventExists :one
SELECT EXISTS (SELECT 1 FROM events WHERE id = sqlc.arg(id));

-- name: FindEventsByGroup :many
SELECT * FROM events WHERE group_id = sqlc.arg(group_id)::uuid ORDER BY scheduled_datetime DESC;

-- name: FindInstantEventsByOwner :many
SELECT * FROM events WHERE owner_id = sqlc.arg(owner_id) AND is_instant = true;

-- name: FindEventsBetween :many
SELECT * FROM events
WHERE scheduled_datetime BETWEEN sqlc.arg(start_at) AND sqlc.arg(end_at);

-- name: UpdateEvent :exec
UPDATE events
SET name = sqlc.arg(name), description = sqlc.narg(description),
    scheduled_datetime = sqlc.arg(scheduled_datetime)
WHERE id = sqlc.arg(id);

-- name: UpdateEventOwner :exec
UPDATE events SET owner_id = sqlc.arg(owner_id) WHERE id = sqlc.arg(id);

-- name: DeleteEvent :exec
DELETE FROM events WHERE id = sqlc.arg(id);

-- name: InsertGameplay :exec
INSERT INTO gameplay_events (event_id, game_id, game_name)
VALUES (sqlc.arg(event_id), sqlc.narg(game_id), sqlc.arg(game_name));

-- name: FindGameplay :one
SELECT * FROM gameplay_events WHERE event_id = sqlc.arg(event_id);

-- name: UpdateGameplay :exec
UPDATE gameplay_events SET game_id = sqlc.narg(game_id), game_name = sqlc.arg(game_name)
WHERE event_id = sqlc.arg(event_id);

-- name: InsertSport :exec
INSERT INTO sport_events (event_id, duration_minutes, arena, price_per_player, max_players, accept_reserve)
VALUES (sqlc.arg(event_id), sqlc.arg(duration_minutes), sqlc.narg(arena),
        sqlc.arg(price_per_player), sqlc.narg(max_players), sqlc.arg(accept_reserve));

-- name: FindSport :one
-- price_per_player chega como o texto do Postgres, com a escala da coluna
-- ("25.50", "0.00") -- ver db.RegisterTypes.
SELECT * FROM sport_events WHERE event_id = sqlc.arg(event_id);

-- name: FindSportsByEvents :many
SELECT * FROM sport_events WHERE event_id = ANY (sqlc.arg(event_ids)::uuid[]);

-- name: UpdateSport :exec
UPDATE sport_events
SET duration_minutes = sqlc.arg(duration_minutes), arena = sqlc.narg(arena),
    price_per_player = sqlc.arg(price_per_player), max_players = sqlc.narg(max_players),
    accept_reserve = sqlc.arg(accept_reserve)
WHERE event_id = sqlc.arg(event_id);

-- name: FindRsvp :one
SELECT * FROM event_rsvps WHERE event_id = sqlc.arg(event_id) AND user_id = sqlc.arg(user_id);

-- name: InsertRsvp :exec
INSERT INTO event_rsvps (event_id, user_id, status, is_paid, obs)
VALUES (sqlc.arg(event_id), sqlc.arg(user_id), sqlc.arg(status), sqlc.arg(is_paid), sqlc.narg(obs));

-- name: UpdateRsvp :exec
UPDATE event_rsvps SET status = sqlc.arg(status), is_paid = sqlc.arg(is_paid), obs = sqlc.narg(obs)
WHERE event_id = sqlc.arg(event_id) AND user_id = sqlc.arg(user_id);

-- name: DeleteRsvp :exec
DELETE FROM event_rsvps WHERE event_id = sqlc.arg(event_id) AND user_id = sqlc.arg(user_id);

-- name: FindRsvpsByEvent :many
SELECT * FROM event_rsvps WHERE event_id = sqlc.arg(event_id) ORDER BY created_at ASC;

-- name: FindRsvpsByEventsAndUser :many
SELECT * FROM event_rsvps
WHERE event_id = ANY (sqlc.arg(event_ids)::uuid[]) AND user_id = sqlc.arg(user_id);

-- name: FindRsvpsByEventsAndStatus :many
-- Sem ORDER BY, como no Kotlin: os "5 primeiros confirmados" saem na ordem do banco.
SELECT * FROM event_rsvps
WHERE event_id = ANY (sqlc.arg(event_ids)::uuid[]) AND status = sqlc.arg(status);

-- name: FindUnpaidRsvps :many
SELECT * FROM event_rsvps
WHERE event_id = ANY (sqlc.arg(event_ids)::uuid[]) AND status = sqlc.arg(status) AND is_paid = false;

-- name: FindFirstOtherRsvp :one
-- Proximo dono de um evento instantaneo: o confirmado mais antigo que nao e o dono atual.
SELECT user_id FROM event_rsvps
WHERE event_id = sqlc.arg(event_id) AND user_id <> sqlc.arg(user_id) AND status = sqlc.arg(status)
ORDER BY created_at ASC
LIMIT 1;

-- name: ClearRsvpObsByUser :exec
UPDATE event_rsvps SET obs = NULL WHERE user_id = sqlc.arg(user_id);
