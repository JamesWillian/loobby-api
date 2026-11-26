// Package library e a biblioteca de jogos da 2.0, em /v2. Sao duas:
//
//   - a da pessoa (GET/PUT/DELETE /v2/me/library): os jogos dela, com o "tenho
//     o jogo" em ownedPlatforms. Todo usuario tem a sua, o anonimo inclusive, e
//     so ele a le. Os outros veem apenas o match, no detalhe das gameplays que
//     dividem com ele (internal/gameplays);
//   - a do grupo (/v2/groups/{groupId}/library): os jogos que o grupo joga,
//     para montar a votacao depressa. Os membros leem; os que nao sao anonimos
//     adicionam e tiram, como os jogos da votacao do grupo.
//
// As duas so aceitam jogo do catalogo (o cache do RAWG, a tabela games), pelo
// id do RAWG. As tabelas sao as da migracao 00004. Os erros seguem os da v2:
// 400 com fieldErrors, 403, 404 e 409, com as mensagens em portugues.
package library

import (
	"context"

	"github.com/JamesWillian/loobby-api/api-go/internal/games"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

const (
	maxUserGames  = 500 // jogos na biblioteca de uma pessoa
	maxGroupGames = 100 // jogos na biblioteca de um grupo
	maxBody       = 4 << 10
	maxGameID     = 20 // digitos do id do RAWG
)

// userRef: quem adicionou o jogo a biblioteca do grupo. O mesmo formato do
// usuario nas gameplays.
type userRef struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	Displayname string  `json:"displayname"`
	AvatarURL   *string `json:"avatarUrl"`
}

// item: um jogo da biblioteca da pessoa.
type item struct {
	GameID             string        `json:"gameId"` // o id do RAWG, o mesmo de GET /games/{id}
	Name               string        `json:"name"`
	CoverURL           *string       `json:"coverUrl"`
	AvailablePlatforms []string      `json:"availablePlatforms"` // onde o jogo existe, pelo RAWG; [] = o RAWG nao diz
	OwnedPlatforms     []string      `json:"ownedPlatforms"`     // o "tenho o jogo"; [] = so na biblioteca
	AddedAt            httpx.Instant `json:"addedAt"`
	UpdatedAt          httpx.Instant `json:"updatedAt"`
}

// groupItem: um jogo da biblioteca do grupo.
type groupItem struct {
	GameID             string        `json:"gameId"`
	Name               string        `json:"name"`
	CoverURL           *string       `json:"coverUrl"`
	AvailablePlatforms []string      `json:"availablePlatforms"`
	AddedBy            *userRef      `json:"addedBy"` // null: a conta de quem adicionou foi excluida
	AddedAt            httpx.Instant `json:"addedAt"`
}

// Catalog e o que a biblioteca pede ao modulo de jogos: trazer ao cache um jogo
// que ainda nao esta la. O games.Service.Details busca no RAWG e grava.
type Catalog interface {
	Details(ctx context.Context, id string) (games.Details, error)
}
