package gameplays

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

// A v2 le o corpo com o encoding/json, e nao com o httpx.Object: aqui nao ha
// Jackson a imitar. O que nao for JSON, ou nao tiver o tipo certo ("mode": 2,
// "scheduledAt" fora do RFC 3339), e 400, e nao o 500 da v1.

const maxBody = 1 << 20

// readJSON le o corpo em dst. optional: corpo vazio vale (a denuncia sem
// motivo). Falhou, ja respondeu 400 e devolve false.
func readJSON(w http.ResponseWriter, r *http.Request, dst any, optional bool) bool {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err == nil && optional && len(bytes.TrimSpace(raw)) == 0 {
		return true
	}
	mt, _, mtErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	isJSON := mtErr == nil && (mt == "application/json" ||
		(strings.HasPrefix(mt, "application/") && strings.HasSuffix(mt, "+json")))
	if err != nil || len(raw) > maxBody || !isJSON || json.Unmarshal(raw, dst) != nil {
		httpx.Fail(w, r, errBadBody)
		return false
	}
	return true
}

// opt distingue, no PATCH, o campo ausente (Set false) do null (Null true).
type opt[T any] struct {
	Set  bool
	Null bool
	Val  T
}

func (o *opt[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(bytes.TrimSpace(b)) == "null" {
		o.Null = true
		return nil
	}
	return json.Unmarshal(b, &o.Val)
}

type gameBody struct {
	GameID *string `json:"gameId"`
	Name   *string `json:"name"`
}

type votingBody struct {
	Finalists *int  `json:"finalists"`
	MultiVote *bool `json:"multiVote"`
}

type createBody struct {
	Name        *string     `json:"name"`
	Description *string     `json:"description"`
	Mode        *string     `json:"mode"`
	Platforms   []string    `json:"platforms"`
	ScheduledAt *time.Time  `json:"scheduledAt"`
	Game        *gameBody   `json:"game"`
	Voting      *votingBody `json:"voting"`
	Games       []gameBody  `json:"games"`
}

type patchBody struct {
	Name        *string       `json:"name"`
	Description opt[string]   `json:"description"`
	ScheduledAt *time.Time    `json:"scheduledAt"`
	Platforms   opt[[]string] `json:"platforms"`
	Game        opt[gameBody] `json:"game"`
	Voting      *votingBody   `json:"voting"`
}

func runes(s string) int { return utf8.RuneCountInString(s) }

// cleanText: sem espacos nas pontas e com os do meio reduzidos a um -- e assim
// que nome de gameplay e de jogo sao gravados e comparados.
func cleanText(s string) string { return strings.Join(strings.Fields(s), " ") }

func checkName(f *fieldErrors, field string, v *string, max int, missing string) string {
	if v == nil || cleanText(*v) == "" {
		f.add(field, missing)
		return ""
	}
	name := cleanText(*v)
	if runes(name) > max {
		f.add(field, "Use até "+itoa(max)+" caracteres")
	}
	return name
}

// checkDescription: vazio vira null.
func checkDescription(f *fieldErrors, v string) *string {
	d := strings.TrimSpace(v)
	if d == "" {
		return nil
	}
	if runes(d) > maxDescription {
		f.add("description", "Use até "+itoa(maxDescription)+" caracteres")
	}
	return &d
}

func checkPlatforms(f *fieldErrors, list []string) []string {
	switch {
	case len(list) == 0:
		f.add("platforms", "Escolha a plataforma")
		return nil
	case len(list) > maxPlatforms:
		f.add("platforms", "Por enquanto, só uma plataforma por gameplay")
		return nil
	}
	seen := map[string]bool{}
	for _, p := range list {
		if !slices.Contains(platforms, p) {
			f.add("platforms", "Plataforma inválida: use PC, PLAYSTATION, XBOX, SWITCH ou MOBILE")
			return nil
		}
		if seen[p] {
			f.add("platforms", "Plataforma repetida")
			return nil
		}
		seen[p] = true
	}
	return list
}

func checkScheduled(f *fieldErrors, t *time.Time, now time.Time) time.Time {
	if t == nil {
		f.add("scheduledAt", "Informe o horário")
		return time.Time{}
	}
	if t.Before(now.Add(-pastTolerance)) {
		f.add("scheduledAt", "Esse horário já passou")
	}
	return *t
}

func checkGame(f *fieldErrors, field string, b gameBody) gameInput {
	in := gameInput{Name: checkName(f, field+".name", b.Name, maxGameName, "Informe o nome do jogo")}
	if b.GameID != nil {
		if id := strings.TrimSpace(*b.GameID); id != "" {
			if runes(id) > 64 {
				f.add(field+".gameId", "Id de jogo inválido")
			}
			in.GameID = &id
		}
	}
	return in
}

func checkFinalists(f *fieldErrors, n int) {
	if n < 1 || n > maxFinalists {
		f.add("voting.finalists", "Finalistas: de 1 a "+itoa(maxFinalists))
	}
}

// repeatedGame: o mesmo jogo do catalogo ou o mesmo nome, sem diferenciar
// maiusculas -- a regra do indice unico de gameplay_games.
func repeatedGame(list []gameInput) bool {
	ids, names := map[string]bool{}, map[string]bool{}
	for _, g := range list {
		key := strings.ToLower(g.Name)
		if names[key] || (g.GameID != nil && ids[*g.GameID]) {
			return true
		}
		names[key] = true
		if g.GameID != nil {
			ids[*g.GameID] = true
		}
	}
	return false
}

func (b createBody) validate(now time.Time) (createInput, *fieldErrors) {
	f := &fieldErrors{}
	in := createInput{Name: checkName(f, "name", b.Name, maxName, "Informe o nome"), Finalists: 1}
	if b.Description != nil {
		in.Description = checkDescription(f, *b.Description)
	}
	if b.Mode == nil || !slices.Contains(modes, *b.Mode) {
		f.add("mode", "Modo inválido: use VOTING, FIXED ou FREE")
	} else {
		in.Mode = *b.Mode
	}
	in.Platforms = checkPlatforms(f, b.Platforms)
	in.ScheduledAt = checkScheduled(f, b.ScheduledAt, now)

	switch {
	case in.Mode == ModeFixed && b.Game == nil:
		f.add("game", "Informe o jogo")
	case in.Mode == ModeFixed:
		g := checkGame(f, "game", *b.Game)
		in.Game = &g
	case b.Game != nil && in.Mode != "":
		f.add("game", "Só a gameplay de jogo definido tem jogo")
	}
	switch {
	case in.Mode == ModeVoting:
		if b.Voting != nil {
			if b.Voting.Finalists != nil {
				in.Finalists = *b.Voting.Finalists
				checkFinalists(f, in.Finalists)
			}
			if b.Voting.MultiVote != nil {
				in.MultiVote = *b.Voting.MultiVote
			}
		}
		if len(b.Games) > maxGames {
			f.add("games", "Até "+itoa(maxGames)+" jogos")
			break
		}
		for i, gb := range b.Games {
			in.Games = append(in.Games, checkGame(f, "games["+itoa(i)+"]", gb))
		}
		if repeatedGame(in.Games) {
			f.add("games", "Jogo repetido")
		}
	case in.Mode != "" && (b.Voting != nil || b.Games != nil):
		f.add("voting", "Só a gameplay com votação tem jogos e configuração de votação")
	}
	return in, f
}

func (b patchBody) validate(now time.Time) (patchInput, *fieldErrors) {
	f := &fieldErrors{}
	var in patchInput
	if b.Name != nil {
		name := checkName(f, "name", b.Name, maxName, "Informe o nome")
		in.Name = &name
	}
	switch {
	case b.Description.Null:
		in.ClearDescription = true
	case b.Description.Set:
		if in.Description = checkDescription(f, b.Description.Val); in.Description == nil {
			in.ClearDescription = true
		}
	}
	if b.ScheduledAt != nil {
		t := checkScheduled(f, b.ScheduledAt, now)
		in.ScheduledAt = &t
	}
	if b.Platforms.Set {
		in.Platforms = checkPlatforms(f, b.Platforms.Val)
	}
	if b.Game.Set {
		if b.Game.Null {
			f.add("game", "Informe o jogo")
		} else {
			g := checkGame(f, "game", b.Game.Val)
			in.Game = &g
		}
	}
	if b.Voting != nil {
		if b.Voting.Finalists != nil {
			checkFinalists(f, *b.Voting.Finalists)
			in.Finalists = b.Voting.Finalists
		}
		in.MultiVote = b.Voting.MultiVote
	}
	return in, f
}

func itoa(n int) string { return strconv.Itoa(n) }
