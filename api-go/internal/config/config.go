package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config espelha o application.yaml do lado Kotlin. As variaveis de ambiente sao
// as MESMAS, de proposito: durante o strangler os dois processos rodam lado a
// lado e devem ler a mesma configuracao -- em especial o JWT_SECRET, que e o que
// permite os tokens emitidos pelo Kotlin continuarem valendo no Go.
type Config struct {
	Port        string
	DatabaseURL string

	JWTSecret string

	// AcceptRefreshTokenAsAccess replica um comportamento MEDIDO do Kotlin: o
	// resource server valida assinatura e expiracao mas NAO checa o claim
	// `type`, entao um refresh token (15 dias) e aceito onde deveria ir um
	// access token (15 minutos). Verificado em producao: GET /users/me com o
	// refresh no Bearer devolve 200.
	//
	// Default true = paridade com o Kotlin. Colocar em false endurece o Go, mas
	// e mudanca de comportamento e precisa ser decisao consciente, com os apps
	// verificados antes.
	AcceptRefreshTokenAsAccess bool

	BaseURL string

	// RawgAPIKey: a mesma do Kotlin. Obrigatoria, como la -- sem ela, o lateinit
	// do RawgProperties derruba a aplicacao na partida.
	RawgAPIKey string

	// TLS mutuo. O Go roda numa maquina separada do nginx (que fica no AWS), e o
	// trafego entre os dois atravessa a internet levando os tokens de login dos
	// usuarios. Com os tres arquivos definidos, o servidor so completa o
	// handshake com quem apresentar um certificado de cliente assinado pela CA
	// privada -- na pratica, so o nginx. Sem eles, o servidor fala HTTP puro e
	// escuta APENAS em 127.0.0.1: HTTP sem TLS nunca fica exposto na rede.
	TLSCertFile     string
	TLSKeyFile      string
	TLSClientCAFile string

	// BindHost sobrepoe o host de escuta. Vazio = automatico (ver ListenAddr).
	BindHost string

	// RawgBaseURL: a API do catalogo de jogos. So muda em teste.
	RawgBaseURL string

	// Resend (envio de email). A chave e obrigatoria, como no Kotlin.
	ResendAPIKey  string
	ResendBaseURL string

	// GoogleClientIDs: os client ids (web, iOS, Android) aceitos como audiencia
	// de um ID token do "Entrar com Google". Obrigatorios, como no Kotlin.
	GoogleClientIDs []string
	GoogleCertsURL  string

	// FileStorageLocation: pasta dos uploads (avatares, imagens de grupo).
	FileStorageLocation string

	// Notificacoes push. Com NotificationsEnabled, o arquivo de credenciais do
	// Firebase precisa existir -- senao o processo nao sobe, como no Kotlin.
	NotificationsEnabled    bool
	FirebaseCredentialsPath string

	// NotificationSchedulerEnabled liga os jobs agendados (lembretes e avisos
	// diarios). O registro de envios no banco impede push duplicado mesmo com
	// mais de um processo rodando os jobs.
	NotificationSchedulerEnabled bool

	// AllowUnsignedAnonRefresh reproduz uma falha de seguranca do Kotlin no
	// /auth/refresh (aceitar refresh token de anonimo sem conferir a
	// assinatura). Default false = corrigido. Ver auth.Service.Refresh.
	AllowUnsignedAnonRefresh bool
}

// ResendFromEmail: o remetente fixo do application.yaml do Kotlin.
const ResendFromEmail = "Loobby <noreply@mail.loobby.app>"

// TLSEnabled informa se o servidor deve exigir TLS mutuo.
func (c Config) TLSEnabled() bool { return c.TLSCertFile != "" }

// ListenAddr: com TLS, todas as interfaces (o nginx vem de outra maquina); sem
// TLS, so loopback. BIND_HOST sobrepoe os dois casos.
func (c Config) ListenAddr() string {
	host := c.BindHost
	if host == "" && !c.TLSEnabled() {
		host = "127.0.0.1"
	}
	return host + ":" + c.Port
}

func Load() (Config, error) {
	c := Config{
		Port:                       envOr("PORT", "8080"),
		DatabaseURL:                normalizeDatabaseURL(os.Getenv("DB_URL")),
		JWTSecret:                  os.Getenv("JWT_SECRET"),
		AcceptRefreshTokenAsAccess: envBool("ACCEPT_REFRESH_AS_ACCESS", true),
		BaseURL:                    envOr("APP_BASE_URL", "https://api.loobby.app"),
		RawgAPIKey:                 os.Getenv("RAWG_API_KEY"),
		TLSCertFile:                os.Getenv("TLS_CERT_FILE"),
		TLSKeyFile:                 os.Getenv("TLS_KEY_FILE"),
		TLSClientCAFile:            os.Getenv("TLS_CLIENT_CA_FILE"),
		BindHost:                   os.Getenv("BIND_HOST"),

		RawgBaseURL:   envOr("RAWG_BASE_URL", "https://api.rawg.io/api"),
		ResendAPIKey:  os.Getenv("RESEND_API_KEY"),
		ResendBaseURL: envOr("RESEND_BASE_URL", "https://api.resend.com"),
		GoogleClientIDs: []string{
			os.Getenv("GOOGLE_WEB_CLIENT_ID"), os.Getenv("GOOGLE_IOS_CLIENT_ID"), os.Getenv("GOOGLE_ANDROID_CLIENT_ID"),
		},
		GoogleCertsURL:               envOr("GOOGLE_CERTS_URL", "https://www.googleapis.com/oauth2/v3/certs"),
		FileStorageLocation:          envOr("FILE_STORAGE_LOCATION", "./uploads"),
		NotificationsEnabled:         envBool("NOTIFICATIONS_ENABLED", true),
		FirebaseCredentialsPath:      envOr("FIREBASE_CREDENTIALS_PATH", "./secrets/loobby-fcm.json"),
		NotificationSchedulerEnabled: envBool("NOTIFICATIONS_SCHEDULER_ENABLED", true),
		AllowUnsignedAnonRefresh:     envBool("ALLOW_UNSIGNED_ANON_REFRESH", false),
	}

	var missing []string
	if c.DatabaseURL == "" {
		missing = append(missing, "DB_URL")
	}
	if c.JWTSecret == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if c.RawgAPIKey == "" {
		missing = append(missing, "RAWG_API_KEY")
	}
	if c.ResendAPIKey == "" {
		missing = append(missing, "RESEND_API_KEY")
	}
	for i, name := range []string{"GOOGLE_WEB_CLIENT_ID", "GOOGLE_IOS_CLIENT_ID", "GOOGLE_ANDROID_CLIENT_ID"} {
		if c.GoogleClientIDs[i] == "" {
			missing = append(missing, name)
		}
	}
	// O segredo curto demais derruba o Kotlin na partida (JwtConfig); aqui tambem.
	if c.JWTSecret != "" && len(c.JWTSecret) < 32 {
		return c, fmt.Errorf("JWT_SECRET curto demais para HS256: use pelo menos 32 bytes")
	}
	if len(missing) > 0 {
		return c, fmt.Errorf("variaveis de ambiente obrigatorias ausentes: %s", strings.Join(missing, ", "))
	}

	// TLS e tudo-ou-nada: com metade dos arquivos, o servidor subiria em HTTP
	// puro achando que estava protegido.
	tlsSet := 0
	for _, v := range []string{c.TLSCertFile, c.TLSKeyFile, c.TLSClientCAFile} {
		if v != "" {
			tlsSet++
		}
	}
	if tlsSet != 0 && tlsSet != 3 {
		return c, fmt.Errorf("TLS incompleto: defina TLS_CERT_FILE, TLS_KEY_FILE e TLS_CLIENT_CA_FILE juntos")
	}
	return c, nil
}

// normalizeDatabaseURL aceita tanto a URL JDBC que o lado Kotlin usa
// (jdbc:postgresql://host/db) quanto a forma que o pgx espera
// (postgres://user:pass@host/db). Assim a mesma DB_URL serve aos dois processos.
//
// Credenciais separadas em DB_USERNAME/DB_PASSWORD (como no Kotlin) sao
// costuradas na URL quando ela nao as traz.
func normalizeDatabaseURL(raw string) string {
	if raw == "" {
		return ""
	}
	url := strings.TrimPrefix(raw, "jdbc:")

	user, pass := os.Getenv("DB_USERNAME"), os.Getenv("DB_PASSWORD")
	if user != "" && !strings.Contains(url, "@") {
		if i := strings.Index(url, "://"); i >= 0 {
			scheme, rest := url[:i+3], url[i+3:]
			if pass != "" {
				url = scheme + user + ":" + pass + "@" + rest
			} else {
				url = scheme + user + "@" + rest
			}
		}
	}
	return url
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

// ShutdownTimeout e quanto o servidor espera as requisicoes em voo terminarem.
const ShutdownTimeout = 10 * time.Second
