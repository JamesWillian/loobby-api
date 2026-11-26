package gameplays

import (
	"slices"
	"testing"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
)

// going grava a participacao direto no banco, como a gameplay de ontem pede.
func (f *fixture) going(gameplayID, userID, status string) {
	f.exec(`INSERT INTO gameplay_participants (gameplay_id, user_id, status) VALUES ($1, $2, $3)
	        ON CONFLICT (gameplay_id, user_id) DO UPDATE SET status = EXCLUDED.status`, gameplayID, userID, status)
}

func summaryIDs(list []gameplaySummary) []string {
	var out []string
	for _, s := range list {
		out = append(out, s.ID)
	}
	return out
}

func TestHistoricoSoComAsEncerradasEmQueFui(t *testing.T) {
	f := newFixture(t)
	me, friend := f.user(), f.user()
	group := f.group(friend, me)
	left := f.group(friend, me)
	base := time.Now().Add(-96 * time.Hour).Round(time.Second)

	went := f.insertGameplay(friend, &group, ModeFree, base)                // encerrou pelo prazo, e eu fui
	notGoing := f.insertGameplay(friend, &group, ModeFree, base)            // eu marquei Nao vou
	single := f.insertGameplay(friend, nil, ModeFixed, base.Add(time.Hour)) // a unica em que eu fui
	gone := f.insertGameplay(friend, &left, ModeFree, base.Add(2*time.Hour))
	f.going(went, me, StatusGoing)
	f.going(notGoing, me, StatusNotGoing)
	f.going(single, me, StatusGoing)
	f.going(gone, me, StatusGoing)

	active := f.create(friend, &group, votingIn("A"))
	f.respond(active.ID, me, StatusGoing) // ativa: ainda nao e historico
	ended := f.create(friend, &group, votingIn("A"))
	f.respond(ended.ID, me, StatusGoing)
	if _, err := f.svc.End(f.ctx(), ended.ID, friend); err != nil {
		t.Fatal(err)
	}

	list, err := f.svc.History(f.ctx(), me, nil, defaultPage)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{ended.ID, gone, single, went}; !slices.Equal(summaryIDs(list), want) {
		t.Fatalf("historico: queria %v, veio %v", want, summaryIDs(list))
	}
	if list[0].Status != "ENDED" || list[0].MyStatus != StatusGoing || list[0].GroupName == nil {
		t.Errorf("item do historico: %+v", list[0])
	}

	// quem sai do grupo deixa de ver as gameplays dele; voltando, elas voltam
	f.exec(`DELETE FROM group_members WHERE group_id = $1 AND user_id = $2`, left, me)
	if list, _ = f.svc.History(f.ctx(), me, nil, defaultPage); slices.Contains(summaryIDs(list), gone) {
		t.Errorf("fora do grupo, a gameplay dele sai do historico: %v", summaryIDs(list))
	}
	f.join(left, me)
	if list, _ = f.svc.History(f.ctx(), me, nil, defaultPage); !slices.Contains(summaryIDs(list), gone) {
		t.Errorf("de volta ao grupo, a gameplay volta: %v", summaryIDs(list))
	}

	// o anonimo nao responde, entao nao tem historico
	if list, err = f.svc.History(f.ctx(), f.anonymous(), nil, defaultPage); err != nil || len(list) != 0 {
		t.Errorf("historico do anonimo: %v (%v)", summaryIDs(list), err)
	}
}

func TestHistoricoPaginado(t *testing.T) {
	f := newFixture(t)
	me, friend := f.user(), f.user()
	group := f.group(friend, me)
	base := time.Now().Add(-96 * time.Hour).Round(time.Second)
	var all []string
	for i := range 3 {
		id := f.insertGameplay(friend, &group, ModeFree, base.Add(time.Duration(i)*time.Hour))
		f.going(id, me, StatusGoing)
		all = append([]string{id}, all...) // a mais nova primeiro
	}
	skipped := f.insertGameplay(friend, &group, ModeFree, base)
	f.going(skipped, me, StatusNotGoing)

	page, err := f.svc.History(f.ctx(), me, nil, 2)
	if err != nil || !slices.Equal(summaryIDs(page), all[:2]) {
		t.Fatalf("primeira pagina: %v (%v)", summaryIDs(page), err)
	}
	next, err := f.svc.History(f.ctx(), me, &page[1].ID, 2)
	if err != nil || !slices.Equal(summaryIDs(next), all[2:]) {
		t.Fatalf("segunda pagina: %v (%v)", summaryIDs(next), err)
	}

	// o cursor e uma encerrada em que eu fui: nem ativa, nem Nao vou, nem alheia
	active := f.create(friend, &group, votingIn("A"))
	f.respond(active.ID, me, StatusGoing)
	other := f.insertGameplay(friend, &group, ModeFree, base)
	for _, cursor := range []string{active.ID, skipped, other, ids.New()} {
		_, err = f.svc.History(f.ctx(), me, &cursor, 2)
		wantErr(t, err, errBadCursor)
	}
}
