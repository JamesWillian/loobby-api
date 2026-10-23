package games

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"
)

// ErrUnavailable: RAWG fora do ar, respondendo erro, ou circuit breaker aberto.
var ErrUnavailable = errors.New("RAWG indisponivel")

// RawgConfig espelha o bloco `rawg:` do application.yaml do Kotlin.
type RawgConfig struct {
	APIKey           string
	BaseURL          string        // https://api.rawg.io/api
	ConnectTimeout   time.Duration // 3s
	ReadTimeout      time.Duration // 5s
	SearchPageSize   int           // 20
	FailureThreshold int32         // 5
	OpenFor          time.Duration // 2min
}

// RawgClient e a porta do RawgClient do Kotlin, com o mesmo circuit breaker
// em memoria: depois de FailureThreshold falhas seguidas, as chamadas sao
// puladas por OpenFor, sem tocar a rede.
//
// O que conta como falha tambem e o do Kotlin, inclusive um efeito colateral
// que vale saber: na BUSCA, qualquer resposta que nao seja 2xx conta -- ate o
// 404 que o RAWG devolve para uma pagina alem do fim. Cinco clientes paginando
// alem do fim abrem o circuito para todo mundo. No DETALHE, 404 e "jogo nao
// existe" e conta como sucesso.
type RawgClient struct {
	cfg  RawgConfig
	http *http.Client
	now  func() time.Time

	failures  atomic.Int32
	openUntil atomic.Int64 // UnixNano; 0 = fechado
}

func NewRawgClient(cfg RawgConfig) *RawgClient {
	return &RawgClient{
		cfg: cfg,
		now: time.Now,
		http: &http.Client{
			Transport: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: cfg.ConnectTimeout}).DialContext,
				TLSHandshakeTimeout:   cfg.ConnectTimeout,
				ResponseHeaderTimeout: cfg.ReadTimeout,
				MaxIdleConnsPerHost:   4,
			},
			Timeout: cfg.ConnectTimeout + 2*cfg.ReadTimeout,
		},
	}
}

func (c *RawgClient) ensureClosed() error {
	if until := c.openUntil.Load(); until != 0 && c.now().UnixNano() < until {
		return fmt.Errorf("%w: circuit breaker aberto ate %s", ErrUnavailable,
			time.Unix(0, until).UTC().Format(time.RFC3339))
	}
	return nil
}

func (c *RawgClient) recordSuccess() {
	c.failures.Store(0)
	c.openUntil.Store(0)
}

func (c *RawgClient) recordFailure() {
	if n := c.failures.Add(1); n >= c.cfg.FailureThreshold {
		until := c.now().Add(c.cfg.OpenFor)
		c.openUntil.Store(until.UnixNano())
		slog.Warn("RAWG circuit breaker aberto", "falhas_seguidas", n, "ate", until.UTC())
	}
}

// Search: GET /games?search=&page=&page_size=. Devolve o JSON cru da resposta.
func (c *RawgClient) Search(ctx context.Context, query string, page int) ([]byte, error) {
	if err := c.ensureClosed(); err != nil {
		return nil, err
	}
	q := url.Values{
		"key":       {c.cfg.APIKey},
		"search":    {query},
		"page":      {strconv.Itoa(page)},
		"page_size": {strconv.Itoa(c.cfg.SearchPageSize)},
	}
	status, body, err := c.get(ctx, c.cfg.BaseURL+"/games?"+q.Encode())
	if err != nil || status/100 != 2 {
		c.recordFailure()
		return nil, fmt.Errorf("%w: busca (status %d): %v", ErrUnavailable, status, err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		// O Kotlin lanca sem contar como falha do circuito; mantido igual.
		return nil, fmt.Errorf("%w: busca com corpo vazio", ErrUnavailable)
	}
	if !json.Valid(body) {
		c.recordFailure()
		return nil, fmt.Errorf("%w: busca com JSON invalido", ErrUnavailable)
	}
	c.recordSuccess()
	return body, nil
}

// Game: GET /games/{id}. Devolve nil, nil quando o RAWG responde 404.
func (c *RawgClient) Game(ctx context.Context, id string) ([]byte, error) {
	if err := c.ensureClosed(); err != nil {
		return nil, err
	}
	u := c.cfg.BaseURL + "/games/" + url.PathEscape(id) + "?" + url.Values{"key": {c.cfg.APIKey}}.Encode()
	status, body, err := c.get(ctx, u)
	switch {
	case err != nil:
		c.recordFailure()
		return nil, fmt.Errorf("%w: jogo %s: %v", ErrUnavailable, id, err)
	case status == http.StatusNotFound:
		c.recordSuccess() // 404 e resposta valida do RAWG, nao falha de infra
		return nil, nil
	case status/100 != 2:
		c.recordFailure()
		return nil, fmt.Errorf("%w: jogo %s: status %d", ErrUnavailable, id, status)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		c.recordSuccess() // o RestClient do Kotlin devolve null, tratado como "nao existe"
		return nil, nil
	}
	if !json.Valid(body) {
		c.recordFailure()
		return nil, fmt.Errorf("%w: jogo %s: JSON invalido", ErrUnavailable, id)
	}
	c.recordSuccess()
	return body, nil
}

// maxBody limita o que se le do RAWG: a maquina tem 1 GB. Um detalhe de jogo
// tem poucos KB.
const maxBody = 4 << 20

func (c *RawgClient) get(ctx context.Context, u string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Loobby-API")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, redactKey(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return resp.StatusCode, nil, redactKey(err)
	}
	return resp.StatusCode, body, nil
}

// redactKey: erros do net/http trazem a URL inteira, e a chave do RAWG vai na
// query string. Nao pode parar no log.
func redactKey(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		if parsed, perr := url.Parse(ue.URL); perr == nil {
			q := parsed.Query()
			if q.Has("key") {
				q.Set("key", "REDACTED")
				parsed.RawQuery = q.Encode()
				ue.URL = parsed.String()
			}
		}
	}
	return err
}
