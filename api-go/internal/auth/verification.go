package auth

import (
	"context"
	"net/http"
	"time"
	"unicode/utf16"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
	"github.com/JamesWillian/loobby-api/api-go/internal/password"
)

// Valores de app.email-verification e app.password-reset no application.yaml.
const (
	verificationValidity = 24 * time.Hour
	resetValidity        = time.Hour
	resendCooldown       = 3 * time.Minute
)

// SendVerificationEmail: EmailVerificationService.sendVerificationEmail. Gera
// um token novo (o anterior deixa de valer) e manda o link.
//
// Tudo numa transacao, como no Kotlin: se o Resend falhar, o token novo nao
// fica gravado e o erro sobe (500 para o cliente).
func (s *Service) SendVerificationEmail(ctx context.Context, userID string) error {
	return db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		creds, err := q.FindCredentials(ctx, userID)
		if db.NotFound(err) {
			return errUsuarioNaoExiste
		}
		if err != nil {
			return err
		}
		if creds.EmailVerified {
			return nil
		}
		token, expires := ids.New(), s.now().Add(verificationValidity)
		err = q.SetVerificationToken(ctx, sqlc.SetVerificationTokenParams{
			UserID: userID, Token: token, ExpiresAt: &expires,
		})
		if err != nil {
			return err
		}
		link := s.opts.BaseURL + "/auth/verify-email?token=" + token
		return s.mail.Send(ctx, creds.Email, "Confirme seu email no Loobby", fill(verificationEmail, "$verifyUrl", link))
	})
}

// VerifyEmail: EmailVerificationService.verifyEmail.
func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	return db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		creds, err := q.FindCredentialsByVerificationToken(ctx, token)
		if db.NotFound(err) {
			return httpx.Status(http.StatusBadRequest, "Token inválido ou já utilizado")
		}
		if err != nil {
			return err
		}
		if creds.EmailVerified {
			return nil
		}
		if exp := creds.EmailVerificationExpiresAt; exp == nil || s.now().After(*exp) {
			return httpx.Status(http.StatusBadRequest, "Token expirado. Solicite um novo email de verificação.")
		}
		return q.MarkEmailVerified(ctx, creds.UserID)
	})
}

// ResendVerificationEmail: EmailVerificationService.resendVerificationEmail.
// So reenvia 3 minutos depois do ultimo envio; o horario do ultimo envio e
// deduzido da validade do token (validade - 24 h).
func (s *Service) ResendVerificationEmail(ctx context.Context, userID string) error {
	creds, err := s.q.FindCredentials(ctx, userID)
	if db.NotFound(err) {
		return errUsuarioNaoExiste
	}
	if err != nil {
		return err
	}
	if creds.EmailVerified {
		return httpx.Status(http.StatusBadRequest, "Email já verificado")
	}
	if exp := creds.EmailVerificationExpiresAt; exp != nil {
		sentAt := exp.Add(-verificationValidity)
		if s.now().Before(sentAt.Add(resendCooldown)) {
			return httpx.Status(http.StatusTooManyRequests, "Aguarde alguns minutos antes de solicitar um novo email.")
		}
	}
	return s.SendVerificationEmail(ctx, userID)
}

// RequestPasswordReset: PasswordResetService.requestPasswordReset. Nunca diz
// se o email existe: email desconhecido e pedido repetido dentro de 3 minutos
// sao ignorados em silencio.
func (s *Service) RequestPasswordReset(ctx context.Context, emailAddr string) error {
	return db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		creds, err := q.FindCredentialsByEmail(ctx, emailAddr)
		if db.NotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if exp := creds.PasswordResetExpiresAt; exp != nil {
			sentAt := exp.Add(-resetValidity)
			if s.now().Before(sentAt.Add(resendCooldown)) {
				return nil
			}
		}
		token, expires := ids.New(), s.now().Add(resetValidity)
		err = q.SetResetToken(ctx, sqlc.SetResetTokenParams{
			UserID: creds.UserID, Token: token, ExpiresAt: &expires,
		})
		if err != nil {
			return err
		}
		link := s.opts.BaseURL + "/auth/reset-password?token=" + token
		return s.mail.Send(ctx, creds.Email, "Redefinir senha — Loobby", fill(resetEmail, "$resetUrl", link))
	})
}

func (s *Service) credentialsForReset(ctx context.Context, q *sqlc.Queries, token string) (sqlc.UserCredential, error) {
	creds, err := q.FindCredentialsByResetToken(ctx, token)
	if db.NotFound(err) {
		return creds, httpx.Status(http.StatusBadRequest, "Link inválido ou já utilizado.")
	}
	if err != nil {
		return creds, err
	}
	if exp := creds.PasswordResetExpiresAt; exp == nil || s.now().After(*exp) {
		return creds, httpx.Status(http.StatusBadRequest, "Link expirado. Solicite uma nova redefinição.")
	}
	return creds, nil
}

// ValidateResetToken: PasswordResetService.validateResetToken. Devolve o email
// da conta, que aparece no formulario.
func (s *Service) ValidateResetToken(ctx context.Context, token string) (string, error) {
	creds, err := s.credentialsForReset(ctx, s.q, token)
	return creds.Email, err
}

// ResetPassword: PasswordResetService.resetPassword. O token vale uma vez.
func (s *Service) ResetPassword(ctx context.Context, token, newPassword string) error {
	if javaLen(newPassword) < 6 {
		return httpx.Status(http.StatusBadRequest, "A senha deve ter pelo menos 6 caracteres.")
	}
	return db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		creds, err := s.credentialsForReset(ctx, q, token)
		if err != nil {
			return err
		}
		hash, err := password.Hash(newPassword)
		if err != nil {
			return err
		}
		return q.ResetPassword(ctx, sqlc.ResetPasswordParams{UserID: creds.UserID, PasswordHash: hash})
	})
}

// javaLen: String.length() -- unidades UTF-16.
func javaLen(s string) int { return len(utf16.Encode([]rune(s))) }
