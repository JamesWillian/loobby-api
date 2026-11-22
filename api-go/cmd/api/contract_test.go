package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/config"
	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/dbtest"
	"github.com/JamesWillian/loobby-api/api-go/internal/server"
)

// TestContratoIgualAoKotlin sobe o servidor inteiro -- a mesma fiacao do main,
// num banco proprio -- e passa por ele a suite de contrato completa: os 73
// cenarios do capture.py e os fluxos felizes do flows.py (cadastro, verificacao
// de email, reset e troca de senha, exclusao de conta, login com Google). O
// resultado tem que ser indistinguivel de contract/golden-local, que e essa
// mesma suite gravada do Kotlin rodando localmente.
//
// E o teste que cobre os modulos de ponta a ponta: rota, filtro de
// autenticacao, leitura do corpo, regra, SQL, gatilhos do banco e o formato
// exato da resposta. Os testes de cada pacote cobrem o que a suite nao alcanca.
//
// Precisa de um Postgres (TEST_DATABASE_URL) e de python3, openssl e psql no
// PATH; sem eles, pula.
func TestContratoIgualAoKotlin(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL nao definida")
	}
	python, openssl, psql := tool(t, "python3"), tool(t, "openssl"), tool(t, "psql")
	root := repoRoot(t)
	work := t.TempDir()

	// 1. Um banco so deste teste, com as migracoes e o seed do ambiente local.
	// Os outros pacotes usam o banco de TEST_DATABASE_URL ao mesmo tempo.
	dbURL := dbtest.MigratedDatabase(t)
	sh(t, nil, psql, dbURL, "-q", "-v", "ON_ERROR_STOP=1", "-f", filepath.Join(root, "contract", "local", "seed.sql"))

	// 2. Resend, RAWG e Google falsos -- os mesmos usados para gravar a referencia.
	key, cert := filepath.Join(work, "google.key"), filepath.Join(work, "google.crt")
	sh(t, nil, openssl, "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "2",
		"-subj", "/CN=fake-google", "-keyout", key, "-out", cert)
	fakes := "http://" + freeAddr(t)
	fakesCmd := exec.Command(python, filepath.Join(root, "contract", "local", "fakes.py"),
		fakes[strings.LastIndexByte(fakes, ':')+1:], "--google-cert", cert)
	var fakesLog bytes.Buffer
	fakesCmd.Stdout, fakesCmd.Stderr = &fakesLog, &fakesLog
	if err := fakesCmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fakesCmd.Process.Kill(); _ = fakesCmd.Wait() })
	waitHTTP(t, fakes+"/emails", &fakesLog)

	// 3. O servidor, montado pelo mesmo build() que o main usa.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	srv := httptest.NewUnstartedServer(nil)
	t.Cleanup(srv.Close)
	base := "http://" + srv.Listener.Addr().String()
	const jwtSecret = "segredo-local-de-teste-com-mais-de-32-bytes-0123456789"
	deps, err := build(ctx, config.Config{
		JWTSecret:                  jwtSecret,
		AcceptRefreshTokenAsAccess: true,
		BaseURL:                    base,
		RawgAPIKey:                 "fake-rawg-key",
		RawgBaseURL:                fakes + "/rawg",
		ResendAPIKey:               "fake-resend-key",
		ResendBaseURL:              fakes + "/resend-go",
		GoogleClientIDs: []string{
			"web-local.apps.googleusercontent.com", "ios-local.apps.googleusercontent.com",
			"android-local.apps.googleusercontent.com",
		},
		GoogleCertsURL:       fakes + "/google/v3/certs",
		FileStorageLocation:  filepath.Join(work, "uploads"),
		NotificationsEnabled: false,
	}, pool)
	if err != nil {
		t.Fatal(err)
	}
	srv.Config.Handler = server.Handler(deps)
	srv.Start()

	// 4. A suite, como ela roda contra qualquer servidor.
	out := filepath.Join(work, "captura")
	envFile := filepath.Join(work, "env")
	env := "LOOBBY_BASE_URL=" + base + "\n" +
		"LOOBBY_TEST_EMAIL=user@loobby.app\nLOOBBY_TEST_PASSWORD=senha-local-123\n" +
		"LOOBBY_TEST2_EMAIL=contract-suite@loobby.app\nLOOBBY_TEST2_PASSWORD=senha-local-123\n" +
		"LOOBBY_TEST2_USER_ID=61d1ba14-2a43-4e31-a6b8-2ed31cadbcdf\n"
	if err := os.WriteFile(envFile, []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	contract := filepath.Join(root, "contract")
	script(t, python, filepath.Join(contract, "capture.py"),
		"--env-file", envFile, "--out", out, "--include-uploads")
	script(t, python, filepath.Join(contract, "flows.py"),
		"--base-url", base, "--out", out, "--mailbox", fakes, "--side", "go",
		"--google-key", key, "--psql", psql, "--db-url", dbURL, "--jwt-secret", jwtSecret)

	// 5. O criterio de aceite e o mesmo do strangler: compare.py, zero diferencas.
	cmp := exec.Command(python, filepath.Join(contract, "compare.py"), filepath.Join(contract, "golden-local"), out)
	if report, err := cmp.CombinedOutput(); err != nil {
		t.Fatalf("o Go divergiu do Kotlin:\n%s", report)
	}
}

func tool(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s nao encontrado no PATH", name)
	}
	return path
}

// repoRoot sobe ate a pasta que tem contract/golden-local.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for ; ; dir = filepath.Dir(dir) {
		if st, err := os.Stat(filepath.Join(dir, "contract", "golden-local")); err == nil && st.IsDir() {
			return dir
		}
		if filepath.Dir(dir) == dir {
			t.Skip("contract/golden-local nao encontrado")
		}
	}
}

func sh(t *testing.T, stdin []byte, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", filepath.Base(name), strings.Join(args, " "), err, tail(out))
	}
}

// script roda um dos scripts da suite com o ambiente limpo: as variaveis
// LOOBBY_* de quem chama (as de producao) nao podem vazar para ca.
func script(t *testing.T, python string, args ...string) {
	t.Helper()
	cmd := exec.Command(python, args...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", filepath.Base(args[0]), err, tail(out))
	}
	// Os scripts nunca falham por status inesperado -- eles gravam e avisam.
	// Aqui, um status que nao e o esperado e erro.
	if bytes.Contains(out, []byte("status fora do esperado")) || bytes.Contains(out, []byte("pulado")) {
		t.Errorf("%s:\n%s", filepath.Base(args[0]), tail(out))
	}
}

func tail(out []byte) []byte {
	if len(out) > 4000 {
		return out[len(out)-4000:]
	}
	return out
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func waitHTTP(t *testing.T, target string, log *bytes.Buffer) {
	t.Helper()
	for range 100 {
		if resp, err := http.Get(target); err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s nao respondeu:\n%s", target, log.String())
}
