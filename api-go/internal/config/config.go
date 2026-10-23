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
}

func Load() (Config, error) {
	c := Config{
		Port:                       envOr("PORT", "8080"),
		DatabaseURL:                normalizeDatabaseURL(os.Getenv("DB_URL")),
		JWTSecret:                  os.Getenv("JWT_SECRET"),
		AcceptRefreshTokenAsAccess: envBool("ACCEPT_REFRESH_AS_ACCESS", true),
		BaseURL:                    envOr("APP_BASE_URL", "https://api.loobby.app"),
	}

	var missing []string
	if c.DatabaseURL == "" {
		missing = append(missing, "DB_URL")
	}
	if c.JWTSecret == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if len(missing) > 0 {
		return c, fmt.Errorf("variaveis de ambiente obrigatorias ausentes: %s", strings.Join(missing, ", "))
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
