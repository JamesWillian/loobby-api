package games

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
)

// cached e uma linha da tabela games, como o cache a serve.
type cached struct {
	ID              string
	Slug            *string
	Name            string
	BackgroundImage *string
	Released        *time.Time
	Rating          *string // texto do Postgres, com a escala da coluna: "0.00"
	Metacritic      *int32
	DescriptionRaw  *string
	Genres          []byte // texto do jsonb, na ordem de chaves do Postgres
	Platforms       []byte
	RefreshedAt     time.Time
}

// Store e o que o servico precisa do banco. A implementacao real usa as
// queries geradas pelo sqlc (db/queries/games.sql).
type Store interface {
	Find(ctx context.Context, id string) (cached, bool, error)
	// Os upserts devolvem o nome que ficou gravado: quando o RAWG nao manda
	// nome, o registro mantem o que ja tinha -- e a resposta mostra esse.
	UpsertFromSearch(ctx context.Context, g game, now time.Time) (string, error)
	UpsertFromDetail(ctx context.Context, g game, description *string, now time.Time) (string, error)
}

type pgStore struct{ q *sqlc.Queries }

func NewStore(db sqlc.DBTX) Store { return pgStore{sqlc.New(db)} }

func (s pgStore) Find(ctx context.Context, id string) (cached, bool, error) {
	row, err := s.q.FindGame(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return cached{}, false, nil
	}
	if err != nil {
		return cached{}, false, err
	}
	c := cached{
		ID: row.ID, Slug: row.Slug, Name: row.Name, BackgroundImage: row.BackgroundImage,
		Rating: row.Rating, Metacritic: row.Metacritic, DescriptionRaw: row.DescriptionRaw,
		Genres: row.Genres, Platforms: row.Platforms, RefreshedAt: row.RefreshedAt.Time,
	}
	if row.Released.Valid {
		t := row.Released.Time
		c.Released = &t
	}
	return c, true, nil
}

func (s pgStore) UpsertFromSearch(ctx context.Context, g game, now time.Time) (string, error) {
	raw := g.RawPayload
	return s.q.UpsertGameFromSearch(ctx, sqlc.UpsertGameFromSearchParams{
		ID: g.ID, Slug: g.Slug, Name: g.Name, BackgroundImage: g.BackgroundImage,
		Released: date(g.Released), Rating: g.Rating, Metacritic: g.Metacritic,
		Genres: g.Genres, Platforms: g.Platforms, RawPayload: &raw, Now: ts(now),
	})
}

func (s pgStore) UpsertFromDetail(ctx context.Context, g game, description *string, now time.Time) (string, error) {
	raw := g.RawPayload
	return s.q.UpsertGameFromDetail(ctx, sqlc.UpsertGameFromDetailParams{
		ID: g.ID, Slug: g.Slug, Name: g.Name, BackgroundImage: g.BackgroundImage,
		Released: date(g.Released), Rating: g.Rating, Metacritic: g.Metacritic,
		DescriptionRaw: description, Genres: g.Genres, Platforms: g.Platforms,
		RawPayload: &raw, Now: ts(now),
	})
}

func date(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
