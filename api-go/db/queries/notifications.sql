-- Tokens de dispositivo e o registro de envios. Porta de DeviceTokenRepository e
-- NotificationDispatchLogRepository.

-- name: UpsertDeviceToken :exec
-- Um token pertence a um aparelho: registrar de novo o move para o usuario atual.
INSERT INTO device_tokens (token, user_id, platform, created_at, updated_at)
VALUES (sqlc.arg(token), sqlc.arg(user_id), sqlc.arg(platform), now(), now())
ON CONFLICT (token) DO UPDATE
SET user_id = EXCLUDED.user_id, platform = EXCLUDED.platform, updated_at = now();

-- name: DeleteDeviceToken :exec
DELETE FROM device_tokens WHERE token = sqlc.arg(token);

-- name: DeleteDeviceTokens :exec
DELETE FROM device_tokens WHERE token = ANY (sqlc.arg(tokens)::text[]);

-- name: FindDeviceTokensByUsers :many
SELECT token FROM device_tokens WHERE user_id = ANY (sqlc.arg(user_ids)::uuid[]);

-- name: DispatchLogExists :one
SELECT EXISTS (SELECT 1 FROM notification_dispatch_log
               WHERE user_id = sqlc.arg(user_id) AND notification_type = sqlc.arg(notification_type)
                 AND reference_id = sqlc.arg(reference_id) AND dispatched_date = sqlc.arg(dispatched_date));

-- name: InsertDispatchLog :execrows
-- A restricao unica e a trava de idempotencia: 0 linhas = outro processo (ou
-- outra execucao) ja registrou este envio hoje, e a notificacao nao sai de novo.
INSERT INTO notification_dispatch_log (id, user_id, notification_type, reference_id, dispatched_date)
VALUES (gen_random_uuid(), sqlc.arg(user_id), sqlc.arg(notification_type),
        sqlc.arg(reference_id), sqlc.arg(dispatched_date))
ON CONFLICT (user_id, notification_type, reference_id, dispatched_date) DO NOTHING;
