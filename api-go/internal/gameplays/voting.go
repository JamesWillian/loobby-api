package gameplays

import (
	"context"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
)

// checkGames: o que adicionar e remover jogos tem em comum. A lista so muda na
// rodada principal: o desempate a congela.
func checkGames(st state) error {
	if err := st.requireAct(); err != nil {
		return err
	}
	if st.g.Mode != ModeVoting {
		return errNoVoting
	}
	if st.round() > 1 {
		return errFrozen
	}
	if st.single() && !st.creator() {
		return errSingleGames
	}
	return nil
}

// AddGame poe um jogo na votacao: ate 20, sem repetir o do catalogo nem o nome.
func (s *Service) AddGame(ctx context.Context, id, userID string, in gameInput) (gameplayResponse, error) {
	resp, err := s.mutate(ctx, id, userID, func(q *sqlc.Queries, st state) error {
		if err := checkGames(st); err != nil {
			return err
		}
		n, err := q.CountGameplayGames(ctx, id)
		if err != nil {
			return err
		}
		if n >= maxGames {
			return errFull
		}
		taken, err := q.GameplayGameTaken(ctx, sqlc.GameplayGameTakenParams{GameplayID: id, GameID: in.GameID, Name: in.Name})
		if err != nil {
			return err
		}
		if taken {
			return errRepeated
		}
		err = q.AddGameplayGame(ctx, sqlc.AddGameplayGameParams{
			ID: ids.New(), GameplayID: id, GameID: in.GameID, Name: in.Name, AddedBy: &userID,
		})
		if isUniqueViolation(err) {
			return errRepeated
		}
		return err
	})
	if err != nil {
		return gameplayResponse{}, err
	}
	s.cache(&in)
	return resp, nil
}

// RemoveGame tira um jogo da votacao, e os votos nele vao junto.
func (s *Service) RemoveGame(ctx context.Context, id, userID, gameID string) (gameplayResponse, error) {
	return s.mutate(ctx, id, userID, func(q *sqlc.Queries, st state) error {
		if err := checkGames(st); err != nil {
			return err
		}
		_, err := q.GetGameplayGame(ctx, sqlc.GetGameplayGameParams{ID: gameID, GameplayID: id})
		if db.NotFound(err) {
			return errGameNotFound
		}
		if err != nil {
			return err
		}
		return q.DeleteGameplayGame(ctx, gameID)
	})
}

// Vote troca os votos de quem le na rodada aberta pelos de choice (vazio tira
// o voto). Voto unico, ou um por jogo no multiplo; desempate e sempre 1. Votar
// estando Pendente passa para Vou; com Nao vou, e recusado.
func (s *Service) Vote(ctx context.Context, id, userID string, choice []string) (gameplayResponse, error) {
	return s.mutate(ctx, id, userID, func(q *sqlc.Queries, st state) error {
		if err := st.requireAct(); err != nil {
			return err
		}
		if st.g.Mode != ModeVoting {
			return errNoVoting
		}
		if st.status() == StatusNotGoing {
			return errNotGoingVote
		}
		round := st.round()
		single := round > 1 || st.g.MultiVote == nil || !*st.g.MultiVote
		if single && len(choice) > 1 {
			return errSingleVote
		}
		games, err := q.ListGameplayGames(ctx, []string{id})
		if err != nil {
			return err
		}
		ballot := map[string]bool{}
		for _, g := range games {
			if int(g.LastRound) >= round {
				ballot[g.ID] = true
			}
		}
		seen := map[string]bool{}
		for _, gameID := range choice {
			if seen[gameID] {
				return errVoteTwice
			}
			seen[gameID] = true
			if !ballot[gameID] {
				return errNotOnBallot
			}
		}

		if err := clearMyVotes(ctx, q, st); err != nil {
			return err
		}
		for _, gameID := range choice {
			err := q.InsertGameplayVote(ctx, sqlc.InsertGameplayVoteParams{
				GameplayGameID: gameID, VotingRound: int16(round), UserID: userID, GameplayID: id,
			})
			if err != nil {
				return err
			}
		}
		if len(choice) > 0 && st.status() != StatusGoing {
			return q.UpsertParticipant(ctx, sqlc.UpsertParticipantParams{GameplayID: id, UserID: userID, Status: StatusGoing})
		}
		return nil
	})
}

// Tiebreak e o Desempatar, de quem criou: com 1 finalista e empate no 1o
// lugar, abre a rodada seguinte so com os empatados. A rodada anterior congela,
// votos e lista de jogos. Quem marcou Vou recebe o push.
func (s *Service) Tiebreak(ctx context.Context, id, userID string) (gameplayResponse, error) {
	var g sqlc.Gameplay
	resp, err := s.mutate(ctx, id, userID, func(q *sqlc.Queries, st state) error {
		if !st.active() {
			return errEnded
		}
		if st.g.Mode != ModeVoting {
			return errNoVoting
		}
		if !st.creator() {
			return errNotCreator
		}
		games, err := q.ListGameplayGames(ctx, []string{id})
		if err != nil {
			return err
		}
		votes, err := q.ListGameplayVotes(ctx, []string{id})
		if err != nil {
			return err
		}
		round := st.round()
		t := count(games, votes, round, finalistsOf(st.g))
		if !t.tie {
			return errNoTie
		}
		next := int16(round + 1)
		if err := q.PromoteGameplayGames(ctx, sqlc.PromoteGameplayGamesParams{LastRound: next, Ids: t.finalists}); err != nil {
			return err
		}
		g = st.g
		return q.SetGameplayVotingRound(ctx, sqlc.SetGameplayVotingRoundParams{ID: id, VotingRound: &next})
	})
	if err != nil {
		return gameplayResponse{}, err
	}
	s.notifyTiebreak(g)
	return resp, nil
}
