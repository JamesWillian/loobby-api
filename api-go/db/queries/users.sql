-- Usuarios e credenciais. Porta de UsersRepository, UserCredentialsRepository,
-- UserGoogleCredentialsRepository e UserFeedRepository do Kotlin.

-- name: FindUser :one
SELECT id, username, displayname, avatar_url, created_at, auth_provider
FROM users
WHERE id = sqlc.arg(id);

-- name: FindUsersByIDs :many
-- findAllById: sem ORDER BY, como no Kotlin -- quem chama indexa por id.
SELECT id, username, displayname, avatar_url, created_at, auth_provider
FROM users
WHERE id = ANY (sqlc.arg(ids)::uuid[]);

-- name: UsernameTaken :one
-- existsByUsernameIgnoreCase: o Hibernate compara com upper() dos dois lados.
SELECT EXISTS (SELECT 1 FROM users WHERE upper(username) = upper(sqlc.arg(username)::text));

-- name: InsertUser :exec
INSERT INTO users (id, username, displayname, avatar_url, auth_provider)
VALUES (sqlc.arg(id), sqlc.arg(username), sqlc.arg(displayname), sqlc.narg(avatar_url), sqlc.arg(auth_provider));

-- name: UpdateUserProfile :exec
-- displayname aceita NULL de proposito: a coluna e NOT NULL e o Kotlin tenta
-- gravar null quando o cliente manda displayname em branco. O banco recusa e a
-- resposta e 500, dos dois lados (contract/ACHADOS.md).
UPDATE users
SET username    = sqlc.arg(username),
    displayname = sqlc.narg(displayname),
    avatar_url  = sqlc.narg(avatar_url)
WHERE id = sqlc.arg(id);

-- name: UpdateUserAvatar :exec
UPDATE users SET avatar_url = sqlc.narg(avatar_url) WHERE id = sqlc.arg(id);

-- name: SetUserAuthProvider :exec
UPDATE users SET auth_provider = sqlc.arg(auth_provider) WHERE id = sqlc.arg(id);

-- name: AnonymizeUser :exec
-- Exclusao de conta: o registro fica (integridade referencial), sem dados pessoais.
UPDATE users SET username = '', displayname = '[deleted]', avatar_url = NULL WHERE id = sqlc.arg(id);

-- name: UserFeed :many
-- A view ja vem ordenada (sort_bucket, scheduled_datetime); o Kotlin nao reordena.
SELECT id, user_id, name, image_url, entry_type, is_finished
FROM user_feed
WHERE user_id = sqlc.arg(user_id);

-- name: FindCredentials :one
SELECT * FROM user_credentials WHERE user_id = sqlc.arg(user_id);

-- name: FindCredentialsByEmail :one
SELECT * FROM user_credentials WHERE email = sqlc.arg(email);

-- name: FindCredentialsByVerificationToken :one
SELECT * FROM user_credentials WHERE email_verification_token = sqlc.arg(token)::text;

-- name: FindCredentialsByResetToken :one
SELECT * FROM user_credentials WHERE password_reset_token = sqlc.arg(token)::text;

-- name: CredentialsExist :one
SELECT EXISTS (SELECT 1 FROM user_credentials WHERE user_id = sqlc.arg(user_id));

-- name: InsertCredentials :exec
INSERT INTO user_credentials (user_id, email, password_hash, roles)
VALUES (sqlc.arg(user_id), sqlc.arg(email), sqlc.arg(password_hash), 'USER');

-- name: UpdatePasswordHash :exec
UPDATE user_credentials SET password_hash = sqlc.arg(password_hash) WHERE user_id = sqlc.arg(user_id);

-- name: SetVerificationToken :exec
UPDATE user_credentials
SET email_verification_token = sqlc.arg(token)::text,
    email_verification_expires_at = sqlc.arg(expires_at)
WHERE user_id = sqlc.arg(user_id);

-- name: MarkEmailVerified :exec
UPDATE user_credentials
SET email_verified = true, email_verification_token = NULL, email_verification_expires_at = NULL
WHERE user_id = sqlc.arg(user_id);

-- name: SetResetToken :exec
UPDATE user_credentials
SET password_reset_token = sqlc.arg(token)::text,
    password_reset_expires_at = sqlc.arg(expires_at)
WHERE user_id = sqlc.arg(user_id);

-- name: ResetPassword :exec
UPDATE user_credentials
SET password_hash = sqlc.arg(password_hash), password_reset_token = NULL, password_reset_expires_at = NULL
WHERE user_id = sqlc.arg(user_id);

-- name: DeleteCredentials :exec
DELETE FROM user_credentials WHERE user_id = sqlc.arg(user_id);

-- name: FindGoogleCredentials :one
SELECT * FROM user_google_credentials WHERE user_id = sqlc.arg(user_id);

-- name: FindGoogleCredentialsByGoogleID :one
SELECT * FROM user_google_credentials WHERE google_id = sqlc.arg(google_id);

-- name: InsertGoogleCredentials :exec
INSERT INTO user_google_credentials (user_id, google_id, email)
VALUES (sqlc.arg(user_id), sqlc.arg(google_id), sqlc.arg(email));
