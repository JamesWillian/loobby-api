package gameplays

import (
	"testing"
	"time"
)

func TestComentariosEModeracao(t *testing.T) {
	f := newFixture(t)
	owner, creator, ana, bia, anon := f.user(), f.user(), f.user(), f.user(), f.anonymous()
	group := f.group(owner, creator, ana, bia, anon)
	g := f.create(creator, &group, votingIn())

	var ids []string
	for _, text := range []string{"primeiro", "segundo", "terceiro"} {
		c, err := f.svc.AddComment(f.ctx(), g.ID, ana, text)
		if err != nil || c.Body != text || c.Author.ID != ana || !c.CanDelete {
			t.Fatalf("comentar: %+v %v", c, err)
		}
		ids = append(ids, c.ID)
		time.Sleep(2 * time.Millisecond)
	}
	_, err := f.svc.AddComment(f.ctx(), g.ID, anon, "oi")
	wantErr(t, err, errAnonymous)

	// os mais novos primeiro, em paginas
	page, err := f.svc.Comments(f.ctx(), g.ID, bia, nil, 2)
	if err != nil || len(page) != 2 || page[0].ID != ids[2] || page[1].ID != ids[1] {
		t.Fatalf("primeira pagina: %+v %v", page, err)
	}
	if page[0].CanDelete {
		t.Error("bia nao apaga o comentario da ana")
	}
	next, err := f.svc.Comments(f.ctx(), g.ID, bia, &page[1].ID, 2)
	if err != nil || len(next) != 1 || next[0].ID != ids[0] {
		t.Fatalf("segunda pagina: %+v %v", next, err)
	}
	asAnon, err := f.svc.Comments(f.ctx(), g.ID, anon, nil, 10)
	if err != nil || len(asAnon) != 3 {
		t.Errorf("o anonimo le os comentarios: %d %v", len(asAnon), err)
	}

	// denuncia: nao no proprio; repetir nao faz nada; quem denunciou ve reportedByMe
	err = f.svc.Report(f.ctx(), g.ID, ana, ids[0], nil)
	wantErr(t, err, errOwnComment)
	reason := "ofensivo"
	for range 2 {
		if err := f.svc.Report(f.ctx(), g.ID, bia, ids[0], &reason); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.svc.Report(f.ctx(), g.ID, anon, ids[0], nil); err != nil {
		t.Errorf("o anonimo tambem denuncia: %v", err)
	}
	var reports int
	_ = f.pool.QueryRow(f.ctx(), `SELECT count(*) FROM gameplay_comment_reports WHERE comment_id = $1`, ids[0]).Scan(&reports)
	if reports != 2 {
		t.Errorf("uma denuncia por pessoa: %d", reports)
	}
	mine, _ := f.svc.Comments(f.ctx(), g.ID, bia, nil, 10)
	if !mine[2].ReportedByMe || mine[1].ReportedByMe {
		t.Errorf("reportedByMe: %+v", mine)
	}

	// apagar: o autor e o dono do grupo; quem criou a gameplay do grupo, nao
	err = f.svc.DeleteComment(f.ctx(), g.ID, bia, ids[1])
	wantErr(t, err, errNotYours)
	err = f.svc.DeleteComment(f.ctx(), g.ID, creator, ids[1])
	wantErr(t, err, errNotYours)
	if err := f.svc.DeleteComment(f.ctx(), g.ID, owner, ids[0]); err != nil {
		t.Fatalf("o dono do grupo apaga qualquer um: %v", err)
	}
	_ = f.pool.QueryRow(f.ctx(), `SELECT count(*) FROM gameplay_comment_reports WHERE comment_id = $1`, ids[0]).Scan(&reports)
	if reports != 0 {
		t.Errorf("as denuncias saem com o comentario: %d", reports)
	}
	if err := f.svc.DeleteComment(f.ctx(), g.ID, ana, ids[1]); err != nil {
		t.Fatalf("o autor apaga: %v", err)
	}
	err = f.svc.DeleteComment(f.ctx(), g.ID, ana, ids[1])
	wantErr(t, err, errCommentNotFound)

	// na unica, quem criou modera
	single := f.create(creator, nil, votingIn())
	if _, err := f.svc.Join(f.ctx(), *single.InviteCode, ana); err != nil {
		t.Fatal(err)
	}
	c, err := f.svc.AddComment(f.ctx(), single.ID, ana, "na unica")
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := f.svc.Comments(f.ctx(), single.ID, creator, nil, 10); !list[0].CanDelete {
		t.Error("quem criou a unica pode apagar")
	}
	if err := f.svc.DeleteComment(f.ctx(), single.ID, creator, c.ID); err != nil {
		t.Errorf("quem criou a unica apaga qualquer um: %v", err)
	}
	// comentario de outra gameplay nao e encontrado por esta
	other, _ := f.svc.AddComment(f.ctx(), g.ID, ana, "em outra")
	err = f.svc.DeleteComment(f.ctx(), single.ID, creator, other.ID)
	wantErr(t, err, errCommentNotFound)
}
