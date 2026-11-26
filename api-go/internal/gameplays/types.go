// Package gameplays e a API da 2.0: gameplays do grupo e unicas (sem grupo),
// participacao (Vou / Nao vou / Pendente), votacao de jogos com desempate,
// comentarios com moderacao e encerramento, manual ou automatico.
//
// Tudo e novo: as rotas ficam em /v2, as tabelas sao as da migracao 00002, e
// nada da v1 (eventos, RSVPs) e lido ou escrito. As tabelas compartilhadas sao
// users, groups, group_members, games e device_tokens.
//
// Diferente da v1, aqui nao ha contrato do Kotlin a reproduzir: "nao existe" e
// 404, corpo ilegivel e 400, conflito de estado e 409, e as mensagens saem em
// portugues, prontas para o app mostrar. O envelope de erro e o mesmo.
package gameplays

import (
	"context"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

// Modos e status, como saem no JSON e ficam no banco. As plataformas sao as
// do pacote platform.
const (
	ModeVoting = "VOTING" // votacao entre jogos
	ModeFixed  = "FIXED"  // jogo definido
	ModeFree   = "FREE"   // livre

	StatusGoing    = "GOING"     // Vou
	StatusNotGoing = "NOT_GOING" // Nao vou
	StatusPending  = "PENDING"   // Pendente
)

var (
	modes    = []string{ModeVoting, ModeFixed, ModeFree}
	statuses = []string{StatusGoing, StatusNotGoing, StatusPending}
)

const (
	// AutoEndAfter: a gameplay encerra sozinha 24 h depois do horario marcado.
	AutoEndAfter = 24 * time.Hour

	maxGames      = 20              // jogos na votacao
	maxFinalists  = 20              // o maximo que o criador pode pedir
	maxPlatforms  = 1               // por enquanto; o JSON ja aceita mais (crossplay)
	pastTolerance = 5 * time.Minute // "comecar agora" chega um pouco no passado
	inviteCodeLen = 8               // "$-" + 8; o do grupo tem 6, o do evento v1 e XXXX-XXXX

	maxName        = 100
	maxDescription = 500
	maxGameName    = 150
	maxComment     = 1000
	maxReason      = 500

	defaultPage      = 20 // encerradas do grupo e Historico
	maxPage          = 50
	defaultComments  = 50
	maxCommentsPage  = 100
	goingAvatarCount = 5 // avatares de quem vai, nas listas
)

// userRef: quem aparece numa gameplay (criador, participante, eleitor, autor).
type userRef struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	Displayname string  `json:"displayname"`
	AvatarURL   *string `json:"avatarUrl"`
}

// gameRef: um jogo. gameId e o id do catalogo (o do RAWG, o mesmo de
// GET /games/{id}); null quando foi digitado a mao, e ai nao ha capa.
type gameRef struct {
	GameID   *string `json:"gameId"`
	Name     string  `json:"name"`
	CoverURL *string `json:"coverUrl"`
}

// permissions diz ao app quais botoes mostrar para quem le. O servidor confere
// de novo cada acao; isto so evita que o app repita as regras.
type permissions struct {
	Respond         bool `json:"respond"`         // Vou / Nao vou / Pendente
	Vote            bool `json:"vote"`            // votar na rodada aberta
	EditGames       bool `json:"editGames"`       // adicionar e remover jogos da votacao
	Comment         bool `json:"comment"`         // escrever comentario
	Manage          bool `json:"manage"`          // editar e Encerrar
	ConfigureVoting bool `json:"configureVoting"` // finalistas e voto multiplo
	Tiebreak        bool `json:"tiebreak"`        // Desempatar
	Leave           bool `json:"leave"`           // sair da unica
	Moderate        bool `json:"moderate"`        // apagar comentario de qualquer um
}

type counts struct {
	Going    int `json:"going"`
	NotGoing int `json:"notGoing"`
	Pending  int `json:"pending"` // quem pode responder e nao respondeu (anonimo nao conta)
}

type participantResponse struct {
	User        userRef `json:"user"`
	Status      string  `json:"status"`
	IsAnonymous bool    `json:"isAnonymous"`
}

type roundVotes struct {
	Round  int       `json:"round"`
	Votes  int       `json:"votes"`
	Voters []userRef `json:"voters"`
}

// votingGame: um jogo da votacao. id e o desta entrada (o que vai no voto);
// gameId, o do catalogo.
type votingGame struct {
	ID        string       `json:"id"`
	GameID    *string      `json:"gameId"`
	Name      string       `json:"name"`
	CoverURL  *string      `json:"coverUrl"`
	AddedBy   *string      `json:"addedBy"`
	Candidate bool         `json:"candidate"` // concorre na rodada aberta
	MyVote    bool         `json:"myVote"`    // quem le votou nele na rodada aberta
	Rounds    []roundVotes `json:"rounds"`    // uma entrada por rodada em que concorreu
	Owners    []userRef    `json:"owners"`    // o match: quem vai e tem o jogo (ver owners)
}

// fixedGameResponse: o jogo definido, no detalhe, com o match.
type fixedGameResponse struct {
	gameRef
	Owners []userRef `json:"owners"` // quem vai e tem o jogo (ver owners)
}

type resultResponse struct {
	Round       int      `json:"round"`
	FinalistIDs []string `json:"finalistIds"`
	Tie         bool     `json:"tie"` // com 1 finalista: empate no 1o lugar
}

type votingResponse struct {
	Finalists int            `json:"finalists"`
	MultiVote bool           `json:"multiVote"`
	Locked    bool           `json:"locked"` // finalistas e voto multiplo ja nao mudam
	Round     int            `json:"round"`  // 1 = principal; 2 em diante = desempates
	Games     []votingGame   `json:"games"`
	Result    resultResponse `json:"result"`
}

// gameplayResponse: o detalhe, que tambem e a resposta de toda acao sobre a
// gameplay -- o app troca a tela inteira pelo que voltou.
type gameplayResponse struct {
	ID           string                `json:"id"`
	GroupID      *string               `json:"groupId"`
	GroupName    *string               `json:"groupName"`
	Name         string                `json:"name"`
	Description  *string               `json:"description"`
	Mode         string                `json:"mode"`
	Platforms    []string              `json:"platforms"`
	ScheduledAt  httpx.Instant         `json:"scheduledAt"`
	AutoEndAt    httpx.Instant         `json:"autoEndAt"`
	Status       string                `json:"status"` // ACTIVE ou ENDED
	EndedAt      *httpx.Instant        `json:"endedAt"`
	EndedBy      *userRef              `json:"endedBy"` // null com endedAt preenchido = automatico
	InviteCode   *string               `json:"inviteCode"`
	Creator      userRef               `json:"creator"`
	CreatedAt    httpx.Instant         `json:"createdAt"`
	MyStatus     string                `json:"myStatus"`
	Permissions  permissions           `json:"permissions"`
	Counts       counts                `json:"counts"`
	Participants []participantResponse `json:"participants"`
	Game         *fixedGameResponse    `json:"game"`   // FIXED
	Voting       *votingResponse       `json:"voting"` // VOTING
}

// gameplaySummary: um item das listas (Lobby, gameplays do grupo, Historico).
type gameplaySummary struct {
	ID           string         `json:"id"`
	GroupID      *string        `json:"groupId"`
	GroupName    *string        `json:"groupName"`
	Name         string         `json:"name"`
	Mode         string         `json:"mode"`
	Platforms    []string       `json:"platforms"`
	ScheduledAt  httpx.Instant  `json:"scheduledAt"`
	AutoEndAt    httpx.Instant  `json:"autoEndAt"`
	Status       string         `json:"status"`
	EndedAt      *httpx.Instant `json:"endedAt"`
	Creator      userRef        `json:"creator"`
	MyStatus     string         `json:"myStatus"`
	GoingCount   int            `json:"goingCount"`
	GoingAvatars []*string      `json:"goingAvatars"` // os primeiros; null para quem nao tem avatar
	Game         *gameRef       `json:"game"`         // FIXED
	Leading      []gameRef      `json:"leading"`      // VOTING: os finalistas (ver summaries)
	Tie          bool           `json:"tie"`          // VOTING: com 1 finalista, empate no 1o lugar
}

// invitePreview: o que o convite mostra antes de a pessoa entrar.
type invitePreview struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Mode        string         `json:"mode"`
	Platforms   []string       `json:"platforms"`
	ScheduledAt httpx.Instant  `json:"scheduledAt"`
	AutoEndAt   httpx.Instant  `json:"autoEndAt"`
	Status      string         `json:"status"`
	EndedAt     *httpx.Instant `json:"endedAt"`
	Creator     userRef        `json:"creator"`
	GoingCount  int            `json:"goingCount"`
	Joined      bool           `json:"joined"`
}

type commentResponse struct {
	ID           string        `json:"id"`
	GameplayID   string        `json:"gameplayId"`
	Author       userRef       `json:"author"`
	Body         string        `json:"body"`
	CreatedAt    httpx.Instant `json:"createdAt"`
	CanDelete    bool          `json:"canDelete"`
	ReportedByMe bool          `json:"reportedByMe"` // para o app esconder de quem denunciou
}

// gameInput: um jogo pedido pelo app, do catalogo (gameId) ou digitado.
type gameInput struct {
	GameID *string
	Name   string
}

// createInput: os campos ja validados de uma gameplay nova.
type createInput struct {
	Name        string
	Description *string
	Mode        string
	Platforms   []string
	ScheduledAt time.Time
	Game        *gameInput  // FIXED
	Finalists   int         // VOTING
	MultiVote   bool        // VOTING
	Games       []gameInput // VOTING: os jogos que ja entram na votacao
}

// patchInput: nil = nao muda.
type patchInput struct {
	Name             *string
	Description      *string
	ClearDescription bool
	ScheduledAt      *time.Time
	Platforms        []string
	Game             *gameInput
	Finalists        *int
	MultiVote        *bool
}

// GameCache e o que o modulo pede ao de jogos: deixar a capa no cache.
type GameCache interface {
	EnsureCached(id string)
}

// runner roda um push fora da requisicao. Trocavel em teste.
type runner func(name string, fn func(ctx context.Context))
