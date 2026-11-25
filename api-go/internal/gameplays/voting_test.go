package gameplays

import (
	"slices"
	"testing"

	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
)

// O resultado, sem banco: finalistas, empate na ultima vaga e desempate.
func TestApuracao(t *testing.T) {
	games := func(lastRound ...int16) []sqlc.ListGameplayGamesRow {
		var out []sqlc.ListGameplayGamesRow
		for i, r := range lastRound {
			out = append(out, sqlc.ListGameplayGamesRow{ID: string(rune('A' + i)), LastRound: r})
		}
		return out
	}
	votes := func(round int16, perGame map[string]int) []sqlc.GameplayVote {
		var out []sqlc.GameplayVote
		for id, n := range perGame {
			for i := 0; i < n; i++ {
				out = append(out, sqlc.GameplayVote{GameplayGameID: id, VotingRound: round, UserID: id + string(rune('0'+i))})
			}
		}
		return out
	}
	cases := []struct {
		name      string
		games     []sqlc.ListGameplayGamesRow
		votes     []sqlc.GameplayVote
		round     int
		finalists int
		want      []string
		tie       bool
	}{
		{"sem voto, sem finalista", games(1, 1), nil, 1, 1, []string{}, false},
		{"1 finalista, vencedor claro", games(1, 1, 1), votes(1, map[string]int{"A": 1, "B": 3}), 1, 1, []string{"B"}, false},
		{"1 finalista, empate no 1o lugar", games(1, 1, 1), votes(1, map[string]int{"A": 2, "B": 2, "C": 1}), 1, 1, []string{"A", "B"}, true},
		{"2 finalistas, empate na ultima vaga leva todos", games(1, 1, 1, 1),
			votes(1, map[string]int{"A": 3, "B": 2, "C": 2, "D": 0}), 1, 2, []string{"A", "B", "C"}, false},
		{"3 finalistas, so 2 com voto", games(1, 1, 1), votes(1, map[string]int{"A": 1, "C": 2}), 1, 3, []string{"C", "A"}, false},
		{"2 finalistas, todos empatados nao e desempate", games(1, 1, 1), votes(1, map[string]int{"A": 1, "B": 1, "C": 1}), 1, 2,
			[]string{"A", "B", "C"}, false},
		{"desempate e sempre de 1, so entre os candidatos", games(2, 2, 1),
			append(votes(1, map[string]int{"A": 2, "B": 2}), votes(2, map[string]int{"A": 1, "B": 2, "C": 5})...), 2, 3,
			[]string{"B"}, false},
		{"desempate empatado de novo", games(2, 2, 1), votes(2, map[string]int{"A": 1, "B": 1}), 2, 1, []string{"A", "B"}, true},
	}
	for _, c := range cases {
		got := count(c.games, c.votes, c.round, c.finalists)
		if !slices.Equal(got.finalists, c.want) || got.tie != c.tie {
			t.Errorf("%s: finalistas %v (empate %v), queria %v (empate %v)", c.name, got.finalists, got.tie, c.want, c.tie)
		}
	}
}

func TestVotoUnicoEMultiplo(t *testing.T) {
	f := newFixture(t)
	owner, member := f.user(), f.user()
	group := f.group(owner, member)
	g := f.create(owner, &group, votingIn("A", "B", "C"))
	a, b, c := gameID(t, g, "A"), gameID(t, g, "B"), gameID(t, g, "C")

	_, err := f.svc.Vote(f.ctx(), g.ID, member, []string{a, b})
	wantErr(t, err, errSingleVote)
	g = f.vote(g.ID, member, a)
	if g.MyStatus != StatusGoing || participantStatus(g, member) != StatusGoing {
		t.Errorf("votar estando Pendente passa para Vou: %s", g.MyStatus)
	}
	g = f.vote(g.ID, member, b) // trocar o voto
	if votesOf(g, a) != 0 || votesOf(g, b) != 1 || !myVote(g, b) {
		t.Errorf("o voto troca: A %d, B %d", votesOf(g, a), votesOf(g, b))
	}
	if voters := votersOf(g, b, 1); len(voters) != 1 || voters[0].ID != member {
		t.Errorf("o voto e visivel: %+v", voters)
	}
	var names []string
	for _, gm := range g.Voting.Games {
		names = append(names, gm.Name)
	}
	if !slices.Equal(names, []string{"A", "B", "C"}) {
		t.Errorf("os jogos saem na ordem em que foram escolhidos: %v", names)
	}
	_, err = f.svc.Vote(f.ctx(), g.ID, member, []string{"00000000-0000-4000-8000-000000000000"})
	wantErr(t, err, errNotOnBallot)

	yes := true
	other := f.create(owner, &group, votingIn("A", "B", "C"))
	if _, err := f.svc.Patch(f.ctx(), other.ID, owner, patchInput{MultiVote: &yes}); err != nil {
		t.Fatal(err)
	}
	a, b, c = gameID(t, other, "A"), gameID(t, other, "B"), gameID(t, other, "C")
	g = f.vote(other.ID, member, a, c)
	if votesOf(g, a) != 1 || votesOf(g, c) != 1 || votesOf(g, b) != 0 {
		t.Errorf("voto multiplo: %+v", g.Voting.Games)
	}
	_, err = f.svc.Vote(f.ctx(), other.ID, member, []string{a, a})
	wantErr(t, err, errVoteTwice)
	g = f.vote(other.ID, member) // lista vazia tira o voto
	if votesOf(g, a) != 0 || votesOf(g, c) != 0 || g.MyStatus != StatusGoing {
		t.Errorf("tirar o voto nao muda o Vou: %+v %s", g.Voting.Games, g.MyStatus)
	}
}

func TestNaoVouEPendenteApagamOsVotos(t *testing.T) {
	f := newFixture(t)
	owner, member, anon := f.user(), f.user(), f.anonymous()
	group := f.group(owner, member, anon)
	g := f.create(owner, &group, votingIn("A", "B"))
	a := gameID(t, g, "A")

	f.vote(g.ID, member, a)
	g = f.respond(g.ID, member, StatusNotGoing)
	if votesOf(g, a) != 0 || g.Permissions.Vote {
		t.Errorf("Nao vou apaga o voto e nao vota: votos %d, vote %v", votesOf(g, a), g.Permissions.Vote)
	}
	_, err := f.svc.Vote(f.ctx(), g.ID, member, []string{a})
	wantErr(t, err, errNotGoingVote)

	f.respond(g.ID, member, StatusGoing)
	f.vote(g.ID, member, a)
	g = f.respond(g.ID, member, StatusPending)
	if votesOf(g, a) != 0 || participantStatus(g, member) != StatusPending {
		t.Errorf("Pendente apaga o voto: %d %s", votesOf(g, a), participantStatus(g, member))
	}

	_, err = f.svc.Respond(f.ctx(), g.ID, anon, StatusGoing)
	wantErr(t, err, errAnonymous)
	_, err = f.svc.Vote(f.ctx(), g.ID, anon, []string{a})
	wantErr(t, err, errAnonymous)
}

func TestListaDeJogosDaVotacao(t *testing.T) {
	f := newFixture(t)
	owner, member, anon, guest := f.user(), f.user(), f.anonymous(), f.user()
	group := f.group(owner, member, anon)
	g := f.create(owner, &group, votingIn("Valorant"))

	catalog := "3498"
	g2, err := f.svc.AddGame(f.ctx(), g.ID, member, gameInput{GameID: &catalog, Name: "GTA V"}) // qualquer membro adiciona
	if err != nil || len(g2.Voting.Games) != 2 || *g2.Voting.Games[1].AddedBy != member {
		t.Fatalf("membro adiciona: %+v %v", g2.Voting, err)
	}
	_, err = f.svc.AddGame(f.ctx(), g.ID, member, gameInput{Name: "valorant"})
	wantErr(t, err, errRepeated) // mesmo nome, outra caixa
	_, err = f.svc.AddGame(f.ctx(), g.ID, owner, gameInput{GameID: &catalog, Name: "Grand Theft Auto V"})
	wantErr(t, err, errRepeated) // mesmo jogo do catalogo
	_, err = f.svc.AddGame(f.ctx(), g.ID, anon, gameInput{Name: "Lol"})
	wantErr(t, err, errAnonymous)

	// remover leva os votos junto
	gta := gameID(t, g2, "GTA V")
	f.vote(g.ID, owner, gta)
	g2, err = f.svc.RemoveGame(f.ctx(), g.ID, member, gta)
	if err != nil || len(g2.Voting.Games) != 1 || g2.Voting.Locked {
		t.Fatalf("remover: %+v %v", g2.Voting, err)
	}
	_, err = f.svc.RemoveGame(f.ctx(), g.ID, member, gta)
	wantErr(t, err, errGameNotFound)

	// ate 20
	for i := len(g2.Voting.Games); i < maxGames; i++ {
		if _, err := f.svc.AddGame(f.ctx(), g.ID, owner, gameInput{Name: "Jogo " + itoa(i)}); err != nil {
			t.Fatal(err)
		}
	}
	_, err = f.svc.AddGame(f.ctx(), g.ID, owner, gameInput{Name: "O vigesimo primeiro"})
	wantErr(t, err, errFull)

	// na unica, so quem criou mexe na lista
	single := f.create(owner, nil, votingIn("A"))
	if _, err := f.svc.Join(f.ctx(), *single.InviteCode, guest); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.AddGame(f.ctx(), single.ID, guest, gameInput{Name: "B"})
	wantErr(t, err, errSingleGames)
	if g := f.get(single.ID, guest); g.Permissions.EditGames {
		t.Errorf("convidado da unica nao mexe nos jogos: %+v", g.Permissions)
	}
	if _, err := f.svc.AddGame(f.ctx(), single.ID, owner, gameInput{Name: "B"}); err != nil {
		t.Errorf("quem criou a unica adiciona: %v", err)
	}
}

func TestDesempate(t *testing.T) {
	f := newFixture(t)
	owner, creator, ana, bia, caio := f.user(), f.user(), f.user(), f.user(), f.user()
	group := f.group(owner, creator, ana, bia, caio)
	g := f.create(creator, &group, votingIn("A", "B", "C"))
	a, b, c := gameID(t, g, "A"), gameID(t, g, "B"), gameID(t, g, "C")

	f.vote(g.ID, ana, a)
	g = f.vote(g.ID, bia, b)
	if !g.Voting.Result.Tie || !sameSet(g.Voting.Result.FinalistIDs, []string{a, b}) {
		t.Fatalf("empate: %+v", g.Voting.Result)
	}
	if g.Permissions.Tiebreak {
		t.Error("so quem criou ve o Desempatar")
	}
	if !f.get(g.ID, creator).Permissions.Tiebreak {
		t.Error("quem criou deveria poder desempatar")
	}
	_, err := f.svc.Tiebreak(f.ctx(), g.ID, owner)
	wantErr(t, err, errNotCreator) // nem o dono do grupo

	g, err = f.svc.Tiebreak(f.ctx(), g.ID, creator)
	if err != nil {
		t.Fatal(err)
	}
	if g.Voting.Round != 2 || !candidate(g, a) || !candidate(g, b) || candidate(g, c) || !g.Voting.Locked {
		t.Fatalf("rodada 2 so com os empatados: %+v", g.Voting)
	}
	if len(g.Voting.Result.FinalistIDs) != 0 || g.Permissions.EditGames || g.Permissions.Tiebreak {
		t.Errorf("rodada nova comeca sem voto e com a lista congelada: %+v %+v", g.Voting.Result, g.Permissions)
	}
	if got := f.sent.to(TypeTiebreakOpened); !sameSet(got, []string{ana, bia}) {
		t.Errorf("push do desempate para quem marcou Vou, menos quem abriu: %v", got)
	}

	_, err = f.svc.AddGame(f.ctx(), g.ID, ana, gameInput{Name: "D"})
	wantErr(t, err, errFrozen)
	_, err = f.svc.RemoveGame(f.ctx(), g.ID, ana, c)
	wantErr(t, err, errFrozen)
	_, err = f.svc.Vote(f.ctx(), g.ID, caio, []string{c})
	wantErr(t, err, errNotOnBallot)

	g = f.vote(g.ID, caio, a) // Caio nao votou na principal, mas vota no desempate
	if roundVotesOf(g, a, 1) != 1 || roundVotesOf(g, a, 2) != 1 {
		t.Errorf("a rodada 1 fica congelada e a 2 conta a parte: %+v", g.Voting.Games)
	}
	// sair (Pendente ou Nao vou) apaga so o voto da rodada aberta
	f.vote(g.ID, ana, b)
	g = f.respond(g.ID, ana, StatusNotGoing)
	if roundVotesOf(g, a, 1) != 1 || roundVotesOf(g, b, 2) != 0 {
		t.Errorf("o voto da rodada congelada fica: rodada 1 de A %d, rodada 2 de B %d", roundVotesOf(g, a, 1), roundVotesOf(g, b, 2))
	}

	// empatou de novo: outro desempate
	g = f.vote(g.ID, bia, b)
	if !g.Voting.Result.Tie {
		t.Fatalf("A 1 x B 1 no desempate: %+v", g.Voting.Result)
	}
	g, err = f.svc.Tiebreak(f.ctx(), g.ID, creator)
	if err != nil || g.Voting.Round != 3 {
		t.Fatalf("terceira rodada: %v %v", g.Voting.Round, err)
	}
	g = f.vote(g.ID, caio, b)
	if g.Voting.Result.Tie || !slices.Equal(g.Voting.Result.FinalistIDs, []string{b}) {
		t.Errorf("desempatou: %+v", g.Voting.Result)
	}
	_, err = f.svc.Tiebreak(f.ctx(), g.ID, creator)
	wantErr(t, err, errNoTie)
}

func TestDesempateNaoValeComMaisFinalistas(t *testing.T) {
	f := newFixture(t)
	owner, ana, bia := f.user(), f.user(), f.user()
	group := f.group(owner, ana, bia)
	in := votingIn("A", "B", "C")
	in.Finalists = 2
	g := f.create(owner, &group, in)
	f.vote(g.ID, ana, gameID(t, g, "A"))
	g = f.vote(g.ID, bia, gameID(t, g, "B"))
	if g.Voting.Result.Tie || len(g.Voting.Result.FinalistIDs) != 2 || g.Permissions.Tiebreak {
		t.Fatalf("2 finalistas: os dois vao, sem desempate: %+v %+v", g.Voting.Result, g.Permissions)
	}
	_, err := f.svc.Tiebreak(f.ctx(), g.ID, owner)
	wantErr(t, err, errNoTie)
}

func votesOf(g gameplayResponse, id string) int { return roundVotesOf(g, id, g.Voting.Round) }

func roundVotesOf(g gameplayResponse, id string, round int) int {
	for _, gm := range g.Voting.Games {
		if gm.ID != id {
			continue
		}
		for _, r := range gm.Rounds {
			if r.Round == round {
				return r.Votes
			}
		}
	}
	return 0
}

func votersOf(g gameplayResponse, id string, round int) []userRef {
	for _, gm := range g.Voting.Games {
		if gm.ID != id {
			continue
		}
		for _, r := range gm.Rounds {
			if r.Round == round {
				return r.Voters
			}
		}
	}
	return nil
}

func myVote(g gameplayResponse, id string) bool {
	for _, gm := range g.Voting.Games {
		if gm.ID == id {
			return gm.MyVote
		}
	}
	return false
}

func candidate(g gameplayResponse, id string) bool {
	for _, gm := range g.Voting.Games {
		if gm.ID == id {
			return gm.Candidate
		}
	}
	return false
}
