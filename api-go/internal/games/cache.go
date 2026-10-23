package games

import (
	"strconv"
	"sync"
	"time"
)

// searchCache substitui o Caffeine da busca: 10 minutos a partir da escrita,
// ate 500 consultas. Ao lotar, descarta primeiro as vencidas e depois a mais
// antiga. (O Caffeine descarta por frequencia de uso; a diferenca nao aparece
// para o cliente, so muda qual consulta pode ir de novo ao RAWG.)
type searchCache struct {
	mu    sync.Mutex
	max   int
	ttl   time.Duration
	items map[string]searchEntry
	order []string
}

type searchEntry struct {
	resp    SearchResponse
	expires time.Time
}

func newSearchCache(max int, ttl time.Duration) *searchCache {
	return &searchCache{max: max, ttl: ttl, items: map[string]searchEntry{}}
}

func (c *searchCache) get(key string, now time.Time) (SearchResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok || !now.Before(e.expires) {
		return SearchResponse{}, false
	}
	return e.resp, true
}

func (c *searchCache) put(key string, resp SearchResponse, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.items[key]; !exists {
		if len(c.items) >= c.max {
			c.evict(now)
		}
		c.order = append(c.order, key)
	}
	c.items[key] = searchEntry{resp: resp, expires: now.Add(c.ttl)}
}

func (c *searchCache) evict(now time.Time) {
	kept := c.order[:0]
	for _, k := range c.order {
		if now.Before(c.items[k].expires) {
			kept = append(kept, k)
		} else {
			delete(c.items, k)
		}
	}
	c.order = kept
	for len(c.items) >= c.max && len(c.order) > 0 {
		delete(c.items, c.order[0])
		c.order = c.order[1:]
	}
}

func itoa(i int) string { return strconv.Itoa(i) }
