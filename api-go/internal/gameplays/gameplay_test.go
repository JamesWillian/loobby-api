package gameplays

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
)

func TestCriaGameplayDoGrupoEAvisaOsMembros(t *testing.T) {
	f := newFixture(t)
	owner, member, anon := f.user(), f.user(), f.anonymous()
	group := f.group(owner, member, anon)

	in := votingIn("Counter-Strike 2", "Valorant")
	in.Games[0].GameID = strPtr("4291")
	g := f.create(owner, &group, in)

	if g.GroupID == nil || *g.GroupID != group || g.GroupName == nil || g.InviteCode != nil {
		t.Fatalf("gameplay do grupo: groupId %v, groupName %v, inviteCode %v", g.GroupID, g.GroupName, g.InviteCode)
	}
	if g.MyStatus != StatusGoing || g.Creator.ID != owner || g.Status != "ACTIVE" {
		t.Errorf("quem cria entra como Vou: myStatus %s, creator %s, status %s", g.MyStatus, g.Creator.ID, g.Status)
	}
	if !time.Time(g.AutoEndAt).Equal(time.Time(g.ScheduledAt).Add(AutoEndAfter)) {
		t.Errorf("autoEndAt deveria ser scheduledAt + 24 h: %s e %s", g.ScheduledAt.Format(), g.AutoEndAt.Format())
	}
	// todos os membros aparecem; o anonimo nao conta como pendente (nao pode responder)
	if len(g.Participants) != 3 || participantStatus(g, member) != StatusPending || participantStatus(g, anon) != StatusPending {
		t.Errorf("participantes: %+v", g.Participants)
	}
	if g.Counts != (counts{Going: 1, Pending: 1}) {
		t.Errorf("contagem: %+v", g.Counts)
	}
	if g.Voting == nil || len(g.Voting.Games) != 2 || g.Voting.Round != 1 || g.Voting.Locked || len(g.Voting.Result.FinalistIDs) != 0 {
		t.Fatalf("votacao: %+v", g.Voting)
	}
	p := g.Permissions
	if !p.Manage || !p.ConfigureVoting || !p.EditGames || !p.Vote || !p.Comment || !p.Moderate || p.Tiebreak || p.Leave {
		t.Errorf("permissoes de quem criou (e e dono do grupo): %+v", p)
	}
	if got := f.sent.to(TypeNewInGroup); !sameSet(got, []string{member, anon}) {
		t.Errorf("push de gameplay nova: %v", got)
	}
	if !slices.Contains(f.cache.ids, "4291") {
		t.Errorf("o jogo do catalogo deveria ir para o cache: %v", f.cache.ids)
	}

	// o membro ve a mesma gameplay, com as permissoes dele
	seen := f.get(g.ID, member)
	if seen.MyStatus != StatusPending || seen.Permissions.Manage || seen.Permissions.ConfigureVoting || !seen.Permissions.EditGames {
		t.Errorf("visao do membro: %s %+v", seen.MyStatus, seen.Permissions)
	}
	// o anonimo ve tudo e nao pode nada
	asAnon := f.get(g.ID, anon)
	if a := asAnon.Permissions; a.Respond || a.Vote || a.Comment || a.EditGames || a.Manage {
		t.Errorf("anonimo so ve: %+v", a)
	}
}

func TestCriaUnicaComConvite(t *testing.T) {
	f := newFixture(t)
	creator := f.user()
	in := createInput{Name: "Jogatina", Mode: ModeFree, Platforms: []string{"SWITCH"}, ScheduledAt: soon()}
	g := f.create(creator, nil, in)
	if g.InviteCode == nil || !strings.HasPrefix(*g.InviteCode, "$-") || len(*g.InviteCode) != 2+inviteCodeLen {
		t.Fatalf("convite: %v", g.InviteCode)
	}
	if g.GroupID != nil || len(g.Participants) != 1 || g.Voting != nil || g.Game != nil {
		t.Errorf("unica livre: %+v", g)
	}
	if !slices.Equal(g.Platforms, []string{"SWITCH"}) {
		t.Errorf("plataformas: %v", g.Platforms)
	}
	if !g.Permissions.Moderate || g.Permissions.Leave {
		t.Errorf("na unica, quem criou modera e nao sai: %+v", g.Permissions)
	}
	if len(f.sent.pushes) != 0 {
		t.Errorf("a unica nao avisa ninguem: %v", f.sent.pushes)
	}
}

func TestQuemNaoParticipaNaoCriaNemVe(t *testing.T) {
	f := newFixture(t)
	owner, outsider, anon := f.user(), f.user(), f.anonymous()
	group := f.group(owner, anon)

	_, err := f.svc.Create(f.ctx(), anon, &group, votingIn())
	wantErr(t, err, errAnonymous)
	_, err = f.svc.Create(f.ctx(), outsider, &group, votingIn())
	wantErr(t, err, errNotMember)
	missing := ids.New()
	_, err = f.svc.Create(f.ctx(), owner, &missing, votingIn())
	wantErr(t, err, errGroupNotFound)

	g := f.create(owner, &group, votingIn("Valorant"))
	_, err = f.svc.Get(f.ctx(), g.ID, outsider)
	wantErr(t, err, errNotFound)
	_, err = f.svc.Get(f.ctx(), ids.New(), owner)
	wantErr(t, err, errNotFound)
	single := f.create(owner, nil, votingIn("Valorant"))
	_, err = f.svc.Get(f.ctx(), single.ID, outsider)
	wantErr(t, err, errNotFound) // a unica so e vista por quem entrou pelo convite
}

func TestJogoDefinidoComCapaDoCatalogo(t *testing.T) {
	f := newFixture(t)
	creator := f.user()
	catalogID := "teste-" + ids.New()[:8]
	f.exec(`INSERT INTO games (id, name, background_image) VALUES ($1, 'Elden Ring', 'https://capa/elden.jpg')`, catalogID)
	t.Cleanup(func() { f.exec(`DELETE FROM games WHERE id = $1`, catalogID) })

	in := createInput{Name: "Chefe", Mode: ModeFixed, Platforms: []string{"PLAYSTATION"}, ScheduledAt: soon(),
		Game: &gameInput{GameID: &catalogID, Name: "Elden Ring"}}
	g := f.create(creator, nil, in)
	if g.Game == nil || *g.Game.GameID != catalogID || g.Game.CoverURL == nil || *g.Game.CoverURL != "https://capa/elden.jpg" {
		t.Fatalf("jogo definido: %+v", g.Game)
	}

	typed := createInput{Name: "Torneio", Mode: ModeFixed, Platforms: []string{"PC"}, ScheduledAt: soon(),
		Game: &gameInput{Name: "Jogo da firma"}}
	g = f.create(creator, nil, typed)
	if g.Game == nil || g.Game.GameID != nil || g.Game.CoverURL != nil || g.Game.Name != "Jogo da firma" {
		t.Errorf("jogo digitado nao tem capa: %+v", g.Game)
	}
	_, err := f.svc.AddGame(f.ctx(), g.ID, creator, gameInput{Name: "Outro"})
	wantErr(t, err, errNoVoting)
}

func TestEditarGameplay(t *testing.T) {
	f := newFixture(t)
	owner, creator, member := f.user(), f.user(), f.user()
	group := f.group(owner, creator, member)
	g := f.create(creator, &group, votingIn("A", "B"))

	_, err := f.svc.Patch(f.ctx(), g.ID, member, patchInput{Name: strPtr("Outro nome")})
	wantErr(t, err, errNotManager)

	later := soon().Add(24 * time.Hour)
	desc := "Traga fone"
	g2, err := f.svc.Patch(f.ctx(), g.ID, owner, patchInput{
		Name: strPtr("Sabado do CS"), Description: &desc, ScheduledAt: &later, Platforms: []string{"XBOX"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if g2.Name != "Sabado do CS" || *g2.Description != desc || !time.Time(g2.ScheduledAt).Equal(later) ||
		!slices.Equal(g2.Platforms, []string{"XBOX"}) {
		t.Errorf("o dono do grupo edita: %+v", g2)
	}
	g2, _ = f.svc.Patch(f.ctx(), g.ID, creator, patchInput{ClearDescription: true})
	if g2.Description != nil {
		t.Errorf("descricao limpa: %v", g2.Description)
	}

	// finalistas e voto multiplo: so quem criou, e so ate o primeiro voto
	two := 2
	_, err = f.svc.Patch(f.ctx(), g.ID, owner, patchInput{Finalists: &two})
	wantErr(t, err, errNotCreator)
	yes := true
	g2, err = f.svc.Patch(f.ctx(), g.ID, creator, patchInput{Finalists: &two, MultiVote: &yes})
	if err != nil || g2.Voting.Finalists != 2 || !g2.Voting.MultiVote {
		t.Fatalf("configurar a votacao: %+v %v", g2.Voting, err)
	}
	f.vote(g.ID, member, gameID(t, g2, "A"))
	_, err = f.svc.Patch(f.ctx(), g.ID, creator, patchInput{MultiVote: &yes})
	wantErr(t, err, errLocked)
	if g3 := f.get(g.ID, creator); !g3.Voting.Locked || g3.Permissions.ConfigureVoting {
		t.Errorf("depois do voto, travada: %+v %+v", g3.Voting.Locked, g3.Permissions)
	}

	_, err = f.svc.Patch(f.ctx(), g.ID, creator, patchInput{Game: &gameInput{Name: "X"}})
	wantErr(t, err, errNotFixed)
}

func TestEncerrarDeixaSoLeitura(t *testing.T) {
	f := newFixture(t)
	owner, creator, member := f.user(), f.user(), f.user()
	group := f.group(owner, creator, member)
	g := f.create(creator, &group, votingIn("A"))
	c, err := f.svc.AddComment(f.ctx(), g.ID, member, "bora")
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.svc.End(f.ctx(), g.ID, member)
	wantErr(t, err, errNotManager)
	ended, err := f.svc.End(f.ctx(), g.ID, owner) // o dono do grupo tambem encerra
	if err != nil {
		t.Fatal(err)
	}
	if ended.Status != "ENDED" || ended.EndedAt == nil || ended.EndedBy == nil || ended.EndedBy.ID != owner {
		t.Fatalf("encerrada: status %s, endedAt %v, endedBy %+v", ended.Status, ended.EndedAt, ended.EndedBy)
	}
	if p := ended.Permissions; p.Respond || p.Vote || p.EditGames || p.Comment || p.Manage {
		t.Errorf("encerrada e so leitura: %+v", p)
	}
	_, err = f.svc.Respond(f.ctx(), g.ID, member, StatusGoing)
	wantErr(t, err, errEnded)
	_, err = f.svc.Vote(f.ctx(), g.ID, member, nil)
	wantErr(t, err, errEnded)
	_, err = f.svc.AddComment(f.ctx(), g.ID, member, "atrasado")
	wantErr(t, err, errEnded)
	_, err = f.svc.End(f.ctx(), g.ID, creator)
	wantErr(t, err, errEnded)
	// denunciar e apagar comentario continuam valendo
	if err := f.svc.Report(f.ctx(), g.ID, creator, c.ID, nil); err != nil {
		t.Errorf("denunciar depois do encerramento: %v", err)
	}
	if err := f.svc.DeleteComment(f.ctx(), g.ID, member, c.ID); err != nil {
		t.Errorf("apagar depois do encerramento: %v", err)
	}

	// na unica, so quem criou encerra
	single := f.create(creator, nil, votingIn())
	f.exec(`INSERT INTO gameplay_participants (gameplay_id, user_id, status) VALUES ($1, $2, 'GOING')`, single.ID, member)
	_, err = f.svc.End(f.ctx(), single.ID, member)
	wantErr(t, err, errNotManager)
}

// A que passou das 24 h ja esta encerrada para a API, antes de o job carimbar;
// o job carimba o horario em que ela encerrou de fato.
func TestEncerramentoAutomatico(t *testing.T) {
	f := newFixture(t)
	owner, member := f.user(), f.user()
	group := f.group(owner, member)
	yesterday := time.Now().Add(-25 * time.Hour).Round(time.Second)
	old := f.insertGameplay(owner, &group, ModeVoting, yesterday)
	fresh := f.create(owner, &group, votingIn())

	g := f.get(old, member)
	if g.Status != "ENDED" || g.EndedAt == nil || !time.Time(*g.EndedAt).Equal(yesterday.Add(AutoEndAfter)) || g.EndedBy != nil {
		t.Fatalf("passou das 24 h: status %s, endedAt %v, endedBy %v", g.Status, g.EndedAt, g.EndedBy)
	}
	_, err := f.svc.Respond(f.ctx(), old, member, StatusGoing)
	wantErr(t, err, errEnded)

	jobs, err := NewJobs(f.pool, f.sent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jobs.autoEnd(f.ctx()); err != nil {
		t.Fatal(err)
	}
	var endedAt *time.Time
	var endedBy *string
	if err := f.pool.QueryRow(f.ctx(), `SELECT ended_at, ended_by FROM gameplays WHERE id = $1`, old).Scan(&endedAt, &endedBy); err != nil {
		t.Fatal(err)
	}
	if endedAt == nil || !endedAt.Equal(yesterday.Add(AutoEndAfter)) || endedBy != nil {
		t.Errorf("o job carimba scheduled_at + 24 h, sem ended_by: %v %v", endedAt, endedBy)
	}
	if g := f.get(fresh.ID, owner); g.Status != "ACTIVE" {
		t.Errorf("a de daqui a 2 h continua ativa: %s", g.Status)
	}
}

func TestLobbyEListasDoGrupo(t *testing.T) {
	f := newFixture(t)
	me, friend, stranger := f.user(), f.user(), f.user()
	mine := f.group(friend, me)
	other := f.group(stranger)

	later := votingIn("A", "B")
	later.ScheduledAt = soon().Add(time.Hour)
	inGroupLater := f.create(friend, &mine, later)
	inGroup := f.create(friend, &mine, votingIn("A", "B"))
	f.vote(inGroup.ID, friend, gameID(t, inGroup, "B"))
	endedID := f.insertGameplay(friend, &mine, ModeFree, time.Now().Add(-30*time.Hour))
	f.create(stranger, &other, votingIn())

	single := f.create(friend, nil, createInput{Name: "Unica", Mode: ModeFree, Platforms: []string{"PC"}, ScheduledAt: soon().Add(30 * time.Minute)})
	if _, err := f.svc.Join(f.ctx(), *single.InviteCode, me); err != nil {
		t.Fatal(err)
	}
	f.create(friend, nil, votingIn()) // unica em que eu nao entrei

	lobby, err := f.svc.Lobby(f.ctx(), me)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, g := range lobby {
		got = append(got, g.ID)
	}
	want := []string{inGroup.ID, single.ID, inGroupLater.ID} // da mais proxima para a mais distante
	if !slices.Equal(got, want) {
		t.Fatalf("Lobby: %v, queria %v", got, want)
	}
	first := lobby[0]
	if first.GroupName == nil || first.MyStatus != StatusPending || first.GoingCount != 1 || len(first.GoingAvatars) != 1 {
		t.Errorf("item do Lobby: %+v", first)
	}
	if len(first.Leading) != 1 || first.Leading[0].Name != "B" {
		t.Errorf("na frente: %+v", first.Leading)
	}
	if lobby[1].GroupID != nil || lobby[1].MyStatus != StatusPending {
		t.Errorf("unica no Lobby: %+v", lobby[1])
	}

	active, err := f.svc.ListGroup(f.ctx(), me, mine, false, nil, defaultPage)
	if err != nil || len(active) != 2 {
		t.Fatalf("ativas do grupo: %d %v", len(active), err)
	}
	ended, err := f.svc.ListGroup(f.ctx(), me, mine, true, nil, defaultPage)
	if err != nil || len(ended) != 1 || ended[0].ID != endedID || ended[0].Status != "ENDED" {
		t.Fatalf("encerradas do grupo: %+v %v", ended, err)
	}
	_, err = f.svc.ListGroup(f.ctx(), me, other, false, nil, defaultPage)
	wantErr(t, err, errNotMember)
}

func TestEncerradasDoGrupoPaginadas(t *testing.T) {
	f := newFixture(t)
	owner := f.user()
	group := f.group(owner)
	base := time.Now().Add(-72 * time.Hour).Round(time.Second)
	// tres encerradas: duas pelo prazo (em horarios diferentes) e uma pelo botao, a mais recente
	a := f.insertGameplay(owner, &group, ModeFree, base)
	b := f.insertGameplay(owner, &group, ModeFree, base.Add(time.Hour))
	c := f.create(owner, &group, votingIn())
	if _, err := f.svc.End(f.ctx(), c.ID, owner); err != nil {
		t.Fatal(err)
	}

	page, err := f.svc.ListGroup(f.ctx(), owner, group, true, nil, 2)
	if err != nil || len(page) != 2 || page[0].ID != c.ID || page[1].ID != b {
		t.Fatalf("primeira pagina: %+v %v", page, err)
	}
	next, err := f.svc.ListGroup(f.ctx(), owner, group, true, &page[1].ID, 2)
	if err != nil || len(next) != 1 || next[0].ID != a {
		t.Fatalf("segunda pagina: %+v %v", next, err)
	}
	active := f.create(owner, &group, votingIn())
	_, err = f.svc.ListGroup(f.ctx(), owner, group, true, &active.ID, 2)
	wantErr(t, err, errBadCursor) // o cursor precisa ser uma encerrada do grupo
}

func strPtr(s string) *string { return &s }

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, x := range a {
		if !slices.Contains(b, x) {
			return false
		}
	}
	return true
}
