package gameplays

import (
	"context"
	"log/slog"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
)

// Comments: os comentarios, dos mais novos para os mais antigos, em paginas
// (before = o ultimo comentario da pagina anterior). Qualquer um com acesso le,
// anonimo inclusive.
func (s *Service) Comments(ctx context.Context, id, userID string, before *string, limit int) ([]commentResponse, error) {
	st, err := s.load(ctx, s.q, id, userID, false)
	if err != nil {
		return nil, err
	}
	var rows []sqlc.ListGameplayCommentsRow
	if before == nil {
		rows, err = s.q.ListGameplayComments(ctx, sqlc.ListGameplayCommentsParams{
			ViewerID: userID, GameplayID: id, MaxRows: int32(limit),
		})
	} else {
		cursor, err := s.q.GetGameplayComment(ctx, sqlc.GetGameplayCommentParams{ID: *before, GameplayID: id})
		if db.NotFound(err) {
			return nil, errBadComment
		}
		if err != nil {
			return nil, err
		}
		older, err := s.q.ListGameplayCommentsBefore(ctx, sqlc.ListGameplayCommentsBeforeParams{
			ViewerID: userID, GameplayID: id, BeforeAt: cursor.CreatedAt, BeforeID: cursor.ID, MaxRows: int32(limit),
		})
		if err != nil {
			return nil, err
		}
		for _, r := range older {
			rows = append(rows, sqlc.ListGameplayCommentsRow(r))
		}
	}
	if err != nil {
		return nil, err
	}
	out := make([]commentResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, commentResponse{
			ID: r.ID, GameplayID: r.GameplayID, Body: r.Body, CreatedAt: instant(r.CreatedAt),
			Author:       userRef{ID: r.UserID, Username: r.Username, Displayname: r.Displayname, AvatarURL: r.AvatarUrl},
			CanDelete:    r.UserID == userID || st.moderator(),
			ReportedByMe: r.ReportedByMe,
		})
	}
	return out, nil
}

// AddComment: quem participa e nao e anonimo comenta, enquanto a gameplay
// estiver ativa.
func (s *Service) AddComment(ctx context.Context, id, userID, body string) (commentResponse, error) {
	st, err := s.load(ctx, s.q, id, userID, false)
	if err != nil {
		return commentResponse{}, err
	}
	if err := st.requireAct(); err != nil {
		return commentResponse{}, err
	}
	commentID := ids.New()
	at, err := s.q.InsertGameplayComment(ctx, sqlc.InsertGameplayCommentParams{
		ID: commentID, GameplayID: id, UserID: userID, Body: body,
	})
	if err != nil {
		return commentResponse{}, err
	}
	users, err := people(ctx, s.q, []string{userID})
	if err != nil {
		return commentResponse{}, err
	}
	return commentResponse{
		ID: commentID, GameplayID: id, Author: refOf(users, userID), Body: body,
		CreatedAt: instant(at), CanDelete: true,
	}, nil
}

// DeleteComment: cada um apaga os seus; o dono do grupo (na unica, quem criou)
// apaga qualquer um. Vale tambem depois do encerramento.
func (s *Service) DeleteComment(ctx context.Context, id, userID, commentID string) error {
	st, err := s.load(ctx, s.q, id, userID, false)
	if err != nil {
		return err
	}
	c, err := s.q.GetGameplayComment(ctx, sqlc.GetGameplayCommentParams{ID: commentID, GameplayID: id})
	if db.NotFound(err) {
		return errCommentNotFound
	}
	if err != nil {
		return err
	}
	if c.UserID != userID && !st.moderator() {
		return errNotYours
	}
	return s.q.DeleteGameplayComment(ctx, commentID)
}

// Report: a denuncia feita no app. Qualquer um com acesso denuncia, anonimo
// inclusive, e tambem depois do encerramento; repetir nao faz nada. A denuncia
// fica em gameplay_comment_reports para quem administra o Loobby.
func (s *Service) Report(ctx context.Context, id, userID, commentID string, reason *string) error {
	if _, err := s.load(ctx, s.q, id, userID, false); err != nil {
		return err
	}
	c, err := s.q.GetGameplayComment(ctx, sqlc.GetGameplayCommentParams{ID: commentID, GameplayID: id})
	if db.NotFound(err) {
		return errCommentNotFound
	}
	if err != nil {
		return err
	}
	if c.UserID == userID {
		return errOwnComment
	}
	n, err := s.q.InsertCommentReport(ctx, sqlc.InsertCommentReportParams{
		ID: ids.New(), CommentID: commentID, ReporterID: userID, Reason: reason,
	})
	if err != nil {
		return err
	}
	if n > 0 {
		slog.Warn("comentario denunciado", "comentario", commentID, "gameplay", id)
	}
	return nil
}
