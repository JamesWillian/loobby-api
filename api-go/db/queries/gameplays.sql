-- Gameplays da 2.0 (internal/gameplays). Os nomes FindGameplay, InsertGameplay
-- e UpdateGameplay ja sao da v1 (gameplay_events), entao aqui os verbos sao
-- outros: Get, Create, Lock, Set, List.
--
-- "Ativa" = ended_at NULL e scheduled_at depois de `cutoff` (agora menos as
-- 24 h do encerramento automatico). O Go passa o cutoff, para a regra das 24 h
-- ficar num lugar so (gameplays.AutoEndAfter).
--
-- platforms (jsonb) entra como texto: (sqlc.arg(platforms)::text)::jsonb, como
-- os jsonb de games.sql. Sem o ::text o sqlc tipa o parametro como []byte, e o
-- simple_protocol da producao (ver db.Connect) manda []byte como bytea ('\x5b...'):
-- o Postgres recusa com "invalid input syntax for type json" (SQLSTATE 22P02).

-- name: CreateGameplay :exec
INSERT INTO gameplays (id, group_id, creator_id, name, description, mode, platforms, scheduled_at,
                       invite_code, game_id, game_name, finalists, multi_vote, voting_round)
VALUES (sqlc.arg(id), sqlc.narg(group_id), sqlc.arg(creator_id), sqlc.arg(name), sqlc.narg(description),
        sqlc.arg(mode), (sqlc.arg(platforms)::text)::jsonb, sqlc.arg(scheduled_at), sqlc.narg(invite_code),
        sqlc.narg(game_id), sqlc.narg(game_name), sqlc.narg(finalists), sqlc.narg(multi_vote),
        sqlc.narg(voting_round));

-- name: GetGameplay :one
SELECT * FROM gameplays WHERE id = sqlc.arg(id);

-- name: LockGameplay :one
-- Toda escrita numa gameplay comeca por aqui: a trava da linha serializa votos,
-- limite de jogos, desempate e encerramento.
SELECT * FROM gameplays WHERE id = sqlc.arg(id) FOR UPDATE;

-- name: GetGameplayByInviteCode :one
SELECT * FROM gameplays WHERE invite_code = sqlc.arg(invite_code)::text;

-- name: GameplayInviteCodeExists :one
SELECT EXISTS (SELECT 1 FROM gameplays WHERE invite_code = sqlc.arg(invite_code)::text);

-- name: SetGameplayDetails :exec
UPDATE gameplays
SET name = sqlc.arg(name), description = sqlc.narg(description), scheduled_at = sqlc.arg(scheduled_at),
    platforms = (sqlc.arg(platforms)::text)::jsonb, game_id = sqlc.narg(game_id), game_name = sqlc.narg(game_name),
    finalists = sqlc.narg(finalists), multi_vote = sqlc.narg(multi_vote)
WHERE id = sqlc.arg(id);

-- name: SetGameplayVotingRound :exec
UPDATE gameplays SET voting_round = sqlc.arg(voting_round) WHERE id = sqlc.arg(id);

-- name: EndGameplay :exec
UPDATE gameplays SET ended_at = sqlc.arg(ended_at), ended_by = sqlc.narg(ended_by) WHERE id = sqlc.arg(id);

-- name: AutoEndGameplays :execrows
-- O job de encerramento automatico. O ended_at e o horario em que ela encerrou
-- de fato (o marcado mais as 24 h), nao o da execucao do job.
UPDATE gameplays
SET ended_at = scheduled_at + sqlc.arg(after_seconds)::integer * interval '1 second'
WHERE ended_at IS NULL AND scheduled_at <= sqlc.arg(cutoff);

-- name: SetGameplayCreator :exec
UPDATE gameplays SET creator_id = sqlc.arg(creator_id) WHERE id = sqlc.arg(id);

-- name: DeleteGameplay :exec
DELETE FROM gameplays WHERE id = sqlc.arg(id);

-- name: ListGameplaysByCreator :many
SELECT * FROM gameplays WHERE creator_id = sqlc.arg(creator_id) ORDER BY created_at, id;

-- name: ListActiveGroupGameplays :many
SELECT * FROM gameplays
WHERE group_id = sqlc.arg(group_id)::uuid AND ended_at IS NULL AND scheduled_at > sqlc.arg(cutoff)
ORDER BY scheduled_at, id;

-- name: ListEndedGroupGameplays :many
-- Encerradas, da que encerrou por ultimo para a primeira. A que passou das
-- 24 h e o job ainda nao carimbou entra pelo horario em que encerrou de fato.
SELECT * FROM gameplays
WHERE group_id = sqlc.arg(group_id)::uuid
  AND (ended_at IS NOT NULL OR scheduled_at <= sqlc.arg(cutoff))
ORDER BY COALESCE(ended_at, scheduled_at + sqlc.arg(after_seconds)::integer * interval '1 second') DESC, id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListEndedGroupGameplaysBefore :many
-- A pagina seguinte: as que encerraram antes da gameplay (before_at, before_id).
SELECT * FROM gameplays
WHERE group_id = sqlc.arg(group_id)::uuid
  AND (ended_at IS NOT NULL OR scheduled_at <= sqlc.arg(cutoff))
  AND (COALESCE(ended_at, scheduled_at + sqlc.arg(after_seconds)::integer * interval '1 second'), id)
      < (sqlc.arg(before_at)::pg_catalog.timestamptz, sqlc.arg(before_id)::uuid)
ORDER BY COALESCE(ended_at, scheduled_at + sqlc.arg(after_seconds)::integer * interval '1 second') DESC, id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListLobbyGameplays :many
-- O Lobby: as ativas dos grupos do usuario e as unicas em que ele entrou.
SELECT g.* FROM gameplays g
WHERE g.ended_at IS NULL AND g.scheduled_at > sqlc.arg(cutoff)
  AND CASE WHEN g.group_id IS NOT NULL
           THEN EXISTS (SELECT 1 FROM group_members m
                        WHERE m.group_id = g.group_id AND m.user_id = sqlc.arg(user_id))
           ELSE EXISTS (SELECT 1 FROM gameplay_participants p
                        WHERE p.gameplay_id = g.id AND p.user_id = sqlc.arg(user_id))
      END
ORDER BY g.scheduled_at, g.id;

-- name: ListHistoryGameplays :many
-- O Historico: as encerradas em que a pessoa marcou Vou, na ordem das
-- encerradas do grupo. A do grupo so entra enquanto ela e membro, porque o
-- detalhe da 404 a quem saiu; se ela voltar, a gameplay volta junto.
SELECT g.* FROM gameplays g
JOIN gameplay_participants p ON p.gameplay_id = g.id
WHERE p.user_id = sqlc.arg(user_id) AND p.status = 'GOING'
  AND (g.ended_at IS NOT NULL OR g.scheduled_at <= sqlc.arg(cutoff))
  AND (g.group_id IS NULL OR EXISTS (SELECT 1 FROM group_members m
                                     WHERE m.group_id = g.group_id AND m.user_id = sqlc.arg(user_id)))
ORDER BY COALESCE(g.ended_at, g.scheduled_at + sqlc.arg(after_seconds)::integer * interval '1 second') DESC, g.id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListHistoryGameplaysBefore :many
-- A pagina seguinte: as que encerraram antes da gameplay (before_at, before_id).
SELECT g.* FROM gameplays g
JOIN gameplay_participants p ON p.gameplay_id = g.id
WHERE p.user_id = sqlc.arg(user_id) AND p.status = 'GOING'
  AND (g.ended_at IS NOT NULL OR g.scheduled_at <= sqlc.arg(cutoff))
  AND (g.group_id IS NULL OR EXISTS (SELECT 1 FROM group_members m
                                     WHERE m.group_id = g.group_id AND m.user_id = sqlc.arg(user_id)))
  AND (COALESCE(g.ended_at, g.scheduled_at + sqlc.arg(after_seconds)::integer * interval '1 second'), g.id)
      < (sqlc.arg(before_at)::pg_catalog.timestamptz, sqlc.arg(before_id)::uuid)
ORDER BY COALESCE(g.ended_at, g.scheduled_at + sqlc.arg(after_seconds)::integer * interval '1 second') DESC, g.id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListGameplaysStartingBetween :many
-- Lembrete e pendentes do dia: as que nao foram encerradas e comecam no intervalo.
SELECT * FROM gameplays
WHERE ended_at IS NULL AND scheduled_at BETWEEN sqlc.arg(start_at) AND sqlc.arg(end_at)
ORDER BY scheduled_at, id;

-- name: GetParticipant :one
SELECT * FROM gameplay_participants WHERE gameplay_id = sqlc.arg(gameplay_id) AND user_id = sqlc.arg(user_id);

-- name: UpsertParticipant :exec
INSERT INTO gameplay_participants (gameplay_id, user_id, status)
VALUES (sqlc.arg(gameplay_id), sqlc.arg(user_id), sqlc.arg(status))
ON CONFLICT (gameplay_id, user_id) DO UPDATE SET status = EXCLUDED.status, updated_at = now();

-- name: JoinGameplay :exec
-- Entrada pelo convite da unica. Quem ja estava continua com o status que tinha.
INSERT INTO gameplay_participants (gameplay_id, user_id, status)
VALUES (sqlc.arg(gameplay_id), sqlc.arg(user_id), 'PENDING')
ON CONFLICT (gameplay_id, user_id) DO NOTHING;

-- name: DeleteParticipant :exec
DELETE FROM gameplay_participants WHERE gameplay_id = sqlc.arg(gameplay_id) AND user_id = sqlc.arg(user_id);

-- name: ListParticipantsOfGameplays :many
-- Na ordem em que cada um respondeu pela ultima vez.
SELECT * FROM gameplay_participants
WHERE gameplay_id = ANY (sqlc.arg(gameplay_ids)::uuid[])
ORDER BY updated_at, user_id;

-- name: FindHeirParticipant :one
-- Exclusao de conta: quem herda a unica -- o participante mais antigo que nao e
-- anonimo nem quem esta saindo.
SELECT p.user_id FROM gameplay_participants p JOIN users u ON u.id = p.user_id
WHERE p.gameplay_id = sqlc.arg(gameplay_id) AND p.user_id <> sqlc.arg(user_id) AND u.auth_provider <> 0
ORDER BY p.created_at, p.user_id
LIMIT 1;

-- name: DeleteParticipationsByUser :exec
DELETE FROM gameplay_participants WHERE user_id = sqlc.arg(user_id);

-- name: DeleteParticipationInGameplays :exec
DELETE FROM gameplay_participants
WHERE user_id = sqlc.arg(user_id) AND gameplay_id = ANY (sqlc.arg(gameplay_ids)::uuid[]);

-- name: AddGameplayGame :exec
-- clock_timestamp, e nao o now() do DEFAULT: os jogos que entram junto com a
-- gameplay sao gravados na mesma transacao, e com now() teriam o mesmo
-- created_at -- a lista sairia na ordem dos ids, e nao na em que foram escolhidos.
INSERT INTO gameplay_games (id, gameplay_id, game_id, name, added_by, created_at)
VALUES (sqlc.arg(id), sqlc.arg(gameplay_id), sqlc.narg(game_id), sqlc.arg(name), sqlc.narg(added_by),
        clock_timestamp());

-- name: GameplayGameTaken :one
-- Repetido: o mesmo jogo do catalogo ou o mesmo nome, sem diferenciar maiusculas.
SELECT EXISTS (SELECT 1 FROM gameplay_games
               WHERE gameplay_id = sqlc.arg(gameplay_id)
                 AND (game_id = sqlc.narg(game_id) OR lower(name) = lower(sqlc.arg(name)::text)));

-- name: CountGameplayGames :one
SELECT count(*) FROM gameplay_games WHERE gameplay_id = sqlc.arg(gameplay_id);

-- name: GetGameplayGame :one
SELECT * FROM gameplay_games WHERE id = sqlc.arg(id) AND gameplay_id = sqlc.arg(gameplay_id);

-- name: DeleteGameplayGame :exec
-- Os votos no jogo vao junto (ON DELETE CASCADE).
DELETE FROM gameplay_games WHERE id = sqlc.arg(id);

-- name: ListGameplayGames :many
-- Na ordem em que entraram, com a capa do cache do catalogo quando o jogo e de la.
SELECT gg.id, gg.gameplay_id, gg.game_id, gg.name, gg.added_by, gg.last_round, gg.created_at,
       g.background_image AS cover_url
FROM gameplay_games gg
LEFT JOIN games g ON g.id = gg.game_id
WHERE gg.gameplay_id = ANY (sqlc.arg(gameplay_ids)::uuid[])
ORDER BY gg.created_at, gg.id;

-- name: PromoteGameplayGames :exec
-- Desempate: os empatados seguem para a rodada nova.
UPDATE gameplay_games SET last_round = sqlc.arg(last_round) WHERE id = ANY (sqlc.arg(ids)::uuid[]);

-- name: ClearGameplayGamesAddedBy :exec
UPDATE gameplay_games SET added_by = NULL WHERE added_by = sqlc.arg(user_id)::uuid;

-- name: ListGameCovers :many
SELECT id, background_image FROM games WHERE id = ANY (sqlc.arg(ids)::text[]);

-- name: InsertGameplayVote :exec
INSERT INTO gameplay_votes (gameplay_game_id, voting_round, user_id, gameplay_id)
VALUES (sqlc.arg(gameplay_game_id), sqlc.arg(voting_round), sqlc.arg(user_id), sqlc.arg(gameplay_id));

-- name: DeleteMyRoundVotes :exec
DELETE FROM gameplay_votes
WHERE gameplay_id = sqlc.arg(gameplay_id) AND voting_round = sqlc.arg(voting_round)
  AND user_id = sqlc.arg(user_id);

-- name: ListGameplayVotes :many
SELECT * FROM gameplay_votes
WHERE gameplay_id = ANY (sqlc.arg(gameplay_ids)::uuid[])
ORDER BY voting_round, created_at, user_id;

-- name: DeleteVotesByUser :exec
DELETE FROM gameplay_votes WHERE user_id = sqlc.arg(user_id);

-- name: DeleteOpenRoundVotesInGameplays :exec
-- So a rodada aberta de cada gameplay: as que ja congelaram ficam como estavam.
DELETE FROM gameplay_votes v
USING gameplays g
WHERE g.id = v.gameplay_id AND v.voting_round = g.voting_round
  AND v.user_id = sqlc.arg(user_id) AND v.gameplay_id = ANY (sqlc.arg(gameplay_ids)::uuid[]);

-- name: InsertGameplayComment :one
INSERT INTO gameplay_comments (id, gameplay_id, user_id, body)
VALUES (sqlc.arg(id), sqlc.arg(gameplay_id), sqlc.arg(user_id), sqlc.arg(body))
RETURNING created_at;

-- name: GetGameplayComment :one
SELECT * FROM gameplay_comments WHERE id = sqlc.arg(id) AND gameplay_id = sqlc.arg(gameplay_id);

-- name: DeleteGameplayComment :exec
-- As denuncias do comentario vao junto (ON DELETE CASCADE).
DELETE FROM gameplay_comments WHERE id = sqlc.arg(id);

-- name: ListGameplayComments :many
-- Os mais novos primeiro, com o autor e se quem le ja denunciou.
SELECT c.id, c.gameplay_id, c.user_id, c.body, c.created_at,
       u.username, u.displayname, u.avatar_url,
       EXISTS (SELECT 1 FROM gameplay_comment_reports r
               WHERE r.comment_id = c.id AND r.reporter_id = sqlc.arg(viewer_id)) AS reported_by_me
FROM gameplay_comments c
JOIN users u ON u.id = c.user_id
WHERE c.gameplay_id = sqlc.arg(gameplay_id)
ORDER BY c.created_at DESC, c.id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListGameplayCommentsBefore :many
-- A pagina seguinte: os escritos antes do comentario (before_at, before_id).
SELECT c.id, c.gameplay_id, c.user_id, c.body, c.created_at,
       u.username, u.displayname, u.avatar_url,
       EXISTS (SELECT 1 FROM gameplay_comment_reports r
               WHERE r.comment_id = c.id AND r.reporter_id = sqlc.arg(viewer_id)) AS reported_by_me
FROM gameplay_comments c
JOIN users u ON u.id = c.user_id
WHERE c.gameplay_id = sqlc.arg(gameplay_id)
  AND (c.created_at, c.id) < (sqlc.arg(before_at)::pg_catalog.timestamptz, sqlc.arg(before_id)::uuid)
ORDER BY c.created_at DESC, c.id DESC
LIMIT sqlc.arg(max_rows);

-- name: DeleteCommentsByUser :exec
DELETE FROM gameplay_comments WHERE user_id = sqlc.arg(user_id);

-- name: InsertCommentReport :execrows
-- Uma denuncia por pessoa e comentario: repetir nao faz nada.
INSERT INTO gameplay_comment_reports (id, comment_id, reporter_id, reason)
VALUES (sqlc.arg(id), sqlc.arg(comment_id), sqlc.arg(reporter_id), sqlc.narg(reason))
ON CONFLICT (comment_id, reporter_id) DO NOTHING;

-- name: DeleteReportsByUser :exec
DELETE FROM gameplay_comment_reports WHERE reporter_id = sqlc.arg(user_id);
