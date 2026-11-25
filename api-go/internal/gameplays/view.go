package gameplays

import (
	"context"
	"sort"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

// tally e o resultado de uma rodada da votacao.
//
// Finalistas sao os N mais votados com pelo menos 1 voto; um empate na ultima
// vaga leva todos os empatados. Com 1 finalista (e em todo desempate, que e
// sempre de 1), mais de um finalista e empate no 1o lugar: e o que abre o
// botao Desempatar.
type tally struct {
	round     int
	counts    map[string]int // votos por jogo da votacao, nesta rodada
	finalists []string       // do mais votado para o menos; no empate, na ordem em que entraram
	tie       bool
}

func count(games []sqlc.ListGameplayGamesRow, votes []sqlc.GameplayVote, round, finalists int) tally {
	t := tally{round: round, counts: map[string]int{}, finalists: []string{}}
	for _, v := range votes {
		if int(v.VotingRound) == round {
			t.counts[v.GameplayGameID]++
		}
	}
	n := finalists
	if round > 1 {
		n = 1
	}
	var voted []string
	for _, g := range games { // games vem na ordem de entrada
		if int(g.LastRound) >= round && t.counts[g.ID] > 0 {
			voted = append(voted, g.ID)
		}
	}
	if len(voted) == 0 || n < 1 {
		return t
	}
	sort.SliceStable(voted, func(i, j int) bool { return t.counts[voted[i]] > t.counts[voted[j]] })
	threshold := t.counts[voted[min(n, len(voted))-1]]
	for _, id := range voted {
		if t.counts[id] >= threshold {
			t.finalists = append(t.finalists, id)
		}
	}
	t.tie = n == 1 && len(t.finalists) > 1
	return t
}

func finalistsOf(g sqlc.Gameplay) int {
	if g.Finalists == nil {
		return 0
	}
	return int(*g.Finalists)
}

// people le os usuarios de uma vez, sem repetir ids.
func people(ctx context.Context, q *sqlc.Queries, ids []string) (map[string]sqlc.User, error) {
	seen := map[string]bool{}
	var unique []string
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	out := make(map[string]sqlc.User, len(unique))
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

func refOf(users map[string]sqlc.User, id string) userRef {
	u, ok := users[id]
	if !ok {
		return userRef{ID: id}
	}
	return userRef{ID: u.ID, Username: u.Username, Displayname: u.Displayname, AvatarURL: u.AvatarUrl}
}

func instant(t time.Time) httpx.Instant { return httpx.Instant(t) }

func instantPtr(t *time.Time) *httpx.Instant {
	if t == nil {
		return nil
	}
	i := httpx.Instant(*t)
	return &i
}

func statusOf(g sqlc.Gameplay, now time.Time) string {
	if endedAt(g, now) != nil {
		return "ENDED"
	}
	return "ACTIVE"
}

var statusRank = map[string]int{StatusGoing: 0, StatusPending: 1, StatusNotGoing: 2}

// covers: a capa de cada jogo do catalogo, pelo cache local.
func covers(ctx context.Context, q *sqlc.Queries, gameIDs []string) (map[string]*string, error) {
	out := map[string]*string{}
	if len(gameIDs) == 0 {
		return out, nil
	}
	rows, err := q.ListGameCovers(ctx, gameIDs)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = r.BackgroundImage
	}
	return out, nil
}

func fixedGame(g sqlc.Gameplay, cover map[string]*string) *gameRef {
	if g.Mode != ModeFixed || g.GameName == nil {
		return nil
	}
	ref := &gameRef{GameID: g.GameID, Name: *g.GameName}
	if g.GameID != nil {
		ref.CoverURL = cover[*g.GameID]
	}
	return ref
}

// detail monta a gameplay inteira para quem le (st.me).
func (s *Service) detail(ctx context.Context, q *sqlc.Queries, st state) (gameplayResponse, error) {
	g := st.g
	rows, err := q.ListParticipantsOfGameplays(ctx, []string{g.ID})
	if err != nil {
		return gameplayResponse{}, err
	}
	var members []string
	if st.group != nil {
		if members, err = q.FindMemberUserIDs(ctx, st.group.ID); err != nil {
			return gameplayResponse{}, err
		}
	}
	var (
		games []sqlc.ListGameplayGamesRow
		votes []sqlc.GameplayVote
	)
	if g.Mode == ModeVoting {
		if games, err = q.ListGameplayGames(ctx, []string{g.ID}); err != nil {
			return gameplayResponse{}, err
		}
		if votes, err = q.ListGameplayVotes(ctx, []string{g.ID}); err != nil {
			return gameplayResponse{}, err
		}
	}

	need := []string{g.CreatorID}
	if g.EndedBy != nil {
		need = append(need, *g.EndedBy)
	}
	for _, r := range rows {
		need = append(need, r.UserID)
	}
	need = append(need, members...)
	for _, v := range votes {
		need = append(need, v.UserID)
	}
	users, err := people(ctx, q, need)
	if err != nil {
		return gameplayResponse{}, err
	}

	resp := gameplayResponse{
		ID: g.ID, GroupID: g.GroupID, Name: g.Name, Description: g.Description, Mode: g.Mode,
		Platforms: decodePlatforms(g.Platforms), ScheduledAt: instant(g.ScheduledAt),
		AutoEndAt: instant(autoEndAt(g)), Status: statusOf(g, st.now), EndedAt: instantPtr(endedAt(g, st.now)),
		InviteCode: g.InviteCode, Creator: refOf(users, g.CreatorID), CreatedAt: instant(g.CreatedAt),
		MyStatus: st.status(), Participants: []participantResponse{},
	}
	if st.group != nil {
		resp.GroupName = &st.group.Name
	}
	if g.EndedBy != nil {
		ref := refOf(users, *g.EndedBy)
		resp.EndedBy = &ref
	}

	// Participantes: quem tem linha (na ordem em que respondeu) e, no grupo, os
	// membros que ainda nao responderam. Vou primeiro, depois Pendente, Nao vou.
	listed := map[string]bool{}
	add := func(id, status string) {
		listed[id] = true
		u := users[id]
		anonymous := u.AuthProvider == 0
		resp.Participants = append(resp.Participants, participantResponse{
			User: refOf(users, id), Status: status, IsAnonymous: anonymous,
		})
		switch {
		case status == StatusGoing:
			resp.Counts.Going++
		case status == StatusNotGoing:
			resp.Counts.NotGoing++
		case !anonymous:
			resp.Counts.Pending++
		}
	}
	for _, r := range rows {
		add(r.UserID, r.Status)
	}
	for _, id := range members {
		if !listed[id] {
			add(id, StatusPending)
		}
	}
	sort.SliceStable(resp.Participants, func(i, j int) bool {
		return statusRank[resp.Participants[i].Status] < statusRank[resp.Participants[j].Status]
	})

	locked, tie := false, false
	switch g.Mode {
	case ModeFixed:
		var ids []string
		if g.GameID != nil {
			ids = append(ids, *g.GameID)
		}
		cover, err := covers(ctx, q, ids)
		if err != nil {
			return gameplayResponse{}, err
		}
		resp.Game = fixedGame(g, cover)
	case ModeVoting:
		round := st.round()
		t := count(games, votes, round, finalistsOf(g))
		locked = round > 1 || len(votes) > 0
		tie = t.tie
		v := &votingResponse{
			Finalists: finalistsOf(g), MultiVote: g.MultiVote != nil && *g.MultiVote, Locked: locked,
			Round: round, Games: []votingGame{},
			Result: resultResponse{Round: round, FinalistIDs: t.finalists, Tie: t.tie},
		}
		for _, gm := range games {
			vg := votingGame{
				ID: gm.ID, GameID: gm.GameID, Name: gm.Name, CoverURL: gm.CoverUrl, AddedBy: gm.AddedBy,
				Candidate: int(gm.LastRound) >= round, Rounds: []roundVotes{},
			}
			for r := 1; r <= min(int(gm.LastRound), round); r++ {
				rv := roundVotes{Round: r, Voters: []userRef{}}
				for _, vote := range votes {
					if vote.GameplayGameID != gm.ID || int(vote.VotingRound) != r {
						continue
					}
					rv.Votes++
					rv.Voters = append(rv.Voters, refOf(users, vote.UserID))
					if r == round && vote.UserID == st.me {
						vg.MyVote = true
					}
				}
				vg.Rounds = append(vg.Rounds, rv)
			}
			v.Games = append(v.Games, vg)
		}
		resp.Voting = v
	}
	resp.Permissions = st.permissions(locked, tie)
	return resp, nil
}

// summaries monta os itens de uma lista com poucas consultas, para todas as
// gameplays de uma vez.
func (s *Service) summaries(ctx context.Context, q *sqlc.Queries, list []sqlc.Gameplay, userID string, now time.Time) ([]gameplaySummary, error) {
	out := make([]gameplaySummary, 0, len(list))
	if len(list) == 0 {
		return out, nil
	}
	var all, groupIDs, voting, catalog []string
	for _, g := range list {
		all = append(all, g.ID)
		if g.GroupID != nil {
			groupIDs = append(groupIDs, *g.GroupID)
		}
		switch {
		case g.Mode == ModeVoting:
			voting = append(voting, g.ID)
		case g.Mode == ModeFixed && g.GameID != nil:
			catalog = append(catalog, *g.GameID)
		}
	}

	groupNames := map[string]string{}
	if len(groupIDs) > 0 {
		groups, err := q.FindGroupsByIDs(ctx, groupIDs)
		if err != nil {
			return nil, err
		}
		for _, grp := range groups {
			groupNames[grp.ID] = grp.Name
		}
	}
	rows, err := q.ListParticipantsOfGameplays(ctx, all)
	if err != nil {
		return nil, err
	}
	going := map[string][]string{}
	mine := map[string]string{}
	for _, r := range rows {
		if r.Status == StatusGoing {
			going[r.GameplayID] = append(going[r.GameplayID], r.UserID)
		}
		if r.UserID == userID {
			mine[r.GameplayID] = r.Status
		}
	}
	gamesOf := map[string][]sqlc.ListGameplayGamesRow{}
	votesOf := map[string][]sqlc.GameplayVote{}
	if len(voting) > 0 {
		games, err := q.ListGameplayGames(ctx, voting)
		if err != nil {
			return nil, err
		}
		for _, gm := range games {
			gamesOf[gm.GameplayID] = append(gamesOf[gm.GameplayID], gm)
		}
		votes, err := q.ListGameplayVotes(ctx, voting)
		if err != nil {
			return nil, err
		}
		for _, v := range votes {
			votesOf[v.GameplayID] = append(votesOf[v.GameplayID], v)
		}
	}
	cover, err := covers(ctx, q, catalog)
	if err != nil {
		return nil, err
	}
	var need []string
	for _, g := range list {
		need = append(need, g.CreatorID)
		ids := going[g.ID]
		need = append(need, ids[:min(len(ids), goingAvatarCount)]...)
	}
	users, err := people(ctx, q, need)
	if err != nil {
		return nil, err
	}

	for _, g := range list {
		item := gameplaySummary{
			ID: g.ID, GroupID: g.GroupID, Name: g.Name, Mode: g.Mode, Platforms: decodePlatforms(g.Platforms),
			ScheduledAt: instant(g.ScheduledAt), AutoEndAt: instant(autoEndAt(g)), Status: statusOf(g, now),
			EndedAt: instantPtr(endedAt(g, now)), Creator: refOf(users, g.CreatorID), MyStatus: StatusPending,
			GoingCount: len(going[g.ID]), GoingAvatars: []*string{}, Game: fixedGame(g, cover), Leading: []gameRef{},
		}
		if g.GroupID != nil {
			name := groupNames[*g.GroupID]
			item.GroupName = &name
		}
		if st, ok := mine[g.ID]; ok {
			item.MyStatus = st
		}
		ids := going[g.ID]
		for _, id := range ids[:min(len(ids), goingAvatarCount)] {
			item.GoingAvatars = append(item.GoingAvatars, users[id].AvatarUrl)
		}
		if g.Mode == ModeVoting && g.VotingRound != nil {
			games := gamesOf[g.ID]
			t := count(games, votesOf[g.ID], int(*g.VotingRound), finalistsOf(g))
			byID := map[string]sqlc.ListGameplayGamesRow{}
			for _, gm := range games {
				byID[gm.ID] = gm
			}
			for _, id := range t.finalists {
				gm := byID[id]
				item.Leading = append(item.Leading, gameRef{GameID: gm.GameID, Name: gm.Name, CoverURL: gm.CoverUrl})
			}
		}
		out = append(out, item)
	}
	return out, nil
}
