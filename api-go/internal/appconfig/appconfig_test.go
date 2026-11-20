package appconfig

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// Current e editado a mao no dia de mudar o aviso ou a versao minima. Um erro
// aqui so apareceria no app, sem mensagem nenhuma: o app ignora o que nao
// entende e segue aberto.
func TestCurrentValido(t *testing.T) {
	for name, p := range map[string]Platform{"android": Current.Android, "ios": Current.IOS} {
		if p.MinVersion < 0 {
			t.Errorf("%s: MinVersion negativo", name)
		}
		if p.StoreURL != nil && !strings.HasPrefix(*p.StoreURL, "https://") {
			t.Errorf("%s: StoreURL precisa ser https: %q", name, *p.StoreURL)
		}
		if p.MinVersion > 0 && p.StoreURL == nil {
			t.Errorf("%s: versao bloqueada sem StoreURL deixa a pessoa sem saida", name)
		}
		if u := p.Update; u != nil && (strings.TrimSpace(u.Title) == "" || strings.TrimSpace(u.Message) == "") {
			t.Errorf("%s: Update precisa de titulo e texto", name)
		}
		n := p.Notice
		if n == nil {
			continue
		}
		if strings.TrimSpace(n.ID) == "" || strings.TrimSpace(n.Title) == "" || strings.TrimSpace(n.Message) == "" {
			t.Errorf("%s: Notice precisa de ID, titulo e texto", name)
		}
		if n.MaxVersion < 0 {
			t.Errorf("%s: Notice.MaxVersion negativo", name)
		}
		// O esquecimento tipico do dia da 2.0: subir a versao minima e deixar o
		// aviso da 1.x, que nenhuma versao liberada chega a ver.
		if n.MaxVersion > 0 && p.MinVersion > n.MaxVersion {
			t.Errorf("%s: aviso ate a versao %d, mas a minima e %d -- ninguem o ve; remova-o",
				name, n.MaxVersion, p.MinVersion)
		}
	}
}

func TestResposta(t *testing.T) {
	cfg := Config{
		Android: Platform{
			MinVersion: 8,
			StoreURL:   ptr("https://play.google.com/store/apps/details?id=app.loobby"),
			Update:     &Message{Title: "Chegou a 2.0", Message: "Atualize & jogue"},
			Notice:     &Notice{ID: "n1", Title: "Aviso", Message: "<texto>", MaxVersion: 7},
		},
	}
	rec := httptest.NewRecorder()
	handler(cfg)(rec, httptest.NewRequest(http.MethodGet, "/app/config", nil))

	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status %d, content-type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	body := rec.Body.Bytes()
	// o formato do resto da API: sem \n no fim e sem escapar & < >
	if bytes.HasSuffix(body, []byte("\n")) || !bytes.Contains(body, []byte(`"Atualize & jogue"`)) ||
		!bytes.Contains(body, []byte(`"<texto>"`)) {
		t.Errorf("JSON fora do formato da API: %s", body)
	}
	if rec.Header().Get("Content-Length") != strconv.Itoa(len(body)) {
		t.Errorf("Content-Length %q para %d bytes", rec.Header().Get("Content-Length"), len(body))
	}

	// O app le estas chaves; campo vazio sai como null, nunca some.
	const want = `{"android":{"minVersion":8,` +
		`"storeUrl":"https://play.google.com/store/apps/details?id=app.loobby",` +
		`"update":{"title":"Chegou a 2.0","message":"Atualize & jogue"},` +
		`"notice":{"id":"n1","title":"Aviso","message":"<texto>","maxVersion":7}},` +
		`"ios":{"minVersion":0,"storeUrl":null,"update":null,"notice":null}}`
	if string(body) != want {
		t.Errorf("corpo:\n%s\nesperado:\n%s", body, want)
	}

	var back Config
	if err := json.Unmarshal(body, &back); err != nil || !reflect.DeepEqual(back, cfg) {
		t.Errorf("ida e volta: %v %+v", err, back)
	}
}
