-- Grupos e membros. Porta de GroupRepository e GroupMemberRepository.
-- Nenhuma listagem tem ORDER BY, como no Kotlin: a ordem e a do banco.

-- name: InsertGroup :exec
-- O gatilho trg_add_owner_to_group_members inclui o dono como membro.
INSERT INTO groups (id, name, invite_code, image_url, owner_id)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(invite_code), sqlc.narg(image_url), sqlc.arg(owner_id));

-- name: FindGroup :one
SELECT * FROM groups WHERE id = sqlc.arg(id);

-- name: FindGroupByInviteCode :one
SELECT * FROM groups WHERE invite_code = sqlc.arg(invite_code);

-- name: GroupInviteCodeExists :one
SELECT EXISTS (SELECT 1 FROM groups WHERE invite_code = sqlc.arg(invite_code));

-- name: FindGroupsByOwner :many
SELECT * FROM groups WHERE owner_id = sqlc.arg(owner_id);

-- name: FindGroupsByIDs :many
SELECT * FROM groups WHERE id = ANY (sqlc.arg(ids)::uuid[]);

-- name: UpdateGroupName :exec
UPDATE groups SET name = sqlc.arg(name) WHERE id = sqlc.arg(id);

-- name: UpdateGroupImage :exec
UPDATE groups SET image_url = sqlc.narg(image_url) WHERE id = sqlc.arg(id);

-- name: UpdateGroupOwner :exec
UPDATE groups SET owner_id = sqlc.arg(owner_id) WHERE id = sqlc.arg(id);

-- name: DeleteGroup :exec
-- ON DELETE CASCADE leva membros, eventos, RSVPs e times junto.
DELETE FROM groups WHERE id = sqlc.arg(id);

-- name: FindMemberGroupIDs :many
SELECT group_id FROM group_members WHERE user_id = sqlc.arg(user_id);

-- name: FindMemberUserIDs :many
SELECT user_id FROM group_members WHERE group_id = sqlc.arg(group_id);

-- name: MemberExists :one
SELECT EXISTS (SELECT 1 FROM group_members
               WHERE group_id = sqlc.arg(group_id) AND user_id = sqlc.arg(user_id));

-- name: InsertMember :exec
INSERT INTO group_members (id, group_id, user_id)
VALUES (gen_random_uuid(), sqlc.arg(group_id), sqlc.arg(user_id))
ON CONFLICT (group_id, user_id) DO NOTHING;

-- name: DeleteMember :exec
DELETE FROM group_members WHERE group_id = sqlc.arg(group_id) AND user_id = sqlc.arg(user_id);

-- name: FindFirstOtherMember :one
-- findFirstByGroupIdAndUserIdNot: qualquer outro membro, sem ordem definida.
SELECT user_id FROM group_members
WHERE group_id = sqlc.arg(group_id) AND user_id <> sqlc.arg(user_id)
LIMIT 1;
