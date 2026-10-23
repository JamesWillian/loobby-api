package games

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakeServer struct {
	*httptest.Server
	hits   atomic.Int32
	status atomic.Int32
	body   atomic.Value // string
	last   atomic.Value // *http.Request
}

func newFakeServer(t *testing.T) *fakeServer {
	f := &fakeServer{}
	f.status.Store(200)
	f.body.Store(`{"count":0,"results":[]}`)
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		f.last.Store(r)
		w.WriteHeader(int(f.status.Load()))
		_, _ = w.Write([]byte(f.body.Load().(string)))
	}))
	t.Cleanup(f.Close)
	return f
}

func newClient(base string, clock *time.Time) *RawgClient {
	c := NewRawgClient(RawgConfig{
		APIKey: "CHAVE-SECRETA-DO-RAWG", BaseURL: base, ConnectTimeout: time.Second,
		ReadTimeout: time.Second, SearchPageSize: 20, FailureThreshold: 5, OpenFor: 2 * time.Minute,
	})
	if clock != nil {
		c.now = func() time.Time { return *clock }
	}
	return c
}

func TestBuscaMandaOsParametrosDoKotlin(t *testing.T) {
	f := newFakeServer(t)
	c := newClient(f.URL, nil)
	if _, err := c.Search(context.Background(), "zelda breath", 2); err != nil {
		t.Fatal(err)
	}
	r := f.last.Load().(*http.Request)
	q := r.URL.Query()
	if r.URL.Path != "/games" || q.Get("search") != "zelda breath" || q.Get("page") != "2" ||
		q.Get("page_size") != "20" || q.Get("key") != "CHAVE-SECRETA-DO-RAWG" {
		t.Errorf("requisicao ao RAWG: %s", r.URL.String())
	}
}

// Depois de 5 falhas seguidas o circuito abre: a 6a chamada nao pode nem tocar
// a rede. Passados os 2 minutos, a proxima passa; um sucesso fecha de vez.
func TestCircuitoAbreFicaAbertoEFecha(t *testing.T) {
	f := newFakeServer(t)
	f.status.Store(500)
	clock := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	c := newClient(f.URL, &clock)

	for i := 0; i < 5; i++ {
		if _, err := c.Search(context.Background(), "x", 1); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("falha %d: esperava ErrUnavailable, veio %v", i+1, err)
		}
	}
	if _, err := c.Search(context.Background(), "x", 1); !errors.Is(err, ErrUnavailable) {
		t.Fatal("6a chamada deveria falhar")
	}
	if got := f.hits.Load(); got != 5 {
		t.Fatalf("com o circuito aberto o RAWG nao pode receber chamada: %d chamadas", got)
	}

	clock = clock.Add(2*time.Minute + time.Second)
	f.status.Store(200)
	if _, err := c.Search(context.Background(), "x", 1); err != nil {
		t.Fatalf("depois de 2 min deveria passar: %v", err)
	}
	if c.failures.Load() != 0 || c.openUntil.Load() != 0 {
		t.Error("um sucesso deveria fechar o circuito")
	}
}

// Medido no Kotlin: pagina alem do fim (404 do RAWG) conta como falha do
// circuito na busca, mas 404 no detalhe e "jogo nao existe" e conta como sucesso.
func TestDiferencaDo404EntreBuscaEDetalhe(t *testing.T) {
	f := newFakeServer(t)
	f.status.Store(404)
	c := newClient(f.URL, nil)

	if _, err := c.Search(context.Background(), "x", 999999); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("busca com 404: %v", err)
	}
	if c.failures.Load() != 1 {
		t.Errorf("busca com 404 deveria contar falha: %d", c.failures.Load())
	}
	body, err := c.Game(context.Background(), "nao-existe")
	if err != nil || body != nil {
		t.Fatalf("detalhe 404: body=%v err=%v", body, err)
	}
	if c.failures.Load() != 0 {
		t.Errorf("detalhe 404 e sucesso e zera o contador: %d", c.failures.Load())
	}
}

func TestJSONInvalidoEFalha(t *testing.T) {
	f := newFakeServer(t)
	f.body.Store(`<html>erro do proxy</html>`)
	c := newClient(f.URL, nil)
	if _, err := c.Game(context.Background(), "1"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("JSON invalido: %v", err)
	}
	if c.failures.Load() != 1 {
		t.Error("JSON invalido deveria contar falha")
	}
}

// O net/http poe a URL inteira na mensagem de erro, e a chave do RAWG vai na
// query string. Ela nao pode chegar ao log. (O Kotlin nao tem esse problema: o
// RestClient do Spring 6.2 corta a query string da URL antes de montar a
// mensagem da ResourceAccessException. O Go precisa fazer o corte por conta.)
func TestChaveDoRawgNaoVazaNoErro(t *testing.T) {
	c := newClient("http://127.0.0.1:1", nil) // porta fechada
	_, err := c.Search(context.Background(), "x", 1)
	if err == nil {
		t.Fatal("esperava erro de conexao")
	}
	if strings.Contains(err.Error(), "CHAVE-SECRETA-DO-RAWG") {
		t.Errorf("a chave vazou na mensagem: %v", err)
	}
}

func TestIdVaiEscapadoNoCaminho(t *testing.T) {
	f := newFakeServer(t)
	f.body.Store(`{"id":1}`)
	c := newClient(f.URL, nil)
	if _, err := c.Game(context.Background(), "a/b c"); err != nil {
		t.Fatal(err)
	}
	r := f.last.Load().(*http.Request)
	if got := r.URL.EscapedPath(); got != "/games/a%2Fb%20c" {
		t.Errorf("caminho: %s", got)
	}
}
