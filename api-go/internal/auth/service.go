package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/email"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
	"github.com/JamesWillian/loobby-api/api-go/internal/password"
)

// authResponse: AuthResponse.
type authResponse struct {
	AccessToken  string   `json:"accessToken"`
	RefreshToken string   `json:"refreshToken"`
	ExpiresIn    int64    `json:"expiresIn"` // segundos de validade do access token
	UserID       string   `json:"userId"`
	Username     string   `json:"username"`
	Roles        []string `json:"roles"`
}

// Options sao os valores de configuracao do modulo.
type Options struct {
	BaseURL string // base dos links enviados por email (app.base-url)

	// AllowUnsignedAnonRefresh reproduz uma falha do Kotlin; ver Service.Refresh.
	AllowUnsignedAnonRefresh bool
}

// Service: AuthService + EmailVerificationService + PasswordResetService +
// GoogleAuthService do Kotlin.
type Service struct {
	pool   *pgxpool.Pool
	q      *sqlc.Queries
	tokens *Tokens
	mail   email.Sender
	google *GoogleVerifier
	opts   Options
	now    func() time.Time
}

func NewService(pool *pgxpool.Pool, tokens *Tokens, mail email.Sender, google *GoogleVerifier, opts Options) *Service {
	return &Service{pool: pool, q: sqlc.New(pool), tokens: tokens, mail: mail, google: google, opts: opts, now: time.Now}
}

var (
	errUserNotFound     = httpx.BadRequest("User not found")
	errRefreshInvalid   = httpx.Status(http.StatusUnauthorized, "Refresh token expired or invalid")
	errUsuarioNaoExiste = httpx.Status(http.StatusNotFound, "Usuário não encontrado")
)

// issue: AuthService.generateAuthResponseForUser. Papel ANON para conta
// anonima (auth_provider 0), USER para as demais.
func (s *Service) issue(u sqlc.User) authResponse {
	roles := []string{"USER"}
	if u.AuthProvider == 0 {
		roles = []string{"ANON"}
	}
	return authResponse{
		AccessToken:  s.tokens.Access(u.ID, u.Username, roles),
		RefreshToken: s.tokens.Refresh(u.ID),
		ExpiresIn:    int64(AccessTokenValidity / time.Second),
		UserID:       u.ID,
		Username:     u.Username,
		Roles:        roles,
	}
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

// CreateAnonymous: AuthService.createAnonymous. O usuario nasce com username
// "user_" + 6 hexadecimais, que tambem e o displayname.
//
// Se o username sorteado ja existir, sorteia outro. (O Kotlin nao tenta de
// novo: a colisao, rara, vira 500.)
func (s *Service) CreateAnonymous(ctx context.Context) (authResponse, error) {
	for attempt := 0; ; attempt++ {
		u := sqlc.User{ID: ids.New()}
		u.Username = "user_" + ids.New()[:6]
		u.Displayname = u.Username
		err := s.q.InsertUser(ctx, sqlc.InsertUserParams{ID: u.ID, Username: u.Username, Displayname: u.Displayname})
		if err == nil {
			return s.issue(u), nil
		}
		if !isUniqueViolation(err) || attempt == 4 {
			return authResponse{}, err
		}
	}
}

// Register: AuthService.register -- o anonimo ganha email e senha e passa a
// ser uma conta de verdade (auth_provider 1). O email e gravado como digitado.
//
// Email ja usado por outra conta esbarra na restricao unica do banco e a
// resposta e 500, nao um erro de negocio -- igual ao Kotlin (contract/ACHADOS.md).
func (s *Service) Register(ctx context.Context, userID, emailAddr, pass string) (authResponse, error) {
	var resp authResponse
	err := db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		u, err := q.FindUser(ctx, userID)
		if db.NotFound(err) {
			return errUserNotFound
		}
		if err != nil {
			return err
		}
		if exists, err := q.CredentialsExist(ctx, userID); err != nil {
			return err
		} else if exists {
			return httpx.BadRequest("User already registered")
		}
		if pass == "" {
			return httpx.BadRequest("Password invalid")
		}
		hash, err := password.Hash(pass)
		if err != nil {
			return err
		}
		u.AuthProvider = 1
		if err := q.SetUserAuthProvider(ctx, sqlc.SetUserAuthProviderParams{ID: userID, AuthProvider: 1}); err != nil {
			return err
		}
		err = q.InsertCredentials(ctx, sqlc.InsertCredentialsParams{UserID: userID, Email: emailAddr, PasswordHash: hash})
		if err != nil {
			return err
		}
		resp = s.issue(u)
		return nil
	})
	return resp, err
}

// Login: AuthService.login. Email e comparado exatamente como foi gravado.
func (s *Service) Login(ctx context.Context, emailAddr, pass string) (authResponse, error) {
	invalid := httpx.BadRequest("Invalid credentials")
	var resp authResponse
	err := db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		creds, err := q.FindCredentialsByEmail(ctx, emailAddr)
		if db.NotFound(err) {
			return invalid
		}
		if err != nil {
			return err
		}
		if !password.Matches(pass, creds.PasswordHash) {
			return invalid
		}
		u, err := q.FindUser(ctx, creds.UserID)
		if db.NotFound(err) {
			return errUserNotFound
		}
		if err != nil {
			return err
		}
		// Conserto de dado antigo: uma versao anterior do cadastro nao gravava
		// auth_provider = 1. Quem tem credenciais de email e conta de email.
		if u.AuthProvider == 0 {
			u.AuthProvider = 1
			if err := q.SetUserAuthProvider(ctx, sqlc.SetUserAuthProviderParams{ID: u.ID, AuthProvider: 1}); err != nil {
				return err
			}
		}
		resp = s.issue(u)
		return nil
	})
	return resp, err
}

// IsEmailVerified: EmailVerificationService.isEmailVerified.
func (s *Service) IsEmailVerified(ctx context.Context, userID string) (bool, error) {
	creds, err := s.q.FindCredentials(ctx, userID)
	if db.NotFound(err) {
		return false, nil
	}
	return creds.EmailVerified, err
}

// Refresh troca um refresh token por um par novo de tokens.
//
// Token valido: precisa ser do tipo "refresh" e de um usuario que existe.
//
// Token recusado (vencido, assinatura errada, ilegivel): a resposta e 401,
// com UMA excecao -- conta anonima com refresh VENCIDO e renovada mesmo assim.
// O anonimo nao tem email nem senha; se o refresh dele vencer (15 dias sem
// abrir o app), nao ha outro jeito de voltar a conta.
//
// Onde o Go difere do Kotlin, de proposito: o Kotlin aplica essa excecao lendo
// o token SEM conferir a assinatura. Basta saber o id de um usuario anonimo --
// e o id aparece para qualquer membro do mesmo grupo -- para fabricar um
// "refresh token" e receber tokens validos da conta dele. Medido: um token
// assinado com um segredo qualquer foi aceito. Aqui a excecao so vale para
// token com assinatura correta; so a validade e relevada.
// AllowUnsignedAnonRefresh restaura o comportamento do Kotlin, e so faz
// sentido se o JWT_SECRET tiver sido trocado e houver anonimos com token
// antigo a preservar (contract/ACHADOS.md).
func (s *Service) Refresh(ctx context.Context, raw string) (authResponse, error) {
	claims, terr := s.tokens.Verify(raw)
	if terr == nil {
		if !claims.HasType {
			return authResponse{}, httpx.BadRequest("Invalid token: missing type")
		}
		if claims.Type != "refresh" {
			return authResponse{}, httpx.BadRequest("Invalid token type")
		}
		userID, ok := httpx.ParseJavaUUID(claims.Subject)
		if !ok {
			return authResponse{}, httpx.BadRequest("Invalid UUID string: " + claims.Subject)
		}
		u, err := s.q.FindUser(ctx, userID)
		if db.NotFound(err) {
			return authResponse{}, errUserNotFound
		}
		if err != nil {
			return authResponse{}, err
		}
		return s.issue(u), nil
	}

	var ok bool
	if s.opts.AllowUnsignedAnonRefresh {
		claims, ok = UnverifiedClaims(raw)
	} else {
		claims, ok = s.tokens.VerifyIgnoringTime(raw)
	}
	if !ok || claims.Type != "refresh" {
		return authResponse{}, errRefreshInvalid
	}
	userID, ok := httpx.ParseJavaUUID(claims.Subject)
	if !ok {
		return authResponse{}, errRefreshInvalid
	}
	u, err := s.q.FindUser(ctx, userID)
	if db.NotFound(err) || (err == nil && u.AuthProvider != 0) {
		return authResponse{}, errRefreshInvalid
	}
	if err != nil {
		return authResponse{}, err
	}
	return s.issue(u), nil
}
