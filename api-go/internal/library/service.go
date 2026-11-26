package library

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/games"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
	"github.com/JamesWillian/loobby-api/api-go/internal/platform"
)

type Service struct {
	pool     *pgxpool.Pool
	q        *sqlc.Queries
	catalog  Catalog
	maxUser  int // trocaveis em teste
	maxGroup int
}

func NewService(pool *pgxpool.Pool, catalog Catalog) *Service {
	return &Service{
		pool: pool, q: sqlc.New(pool), catalog: catalog,
		maxUser: maxUserGames, maxGroup: maxGroupGames,
	}
}

// Mine: a biblioteca de userID, pelo nome do jogo.
func (s *Service) Mine(ctx context.Context, userID string) ([]item, error) {
	rows, err := s.q.ListUserLibrary(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]item, 0, len(rows))
	for _, r := range rows {
		out = append(out, itemOf(sqlc.GetUserLibraryGameRow(r)))
	}
	return out, nil
}

// Put adiciona o jogo a biblioteca, ou troca o "tenho o jogo" de quem ja o
// tem. owned ja vem validado (ver putBody).
func (s *Service) Put(ctx context.Context, userID, gameID string, owned []string) (item, error) {
	if err := s.ensureGame(ctx, gameID); err != nil {
		return item{}, err
	}
	var out item
	err := db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		has, err := q.UserLibraryHas(ctx, sqlc.UserLibraryHasParams{UserID: userID, GameID: gameID})
		if err != nil {
			return err
		}
		if !has {
			n, err := q.CountUserLibrary(ctx, userID)
			if err != nil {
				return err
			}
			if n >= int64(s.maxUser) {
				return errUserFull
			}
		}
		err = q.UpsertUserLibraryGame(ctx, sqlc.UpsertUserLibraryGameParams{
			UserID: userID, GameID: gameID, OwnedPlatforms: platform.Encode(owned),
		})
		if err != nil {
			return err
		}
		row, err := q.GetUserLibraryGame(ctx, sqlc.GetUserLibraryGameParams{UserID: userID, GameID: gameID})
		out = itemOf(row)
		return err
	})
	return out, err
}

// Remove tira o jogo da biblioteca. Tirar o que nao esta la nao e erro: o app
// pode repetir o pedido.
func (s *Service) Remove(ctx context.Context, userID, gameID string) error {
	return s.q.DeleteUserLibraryGame(ctx, sqlc.DeleteUserLibraryGameParams{UserID: userID, GameID: gameID})
}

// Group: a biblioteca do grupo, pelo nome do jogo, para os membros (o anonimo
// inclusive).
func (s *Service) Group(ctx context.Context, userID, groupID string) ([]groupItem, error) {
	if err := member(ctx, s.q, userID, groupID, false); err != nil {
		return nil, err
	}
	rows, err := s.q.ListGroupLibrary(ctx, groupID)
	if err != nil {
		return nil, err
	}
	var who []string
	for _, r := range rows {
		if r.AddedBy != nil {
			who = append(who, *r.AddedBy)
		}
	}
	users, err := people(ctx, s.q, who)
	if err != nil {
		return nil, err
	}
	out := make([]groupItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, groupItemOf(sqlc.GetGroupLibraryGameRow(r), users))
	}
	return out, nil
}

// PutGroup adiciona o jogo a biblioteca do grupo: qualquer membro que nao seja
// anonimo. Repetir nao muda nada, nem quem adicionou.
func (s *Service) PutGroup(ctx context.Context, userID, groupID, gameID string) (groupItem, error) {
	if err := member(ctx, s.q, userID, groupID, true); err != nil {
		return groupItem{}, err
	}
	if err := s.ensureGame(ctx, gameID); err != nil {
		return groupItem{}, err
	}
	var row sqlc.GetGroupLibraryGameRow
	err := db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		key := sqlc.GetGroupLibraryGameParams{GroupID: groupID, GameID: gameID}
		_, err := q.GetGroupLibraryGame(ctx, key)
		switch {
		case db.NotFound(err):
			n, err := q.CountGroupLibrary(ctx, groupID)
			if err != nil {
				return err
			}
			if n >= int64(s.maxGroup) {
				return errGroupFull
			}
			err = q.InsertGroupLibraryGame(ctx, sqlc.InsertGroupLibraryGameParams{
				GroupID: groupID, GameID: gameID, AddedBy: &userID,
			})
			if err != nil {
				return err
			}
		case err != nil:
			return err
		}
		row, err = q.GetGroupLibraryGame(ctx, key)
		return err
	})
	if err != nil {
		return groupItem{}, err
	}
	var who []string
	if row.AddedBy != nil {
		who = append(who, *row.AddedBy)
	}
	users, err := people(ctx, s.q, who)
	if err != nil {
		return groupItem{}, err
	}
	return groupItemOf(row, users), nil
}

// RemoveGroup tira o jogo da biblioteca do grupo: qualquer membro que nao seja
// anonimo. Tirar o que nao esta la nao e erro.
func (s *Service) RemoveGroup(ctx context.Context, userID, groupID, gameID string) error {
	if err := member(ctx, s.q, userID, groupID, true); err != nil {
		return err
	}
	return s.q.DeleteGroupLibraryGame(ctx, sqlc.DeleteGroupLibraryGameParams{GroupID: groupID, GameID: gameID})
}

// ensureGame: o jogo tem que estar no catalogo. O que ainda nao esta no cache
// e buscado no RAWG, como no GET /games/{id}; o que o RAWG nao conhece, ou com
// o RAWG fora do ar, da 404. Na pratica o app adiciona a partir da busca, que
// ja grava cada resultado no cache.
func (s *Service) ensureGame(ctx context.Context, gameID string) error {
	cached, err := s.q.GameInCatalog(ctx, gameID)
	if err != nil || cached {
		return err
	}
	_, err = s.catalog.Details(ctx, gameID)
	if errors.Is(err, games.ErrNotFound) {
		return errGameNotFound
	}
	return err
}

// member: o grupo existe e userID e membro dele. write: so uma conta que nao e
// anonima. Sem o grupo, 404; fora dele, 403, como nas gameplays do grupo.
func member(ctx context.Context, q *sqlc.Queries, userID, groupID string, write bool) error {
	if _, err := q.FindGroup(ctx, groupID); db.NotFound(err) {
		return errGroupNotFound
	} else if err != nil {
		return err
	}
	ok, err := q.MemberExists(ctx, sqlc.MemberExistsParams{GroupID: groupID, UserID: userID})
	if err != nil {
		return err
	}
	if !ok {
		return errNotMember
	}
	if !write {
		return nil
	}
	u, err := q.FindUser(ctx, userID)
	if db.NotFound(err) {
		return errUserNotFound
	}
	if err != nil {
		return err
	}
	if u.AuthProvider == 0 {
		return errAnonymous
	}
	return nil
}

// people le os usuarios de uma vez, sem repetir ids.
func people(ctx context.Context, q *sqlc.Queries, ids []string) (map[string]sqlc.User, error) {
	out := map[string]sqlc.User{}
	seen := map[string]bool{}
	var unique []string
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	if len(unique) == 0 {
		return out, nil
	}
	rows, err := q.FindUsersByIDs(ctx, unique)
	if err != nil {
		return nil, err
	}
	for _, u := range rows {
		out[u.ID] = u
	}
	return out, nil
}

func itemOf(r sqlc.GetUserLibraryGameRow) item {
	return item{
		GameID: r.GameID, Name: r.Name, CoverURL: r.CoverUrl,
		AvailablePlatforms: platform.FromRAWG(r.RawgPlatforms), OwnedPlatforms: platform.Decode(r.OwnedPlatforms),
		AddedAt: httpx.Instant(r.CreatedAt), UpdatedAt: httpx.Instant(r.UpdatedAt),
	}
}

func groupItemOf(r sqlc.GetGroupLibraryGameRow, users map[string]sqlc.User) groupItem {
	out := groupItem{
		GameID: r.GameID, Name: r.Name, CoverURL: r.CoverUrl,
		AvailablePlatforms: platform.FromRAWG(r.RawgPlatforms), AddedAt: httpx.Instant(r.CreatedAt),
	}
	if r.AddedBy != nil {
		ref := userRef{ID: *r.AddedBy}
		if u, ok := users[*r.AddedBy]; ok {
			ref = userRef{ID: u.ID, Username: u.Username, Displayname: u.Displayname, AvatarURL: u.AvatarUrl}
		}
		out.AddedBy = &ref
	}
	return out
}
