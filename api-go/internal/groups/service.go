// Package groups e a porta do GroupController, do GroupService e do
// GroupMemberService do Kotlin: grupos, convite por codigo e membros.
package groups

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/files"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
)

// groupResponse: GroupResponse.
type groupResponse struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	InviteCode string         `json:"inviteCode"`
	ImageURL   *string        `json:"imageUrl"`
	OwnerID    string         `json:"ownerId"`
	CreatedAt  *httpx.Instant `json:"createdAt"`
}

// memberResponse: GroupMemberResponse.
type memberResponse struct {
	UserID      string  `json:"userId"`
	Username    string  `json:"username"`
	Displayname *string `json:"displayname"`
	AvatarURL   *string `json:"avatarUrl"`
	IsOwner     bool    `json:"isOwner"`
}

func response(g sqlc.Group) groupResponse {
	t := httpx.Instant(g.CreatedAt)
	return groupResponse{
		ID: g.ID, Name: g.Name, InviteCode: g.InviteCode, ImageURL: g.ImageUrl,
		OwnerID: g.OwnerID, CreatedAt: &t,
	}
}

type Service struct {
	pool    *pgxpool.Pool
	q       *sqlc.Queries
	storage *files.Storage
}

func NewService(pool *pgxpool.Pool, storage *files.Storage) *Service {
	return &Service{pool: pool, q: sqlc.New(pool), storage: storage}
}

var errGroupNotFound = httpx.BadRequest("Group not found")

func findGroup(ctx context.Context, q *sqlc.Queries, id string) (sqlc.Group, error) {
	g, err := q.FindGroup(ctx, id)
	if db.NotFound(err) {
		return sqlc.Group{}, errGroupNotFound
	}
	return g, err
}

func assertOwner(userID string, g sqlc.Group) error {
	if g.OwnerID != userID {
		return httpx.Forbidden("Only the group owner can perform this action")
	}
	return nil
}

// Create: GroupService.createGroup. O gatilho do banco inclui o dono como membro.
//
// createdAt sai null na resposta: o Kotlin devolve a entidade que acabou de
// salvar, sem reler a coluna preenchida pelo banco (golden groups-create).
func (s *Service) Create(ctx context.Context, ownerID, name string, imageURL *string) (groupResponse, error) {
	code, err := s.inviteCode(ctx)
	if err != nil {
		return groupResponse{}, err
	}
	g := sqlc.InsertGroupParams{ID: ids.New(), Name: name, InviteCode: code, ImageUrl: imageURL, OwnerID: ownerID}
	if err := s.q.InsertGroup(ctx, g); err != nil {
		return groupResponse{}, err
	}
	return groupResponse{ID: g.ID, Name: name, InviteCode: code, ImageURL: imageURL, OwnerID: ownerID}, nil
}

// inviteCode: "$-" + 6 caracteres de A-Z0-9, repetindo enquanto ja existir.
// (O Kotlin procura o codigo SEM o prefixo "$-", entao la a checagem nunca
// acha nada e uma colisao viraria 500 pela restricao unica. Aqui a checagem
// usa o codigo completo.)
func (s *Service) inviteCode(ctx context.Context) (string, error) {
	for {
		code := "$-" + ids.Code(6)
		exists, err := s.q.GroupInviteCodeExists(ctx, code)
		if err != nil {
			return "", err
		}
		if !exists {
			return code, nil
		}
	}
}

// ListForUser: GroupService.listGroupsForUser -- os grupos de que e dono e
// depois os de que e membro, sem repetir e sem ordenacao (nenhuma das duas
// consultas do Kotlin tem ORDER BY).
func (s *Service) ListForUser(ctx context.Context, userID string) ([]groupResponse, error) {
	owned, err := s.q.FindGroupsByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	memberOf, err := s.q.FindMemberGroupIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	var joined []sqlc.Group
	if len(memberOf) > 0 {
		if joined, err = s.q.FindGroupsByIDs(ctx, memberOf); err != nil {
			return nil, err
		}
	}
	seen := make(map[string]bool, len(owned)+len(joined))
	out := make([]groupResponse, 0, len(owned)+len(joined))
	for _, g := range append(owned, joined...) {
		if !seen[g.ID] {
			seen[g.ID] = true
			out = append(out, response(g))
		}
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id string) (groupResponse, error) {
	g, err := findGroup(ctx, s.q, id)
	if err != nil {
		return groupResponse{}, err
	}
	return response(g), nil
}

func (s *Service) GetByCode(ctx context.Context, code string) (groupResponse, error) {
	g, err := s.q.FindGroupByInviteCode(ctx, code)
	if db.NotFound(err) {
		return groupResponse{}, httpx.BadRequest("Invalid Invite Code")
	}
	if err != nil {
		return groupResponse{}, err
	}
	return response(g), nil
}

// Rename: GroupService.updateGroupName. So o dono.
func (s *Service) Rename(ctx context.Context, userID, groupID, name string) (groupResponse, error) {
	g, err := findGroup(ctx, s.q, groupID)
	if err != nil {
		return groupResponse{}, err
	}
	if err := assertOwner(userID, g); err != nil {
		return groupResponse{}, err
	}
	if err := s.q.UpdateGroupName(ctx, sqlc.UpdateGroupNameParams{ID: groupID, Name: name}); err != nil {
		return groupResponse{}, err
	}
	g.Name = name
	return response(g), nil
}

// UpdateImage: GroupService.updateGroupImage. So o dono; o arquivo so e
// gravado depois da checagem de permissao.
func (s *Service) UpdateImage(ctx context.Context, userID, groupID string, upload *files.Upload) (groupResponse, error) {
	g, err := findGroup(ctx, s.q, groupID)
	if err != nil {
		return groupResponse{}, err
	}
	if err := assertOwner(userID, g); err != nil {
		return groupResponse{}, err
	}
	url, err := s.storage.Store(upload, "groups", groupID)
	if err != nil {
		return groupResponse{}, err
	}
	if err := s.q.UpdateGroupImage(ctx, sqlc.UpdateGroupImageParams{ID: groupID, ImageUrl: &url}); err != nil {
		return groupResponse{}, err
	}
	g.ImageUrl = &url
	return response(g), nil
}

// Delete: GroupService.deleteGroup. So o dono.
func (s *Service) Delete(ctx context.Context, userID, groupID string) error {
	g, err := findGroup(ctx, s.q, groupID)
	if err != nil {
		return err
	}
	if err := assertOwner(userID, g); err != nil {
		return err
	}
	return s.q.DeleteGroup(ctx, groupID)
}

// Join: GroupMemberService.joinGroup. Idempotente.
func (s *Service) Join(ctx context.Context, userID, groupID string) error {
	if _, err := findGroup(ctx, s.q, groupID); err != nil {
		return err
	}
	return s.q.InsertMember(ctx, sqlc.InsertMemberParams{GroupID: groupID, UserID: userID})
}

// Leave: GroupMemberService.leaveGroup. O dono nao pode sair do proprio grupo.
func (s *Service) Leave(ctx context.Context, userID, groupID string) error {
	g, err := findGroup(ctx, s.q, groupID)
	if err != nil {
		return err
	}
	if g.OwnerID == userID {
		return httpx.BadRequest("Group owner cannot leave the group")
	}
	return s.q.DeleteMember(ctx, sqlc.DeleteMemberParams{GroupID: groupID, UserID: userID})
}

// RemoveMember: GroupMemberService.removeMember. So o dono remove, e nao a si mesmo.
func (s *Service) RemoveMember(ctx context.Context, requesterID, groupID, memberID string) error {
	g, err := findGroup(ctx, s.q, groupID)
	if err != nil {
		return err
	}
	if g.OwnerID != requesterID {
		return httpx.Forbidden("Only the group owner can remove members")
	}
	if memberID == g.OwnerID {
		return httpx.BadRequest("Cannot remove the group owner.")
	}
	member := sqlc.MemberExistsParams{GroupID: groupID, UserID: memberID}
	isMember, err := s.q.MemberExists(ctx, member)
	if err != nil {
		return err
	}
	if !isMember {
		return httpx.BadRequest("User is not a member of this group")
	}
	return s.q.DeleteMember(ctx, sqlc.DeleteMemberParams(member))
}

// Members: GroupMemberService.listMembers. Na ordem em que o banco devolve os
// vinculos; membro cujo usuario sumiu e pulado.
func (s *Service) Members(ctx context.Context, groupID string) ([]memberResponse, error) {
	g, err := findGroup(ctx, s.q, groupID)
	if err != nil {
		return nil, err
	}
	userIDs, err := s.q.FindMemberUserIDs(ctx, groupID)
	if err != nil {
		return nil, err
	}
	out := make([]memberResponse, 0, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	users, err := s.q.FindUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]sqlc.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}
	for _, id := range userIDs {
		u, ok := byID[id]
		if !ok {
			continue
		}
		out = append(out, memberResponse{
			UserID: u.ID, Username: u.Username, Displayname: &u.Displayname,
			AvatarURL: u.AvatarUrl, IsOwner: u.ID == g.OwnerID,
		})
	}
	return out, nil
}
