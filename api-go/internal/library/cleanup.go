package library

import (
	"context"

	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
)

// DeleteUserData e a parte das bibliotecas na exclusao de conta. Roda dentro
// da transacao do users.DeleteAccount, que so anonimiza a linha de users -- as
// FKs com CASCADE nao chegam a agir.
//
// A biblioteca da pessoa sai inteira. Os jogos que ela adicionou a biblioteca
// de um grupo ficam com o grupo, sem quem os adicionou.
func DeleteUserData(ctx context.Context, q *sqlc.Queries, userID string) error {
	if err := q.DeleteUserLibrary(ctx, userID); err != nil {
		return err
	}
	return q.ClearGroupLibraryAddedBy(ctx, userID)
}
