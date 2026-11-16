package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
	"github.com/JamesWillian/loobby-api/api-go/internal/kotlin"
	"github.com/JamesWillian/loobby-api/api-go/internal/password"
)

// Handler: AuthController.
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register publica as rotas de /auth. Quais sao publicas e quais exigem token
// quem decide e o Filter (ver server.isPublic): register e resend-verification
// exigem; as demais nao.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/anonymous", h.anonymous)
	mux.HandleFunc("POST /auth/register", h.register)
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("POST /auth/google", h.google)
	mux.HandleFunc("POST /auth/refresh", h.refresh)
	mux.HandleFunc("GET /auth/verify-email", h.verifyEmail)
	mux.HandleFunc("POST /auth/resend-verification", h.resendVerification)
	mux.HandleFunc("POST /auth/forgot-password", h.forgotPassword)
	mux.HandleFunc("GET /auth/reset-password", h.resetPasswordForm)
	mux.HandleFunc("POST /auth/reset-password", h.resetPassword)
}

// message: o mapOf("message" to ...) de algumas respostas.
type message struct {
	Message string `json:"message"`
}

// html escreve uma pagina com o Content-Type literal do Kotlin: "text/html",
// sem charset.
func html(w http.ResponseWriter, status int, page string) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(page))
}

// O corpo e ignorado: o metodo do Kotlin nao tem @RequestBody.
func (h *Handler) anonymous(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.CreateAnonymous(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	email, pass := body.String("email"), body.String("password")
	httpx.Req(body, email)
	httpx.Req(body, pass)
	if err := body.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v := httpx.NewViolations(r)
	v.Email("email", email)
	v.NotBlank("email", email)
	v.NotBlank("password", pass)
	v.Size("password", pass, 6, 100)
	if v.Any() {
		v.Write(w, r)
		return
	}
	userID, err := UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.Register(r.Context(), userID, *email, *pass)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// O cadastro ja esta gravado quando o email e enviado: se o envio falhar, a
	// resposta e 500 mas a conta existe -- igual ao Kotlin.
	if err := h.svc.SendVerificationEmail(r.Context(), userID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// login nao valida o formato do corpo (o Kotlin nao tem @Valid aqui): email
// malformado e simplesmente um email que nao existe -> "Invalid credentials".
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	email := httpx.Req(body, body.String("email"))
	pass := httpx.Req(body, body.String("password"))
	if err := body.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.Login(r.Context(), email, pass)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Conta ainda nao verificada recebe um email novo a cada login.
	verified, err := h.svc.IsEmailVerified(r.Context(), resp.UserID)
	if err == nil && !verified {
		err = h.svc.SendVerificationEmail(r.Context(), resp.UserID)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) google(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	idToken := httpx.Req(body, body.String("idToken"))
	if err := body.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.LoginWithGoogle(r.Context(), idToken)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	token := httpx.Req(body, body.String("refreshToken"))
	if err := body.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.Refresh(r.Context(), token)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// queryToken le o ?token= obrigatorio. Ausente e
// MissingServletRequestParameterException no Kotlin: 500 (medido). Presente e
// vazio e aceito e segue como token inexistente.
func queryToken(w http.ResponseWriter, r *http.Request) (string, bool) {
	values, ok := r.URL.Query()["token"]
	if !ok {
		httpx.Unexpected(w, r)
		return "", false
	}
	return strings.Join(values, ","), true
}

// pageError separa os erros que viram pagina HTML (os ResponseStatusException
// do Kotlin, que o controller captura) dos que seguem para o envelope JSON.
func pageError(err error) (*httpx.Error, bool) {
	var e *httpx.Error
	if errors.As(err, &e) && !errors.Is(err, password.ErrTooLong) {
		return e, true
	}
	return nil, false
}

func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	token, ok := queryToken(w, r)
	if !ok {
		return
	}
	err := h.svc.VerifyEmail(r.Context(), token)
	if e, isPage := pageError(err); isPage {
		html(w, e.Status, fill(verifyErrorPage, "$message", e.Error()))
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	html(w, http.StatusOK, verifySuccessPage)
}

func (h *Handler) resendVerification(w http.ResponseWriter, r *http.Request) {
	userID, err := UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.ResendVerificationEmail(r.Context(), userID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, message{"Email de verificação reenviado"})
}

// forgotPassword responde sempre a mesma coisa, exista o email ou nao.
func (h *Handler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	email := httpx.Req(body, body.String("email"))
	if err := body.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// O email e posto em minusculas aqui, mas o cadastro grava como digitado e
	// a busca e exata: conta registrada com maiuscula no email nunca e achada
	// por esta rota. E assim no Kotlin (contract/ACHADOS.md).
	err = h.svc.RequestPasswordReset(r.Context(), strings.ToLower(kotlin.Trim(email)))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, message{"Se o email estiver cadastrado, você receberá um link para redefinir a senha."})
}

func (h *Handler) resetPasswordForm(w http.ResponseWriter, r *http.Request) {
	token, ok := queryToken(w, r)
	if !ok {
		return
	}
	email, err := h.svc.ValidateResetToken(r.Context(), token)
	if e, isPage := pageError(err); isPage {
		html(w, e.Status, fill(resetErrorPage, "$message", e.Error()))
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	html(w, http.StatusOK, fill(resetFormPage, "$email", email, "$token", token))
}

// resetPassword recebe JSON (o formulario envia por fetch) e responde HTML.
func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	token := httpx.Req(body, body.String("token"))
	pass := httpx.Req(body, body.String("password"))
	if err := body.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err = h.svc.ResetPassword(r.Context(), token, pass)
	if e, isPage := pageError(err); isPage {
		html(w, e.Status, fill(resetErrorPage, "$message", e.Error()))
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	html(w, http.StatusOK, resetSuccessPage)
}
