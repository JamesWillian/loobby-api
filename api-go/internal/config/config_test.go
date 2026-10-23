package config

import "testing"

func base(t *testing.T) {
	t.Helper()
	t.Setenv("DB_URL", "postgres://u:p@h/db")
	t.Setenv("JWT_SECRET", "s")
	t.Setenv("RAWG_API_KEY", "k")
	for _, k := range []string{"TLS_CERT_FILE", "TLS_KEY_FILE", "TLS_CLIENT_CA_FILE", "BIND_HOST", "PORT"} {
		t.Setenv(k, "")
	}
}

// HTTP sem TLS nunca pode ficar exposto na rede: sem TLS, so loopback.
func TestSemTLSEscutaSoEmLoopback(t *testing.T) {
	base(t)
	t.Setenv("PORT", "8081")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := c.ListenAddr(); got != "127.0.0.1:8081" {
		t.Errorf("sem TLS: %s, esperava 127.0.0.1:8081", got)
	}
}

func TestComTLSEscutaEmTodasAsInterfaces(t *testing.T) {
	base(t)
	t.Setenv("PORT", "8443")
	t.Setenv("TLS_CERT_FILE", "a")
	t.Setenv("TLS_KEY_FILE", "b")
	t.Setenv("TLS_CLIENT_CA_FILE", "c")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := c.ListenAddr(); got != ":8443" {
		t.Errorf("com TLS: %s, esperava :8443", got)
	}
}

// Metade da configuracao de TLS subiria o servidor em HTTP puro achando que
// estava protegido. Tem que falhar na partida.
func TestTLSIncompletoFalhaNaPartida(t *testing.T) {
	base(t)
	t.Setenv("TLS_CERT_FILE", "a")
	t.Setenv("TLS_KEY_FILE", "b")
	if _, err := Load(); err == nil {
		t.Error("TLS com 2 de 3 arquivos deveria falhar")
	}
}

func TestDBURLJDBCComCredenciaisSeparadas(t *testing.T) {
	base(t)
	t.Setenv("DB_URL", "jdbc:postgresql://db.exemplo.com:5432/postgres")
	t.Setenv("DB_USERNAME", "usuario")
	t.Setenv("DB_PASSWORD", "senha")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if want := "postgresql://usuario:senha@db.exemplo.com:5432/postgres"; c.DatabaseURL != want {
		t.Errorf("DB_URL normalizada: %s", c.DatabaseURL)
	}
}
