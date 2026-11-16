package notifications

import (
	"sync"
	"time"
)

// debouncer agrupa as confirmacoes de presenca de um evento por destinatario:
// em vez de um push por pessoa que confirma, sai um so, alguns minutos depois
// da primeira confirmacao, com "Ana, Bia e mais 3 confirmaram presenca".
// Porta do PeerRsvpDebouncer.
//
// O estado fica em memoria, como no Kotlin: uma janela aberta se perde se o
// processo reiniciar antes de disparar.
type debouncer struct {
	delay time.Duration
	after func(time.Duration, func()) // time.AfterFunc, trocavel em teste

	mu      sync.Mutex
	windows map[windowKey]*window
}

type windowKey struct{ eventID, recipientID string }

type window struct {
	names  []string // os dois primeiros nomes
	extras int      // quantos confirmaram alem deles
	seen   map[string]bool
}

func newDebouncer(delay time.Duration) *debouncer {
	return &debouncer{
		delay:   delay,
		after:   func(d time.Duration, f func()) { time.AfterFunc(d, f) },
		windows: map[windowKey]*window{},
	}
}

// schedule registra que actorID confirmou. A primeira confirmacao abre a janela
// e agenda o disparo; as seguintes so acumulam. Quem confirma duas vezes na
// mesma janela conta uma vez.
func (d *debouncer) schedule(eventID, recipientID, actorID, actorName string, flush func(names []string, extras int)) {
	key := windowKey{eventID, recipientID}
	d.mu.Lock()
	defer d.mu.Unlock()
	if w, open := d.windows[key]; open {
		if !w.seen[actorID] {
			w.seen[actorID] = true
			if len(w.names) < 2 {
				w.names = append(w.names, actorName)
			} else {
				w.extras++
			}
		}
		return
	}
	d.windows[key] = &window{names: []string{actorName}, seen: map[string]bool{actorID: true}}
	d.after(d.delay, func() {
		d.mu.Lock()
		w := d.windows[key]
		delete(d.windows, key)
		d.mu.Unlock()
		if w != nil {
			flush(append([]string(nil), w.names...), w.extras)
		}
	})
}
