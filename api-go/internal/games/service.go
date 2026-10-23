package games

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/jackson"
)

var (
	// ErrSearchUnavailable vira 503: a busca nao tem cache local para cair.
	ErrSearchUnavailable = errors.New("catalogo de jogos indisponivel")
	// ErrNotFound vira 404: nem o cache nem o RAWG tem o jogo (ou o RAWG esta
	// fora e o jogo nunca foi cacheado -- o Kotlin tambem responde 404 ai).
	ErrNotFound = errors.New("jogo nao encontrado")
)

// Rawg e o que o servico precisa do RawgClient (substituivel em teste).
type Rawg interface {
	Search(ctx context.Context, query string, page int) ([]byte, error)
	Game(ctx context.Context, id string) ([]byte, error)
}

// Service e a porta do GameService do Kotlin.
type Service struct {
	rawg    Rawg
	store   Store
	softTTL time.Duration // 7 dias: serve do cache e atualiza em segundo plano
	hardTTL time.Duration // 30 dias: atualiza antes de responder
	now     func() time.Time

	search *searchCache

	mu       sync.Mutex
	inflight map[string]bool
	slots    chan struct{}
	wg       sync.WaitGroup // so para os testes esperarem os refreshes
}

func NewService(rawg Rawg, store Store) *Service {
	return &Service{
		rawg:     rawg,
		store:    store,
		softTTL:  7 * 24 * time.Hour,
		hardTTL:  30 * 24 * time.Hour,
		now:      time.Now,
		search:   newSearchCache(500, 10*time.Minute),
		inflight: map[string]bool{},
		slots:    make(chan struct{}, 4),
	}
}

// Search: proxy da busca do RAWG. Cada resultado e gravado no cache local, e a
// resposta inteira fica 10 minutos em memoria pela consulta normalizada.
//
// A chave do cache usa trim + minusculas, mas o RAWG recebe a consulta como
// veio -- igual ao @Cacheable do Kotlin. "Zelda" e "zelda " dividem a entrada,
// e quem chegou primeiro define a resposta.
func (s *Service) Search(ctx context.Context, query string, page int) (SearchResponse, error) {
	key := strings.ToLower(strings.TrimSpace(query)) + ":" + itoa(page)
	if resp, ok := s.search.get(key, s.now()); ok {
		return resp, nil
	}

	body, err := s.rawg.Search(ctx, query, page)
	if err != nil {
		slog.Warn("RAWG indisponivel na busca", "q", query, "page", page, "err", err)
		return SearchResponse{}, ErrSearchUnavailable
	}
	root, _ := parseNode(body)

	var items []json.RawMessage
	if root.kind("results") == '[' {
		_ = json.Unmarshal(root["results"], &items)
	}
	results := make([]Summary, 0, len(items))
	now := s.now()
	for _, item := range items {
		g, ok := parseGame(item)
		if !ok {
			continue
		}
		name, err := s.store.UpsertFromSearch(ctx, g, now)
		if err != nil {
			// como o runCatching do Kotlin: o item some da resposta, a busca segue
			slog.Warn("falha ao cachear item da busca", "id", g.ID, "err", err)
			continue
		}
		results = append(results, summaryOf(g, name))
	}

	resp := SearchResponse{Page: page, Count: root.asInt("count", len(results)), Results: results}
	s.search.put(key, resp, s.now())
	return resp, nil
}

// Details: read-through com TTL, exatamente na ordem de decisao do Kotlin.
func (s *Service) Details(ctx context.Context, id string) (Details, error) {
	c, found, err := s.store.Find(ctx, id)
	if err != nil {
		return Details{}, err
	}
	if found {
		age := s.now().Sub(c.RefreshedAt)
		switch {
		case c.DescriptionRaw == nil:
			// a tela de detalhe precisa da descricao: busca o completo agora
			return s.refreshOr(ctx, id, &c)
		case age <= s.softTTL:
			return detailsOf(c), nil
		case age <= s.hardTTL:
			s.refreshInBackground(id) // stale-while-revalidate
			return detailsOf(c), nil
		default:
			return s.refreshOr(ctx, id, &c)
		}
	}
	return s.refreshOr(ctx, id, nil)
}

// refreshOr busca no RAWG; se nao conseguir, devolve o cache (vencido ou sem
// descricao), ou ErrNotFound se nao houver cache.
func (s *Service) refreshOr(ctx context.Context, id string, fallback *cached) (Details, error) {
	d, ok, err := s.refresh(ctx, id)
	if err != nil {
		return Details{}, err // erro de banco: o Kotlin tambem deixa estourar (500)
	}
	if ok {
		return d, nil
	}
	if fallback != nil {
		return detailsOf(*fallback), nil
	}
	return Details{}, ErrNotFound
}

// refresh busca o detalhe completo e grava. ok=false quando o RAWG esta fora ou
// nao tem o jogo; err so para falha de banco.
//
// A resposta e montada com o que veio do RAWG, NAO relida do banco: e isso que
// faz o rating sair "0.0" aqui e "0.00" quando a proxima leitura vier do
// cache. Medido em producao (jogos 257, 64911, 21482): mesma rota, mesma hora,
// so a origem do dado muda.
func (s *Service) refresh(ctx context.Context, id string) (Details, bool, error) {
	body, err := s.rawg.Game(ctx, id)
	if err != nil {
		slog.Warn("RAWG indisponivel ao buscar jogo", "id", id, "err", err)
		return Details{}, false, nil
	}
	if body == nil {
		return Details{}, false, nil
	}
	g, ok := parseGame(body)
	if !ok {
		return Details{}, false, nil
	}
	desc := descriptionFromDetail(body)
	name, err := s.store.UpsertFromDetail(ctx, g, desc, s.now())
	if err != nil {
		return Details{}, false, err
	}
	return detailsFromRawg(g, name, desc), true, nil
}

// refreshInBackground: no maximo 4 ao mesmo tempo, e um por jogo. Se nao houver
// vaga, desiste -- a proxima leitura vencida tenta de novo. (O executor do
// Kotlin enfileira sem limite; aqui a maquina tem 1 GB.)
func (s *Service) refreshInBackground(id string) {
	s.mu.Lock()
	if s.inflight[id] {
		s.mu.Unlock()
		return
	}
	select {
	case s.slots <- struct{}{}:
	default:
		s.mu.Unlock()
		return
	}
	s.inflight[id] = true
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			s.mu.Lock()
			delete(s.inflight, id)
			s.mu.Unlock()
			<-s.slots
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, _, err := s.refresh(ctx, id); err != nil {
			slog.Warn("refresh em segundo plano falhou", "id", id, "err", err)
		}
	}()
}

func summaryOf(g game, name string) Summary {
	return Summary{
		ID: g.ID, Slug: g.Slug, Name: name, BackgroundImage: g.BackgroundImage,
		Released: dateString(g.Released), Rating: numberOf(g.Rating), Metacritic: g.Metacritic,
	}
}

func detailsFromRawg(g game, name string, desc *string) Details {
	return Details{
		ID: g.ID, Slug: g.Slug, Name: name, BackgroundImage: g.BackgroundImage,
		Released: dateString(g.Released), Rating: numberOf(g.Rating), Metacritic: g.Metacritic,
		DescriptionRaw: desc, Genres: rawJSON(g.Genres), Platforms: rawJSON(g.Platforms),
	}
}

func detailsOf(c cached) Details {
	return Details{
		ID: c.ID, Slug: c.Slug, Name: c.Name, BackgroundImage: c.BackgroundImage,
		Released: dateString(c.Released), Rating: numberOf(c.Rating), Metacritic: c.Metacritic,
		DescriptionRaw: c.DescriptionRaw, Genres: renormalize(c.Genres), Platforms: renormalize(c.Platforms),
	}
}

func rawJSON(s *string) json.RawMessage {
	if s == nil {
		return nil
	}
	return json.RawMessage(*s)
}

// renormalize: o jsonb chega como o Postgres escreve ({"id": 1, "slug": "a"});
// o Kotlin o le pelo Jackson e re-escreve compacto, na mesma ordem. JSON
// invalido vira null, como o runCatching do parseJson.
func renormalize(b []byte) json.RawMessage {
	if b == nil {
		return nil
	}
	out, err := jackson.Normalize(b)
	if err != nil {
		return nil
	}
	return out
}

func dateString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02") // LocalDate.toString()
	return &s
}
