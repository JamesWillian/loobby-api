// Package users e a porta do UsersController e do UsersService do Kotlin:
// perfil, avatar, feed, troca de senha e exclusao de conta.
package users

import (
	"context"
	"net/http"
	"strings"
	"unicode/utf16"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/gameplays"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
	"github.com/JamesWillian/loobby-api/api-go/internal/kotlin"
	"github.com/JamesWillian/loobby-api/api-go/internal/library"
	"github.com/JamesWillian/loobby-api/api-go/internal/password"
)

type Service struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: sqlc.New(pool)}
}

var (
	errUserNotFound = httpx.BadRequest("User not found")
	// errSemConta: o 404 da troca de senha e da exclusao de conta.
	errSemConta = httpx.Status(http.StatusNotFound, "Usuário não encontrado.")
)

func findUser(ctx context.Context, q *sqlc.Queries, id string) (sqlc.User, error) {
	u, err := q.FindUser(ctx, id)
	if db.NotFound(err) {
		return sqlc.User{}, errUserNotFound
	}
	return u, err
}

// hasGoogle: a conta entra com o Google (tem linha em user_google_credentials).
func hasGoogle(ctx context.Context, q *sqlc.Queries, userID string) (bool, error) {
	_, err := q.FindGoogleCredentials(ctx, userID)
	if db.NotFound(err) {
		return false, nil
	}
	return err == nil, err
}

func instant(u sqlc.User) *httpx.Instant {
	t := httpx.Instant(u.CreatedAt)
	return &t
}

func profile(u sqlc.User, displayname *string) profileResponse {
	return profileResponse{
		ID: u.ID, Username: u.Username, Displayname: displayname,
		AvatarURL: u.AvatarUrl, CreatedAt: instant(u),
	}
}

// Me: UsersService.getMe.
func (s *Service) Me(ctx context.Context, userID string) (meResponse, error) {
	u, err := findUser(ctx, s.q, userID)
	if err != nil {
		return meResponse{}, err
	}
	creds, err := s.q.FindCredentials(ctx, userID)
	hasCreds := err == nil
	if err != nil && !db.NotFound(err) {
		return meResponse{}, err
	}
	google, err := s.q.FindGoogleCredentials(ctx, userID)
	hasGoogle := err == nil
	if err != nil && !db.NotFound(err) {
		return meResponse{}, err
	}

	resp := meResponse{
		ID: u.ID, Username: u.Username, Displayname: &u.Displayname, AvatarURL: u.AvatarUrl,
		IsAnonymous: u.AuthProvider == 0, Roles: []string{"USER"}, CreatedAt: instant(u),
	}
	if u.AuthProvider == 0 {
		resp.Roles = []string{"ANON"}
	}
	// email: o das credenciais de senha para conta de email (provider 1); para
	// as demais, o do Google -- que nao existe para anonimo, e ai sai null.
	switch {
	case u.AuthProvider == 1 && hasCreds:
		resp.Email = &creds.Email
	case u.AuthProvider != 1 && hasGoogle:
		resp.Email = &google.Email
	}
	// sem credenciais de senha, vale "nao e anonimo" (conta Google ja nasce verificada)
	resp.EmailVerified = u.AuthProvider != 0
	if hasCreds {
		resp.EmailVerified = creds.EmailVerified
	}
	return resp, nil
}

// profileUpdate: UpdateUserProfileRequest. nil = campo nao enviado (nao altera).
type profileUpdate struct {
	Username, Displayname, AvatarURL *string
}

// UpdateProfile: UsersService.updateUserProfile.
func (s *Service) UpdateProfile(ctx context.Context, userID string, req profileUpdate) (profileResponse, error) {
	var resp profileResponse
	err := db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		u, err := findUser(ctx, q, userID)
		if err != nil {
			return err
		}
		if req.Username != nil {
			name := kotlin.Trim(*req.Username)
			if name == "" {
				return httpx.BadRequest("Username cannot be blank")
			}
			taken, err := q.UsernameTaken(ctx, name)
			if err != nil {
				return err
			}
			if taken && !strings.EqualFold(name, u.Username) {
				return httpx.BadRequest("Username is already taken")
			}
			u.Username = name
		}
		// displayname e avatarUrl em branco viram null ("limpar"). Para o avatar
		// isso funciona. Para o displayname a coluna e NOT NULL: o banco recusa e
		// a resposta e 500, como no Kotlin (contract/ACHADOS.md).
		displayname := &u.Displayname
		if req.Displayname != nil {
			displayname = kotlin.IfBlankNil(kotlin.Trim(*req.Displayname))
		}
		if req.AvatarURL != nil {
			u.AvatarUrl = kotlin.IfBlankNil(kotlin.Trim(*req.AvatarURL))
		}
		err = q.UpdateUserProfile(ctx, sqlc.UpdateUserProfileParams{
			ID: u.ID, Username: u.Username, Displayname: displayname, AvatarUrl: u.AvatarUrl,
		})
		if err != nil {
			return err
		}
		resp = profile(u, displayname)
		return nil
	})
	return resp, err
}

// UpdateAvatar: UsersService.updateAvatar.
func (s *Service) UpdateAvatar(ctx context.Context, userID, avatarURL string) (profileResponse, error) {
	u, err := findUser(ctx, s.q, userID)
	if err != nil {
		return profileResponse{}, err
	}
	if err := s.q.UpdateUserAvatar(ctx, sqlc.UpdateUserAvatarParams{ID: userID, AvatarUrl: &avatarURL}); err != nil {
		return profileResponse{}, err
	}
	u.AvatarUrl = &avatarURL
	return profile(u, &u.Displayname), nil
}

// Feed: UsersService.getUserFeed. Nao confere se o usuario existe, como no Kotlin.
func (s *Service) Feed(ctx context.Context, userID string) ([]feedResponse, error) {
	rows, err := s.q.UserFeed(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]feedResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, feedResponse{
			ID: r.ID, UserID: r.UserID, Name: r.Name, ImageURL: r.ImageUrl,
			EntryType: r.EntryType, IsFinished: r.IsFinished,
		})
	}
	return out, nil
}

// javaLen: String.length() -- unidades UTF-16, nao bytes nem runas.
func javaLen(s string) int { return len(utf16.Encode([]rune(s))) }

// ChangePassword: UsersService.changePassword. A ordem das checagens e a do
// Kotlin, e define qual mensagem sai quando ha mais de um problema.
//
// Conta so com Google nao tem senha para trocar. Ela recebe 400 com uma mensagem
// que diz isso, e os apps mostram na tela a mensagem dos 400. O Kotlin respondia
// 404 "Usuário não encontrado." (contract/ACHADOS.md §24). Anonimo continua com
// esse 404.
func (s *Service) ChangePassword(ctx context.Context, userID, current, newPass, confirm string) error {
	if javaLen(newPass) < 6 {
		return httpx.Status(http.StatusBadRequest, "A nova senha deve ter pelo menos 6 caracteres.")
	}
	if newPass != confirm {
		return httpx.Status(http.StatusBadRequest, "As senhas não coincidem.")
	}
	if current == newPass {
		return httpx.Status(http.StatusBadRequest, "A nova senha deve ser diferente da atual.")
	}
	return db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		creds, err := q.FindCredentials(ctx, userID)
		if db.NotFound(err) {
			google, err := hasGoogle(ctx, q, userID)
			if err != nil {
				return err
			}
			if google {
				return httpx.Status(http.StatusBadRequest, "Sua conta entra com o Google e não tem senha.")
			}
			return errSemConta
		}
		if err != nil {
			return err
		}
		if !password.Matches(current, creds.PasswordHash) {
			return httpx.Status(http.StatusUnauthorized, "Senha atual incorreta.")
		}
		hash, err := password.Hash(newPass)
		if err != nil {
			return err
		}
		return q.UpdatePasswordHash(ctx, sqlc.UpdatePasswordHashParams{UserID: userID, PasswordHash: hash})
	})
}

// DeleteAccount: UsersService.deleteAccount, com os dois consertos do
// contract/ACHADOS.md (§23 e §24). Tudo numa transacao.
//
// O que confirma a exclusao depende da conta (confirmDeletion):
//   - com senha: a senha certa, como no Kotlin;
//   - so com Google, que nao tem senha: o proprio token. Os apps 1.3 e 1.3.1
//     sempre mandam o campo password, e aqui o valor e ignorado;
//   - sem nenhuma das duas (anonima, ou ja excluida): 404, como no Kotlin.
//
// O registro em users nao e apagado -- outras tabelas apontam para ele. Ele e
// esvaziado: username "", displayname "[deleted]". Times e sorteios, aqui e nos
// apps, reconhecem a conta excluida pelo username vazio. O indice unico do
// username ignora o vazio (migracao 00003), entao cabem quantas contas forem
// excluidas. No Kotlin so cabia uma: da segunda exclusao em diante, 500.
//
// As credenciais saem todas, as de senha e as do Google. Sem apagar as do
// Google, entrar de novo com a mesma conta Google cairia na conta excluida.
//
// Na 2.0, votos, comentarios, denuncias e participacoes nas gameplays saem, e
// as gameplays que a pessoa criou passam para outra (gameplays.DeleteUserData).
// A biblioteca dela sai, e os jogos que ela pos na de um grupo ficam com o grupo
// (library.DeleteUserData). Nada disso aparece na resposta, que e a mesma da v1.
func (s *Service) DeleteAccount(ctx context.Context, userID, pass string) error {
	return db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		if err := confirmDeletion(ctx, q, userID, pass); err != nil {
			return err
		}

		// Grupos do usuario: passam para outro membro, ou somem se nao houver.
		groups, err := q.FindGroupsByOwner(ctx, userID)
		if err != nil {
			return err
		}
		for _, g := range groups {
			next, err := q.FindFirstOtherMember(ctx, sqlc.FindFirstOtherMemberParams{GroupID: g.ID, UserID: userID})
			switch {
			case db.NotFound(err):
				err = q.DeleteGroup(ctx, g.ID) // CASCADE leva eventos, RSVPs e times
			case err == nil:
				err = q.UpdateGroupOwner(ctx, sqlc.UpdateGroupOwnerParams{ID: g.ID, OwnerID: next})
			}
			if err != nil {
				return err
			}
		}

		// Eventos instantaneos do usuario: passam para o confirmado mais antigo.
		events, err := q.FindInstantEventsByOwner(ctx, userID)
		if err != nil {
			return err
		}
		for _, e := range events {
			next, err := q.FindFirstOtherRsvp(ctx, sqlc.FindFirstOtherRsvpParams{
				EventID: e.ID, UserID: userID, Status: "YES",
			})
			switch {
			case db.NotFound(err):
				err = q.DeleteEvent(ctx, e.ID)
			case err == nil:
				err = q.UpdateEventOwner(ctx, sqlc.UpdateEventOwnerParams{ID: e.ID, OwnerID: next})
			}
			if err != nil {
				return err
			}
		}

		if err := gameplays.DeleteUserData(ctx, q, userID); err != nil {
			return err
		}
		if err := library.DeleteUserData(ctx, q, userID); err != nil {
			return err
		}

		if err := q.ClearRsvpObsByUser(ctx, userID); err != nil {
			return err
		}
		if err := q.DeleteCredentials(ctx, userID); err != nil {
			return err
		}
		if err := q.DeleteGoogleCredentials(ctx, userID); err != nil {
			return err
		}
		if _, err := q.FindUser(ctx, userID); db.NotFound(err) {
			return errSemConta
		} else if err != nil {
			return err
		}
		return q.AnonymizeUser(ctx, userID)
	})
}

// confirmDeletion: a conta com senha confirma a exclusao com a senha. A conta so
// com Google confirma com o token que trouxe a requisicao ate aqui, porque nao
// tem senha (contract/ACHADOS.md §24).
func confirmDeletion(ctx context.Context, q *sqlc.Queries, userID, pass string) error {
	creds, err := q.FindCredentials(ctx, userID)
	switch {
	case err == nil:
		if !password.Matches(pass, creds.PasswordHash) {
			return httpx.Status(http.StatusUnauthorized, "Senha incorreta.")
		}
		return nil
	case !db.NotFound(err):
		return err
	}
	google, err := hasGoogle(ctx, q, userID)
	if err != nil {
		return err
	}
	if !google {
		return errSemConta
	}
	return nil
}
