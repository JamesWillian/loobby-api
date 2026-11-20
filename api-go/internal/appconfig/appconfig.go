// Package appconfig serve GET /app/config: a versao minima de cada app e o
// aviso que aparece ao abrir. E a rota que deixa obrigar a atualizacao quando a
// 2.0 sair, e que avisa antes disso que os eventos vao acabar (a politica de
// privacidade, secao 11, promete aviso dentro do app antes de mudanca relevante).
//
// Primeira rota que nao veio do Kotlin: o formato e nosso, e o contrato da 1.3
// (contract/golden*) nao a cobre.
//
// A resposta e a mesma para todo mundo e nao depende de quem pergunta: o app
// compara a propria versao com a daqui. Nenhum dado do aparelho vem junto, e a
// rota e publica porque roda antes do login.
//
// Para mudar a versao minima ou o aviso, edite Current e publique a API
// (deploy/go/deploy.sh). O app nao precisa de versao nova.
package appconfig

import (
	"net/http"
	"strconv"

	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

// Config: o que cada plataforma recebe. O app le so a dele.
type Config struct {
	Android Platform `json:"android"`
	IOS     Platform `json:"ios"`
}

type Platform struct {
	// MinVersion: o menor numero de build que ainda abre o app -- o versionCode
	// no Android, o CFBundleVersion no iOS. Abaixo dele o app mostra so a tela de
	// atualizar. 0 = nenhuma versao bloqueada.
	MinVersion int `json:"minVersion"`
	// StoreURL: para onde o botao Atualizar leva. nil = a tela fica sem botao.
	StoreURL *string `json:"storeUrl"`
	// Update: titulo e texto da tela de atualizar. nil = o texto padrao do app.
	Update *Message `json:"update"`
	// Notice: aviso mostrado ao abrir o app, uma vez por ID. Trocar o ID faz o
	// aviso aparecer de novo para quem ja tinha fechado o anterior.
	Notice *Notice `json:"notice"`
}

type Message struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

type Notice struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Message string `json:"message"`
	// MaxVersion: o aviso so aparece ate este numero de build, inclusive.
	// 0 = em todas as versoes.
	MaxVersion int `json:"maxVersion"`
}

// Current e o que a rota responde.
//
// Na 1.3.1 (versionCode 7) ninguem e bloqueado e o aviso dos eventos vale so
// para a 1.x: a 2.0 nao deve mostra-lo nem se ele ficar aqui por esquecimento.
// Ao publicar uma 1.3.x nova, suba o MaxVersion junto.
//
// No dia da 2.0, depois que ela estiver disponivel na Play Store: MinVersion
// do Android vai para o versionCode dela, e o aviso sai (ou da lugar a outro).
// Antes disso, quem esta na 1.3.1 ficaria preso numa tela de atualizar sem nada
// para baixar.
//
// O iOS ainda nao foi publicado: sem loja, sem bloqueio e sem aviso.
var Current = Config{
	Android: Platform{
		MinVersion: 0,
		StoreURL:   ptr("https://play.google.com/store/apps/details?id=app.loobby"),
		Notice: &Notice{
			ID:    "eventos-descontinuados",
			Title: "O Loobby vai mudar",
			Message: "Em breve o Loobby vai ser só para jogos online, com votação para escolher " +
				"o jogo de cada gameplay. Os eventos, a formação de times e o controle de " +
				"pagamento serão descontinuados. Seus grupos continuam.",
			MaxVersion: 7,
		},
	},
}

// Register monta GET /app/config. O corpo e calculado uma vez, na partida.
func Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /app/config", handler(Current))
}

func handler(c Config) http.HandlerFunc {
	body, err := httpx.Marshal(c)
	if err != nil {
		// inalcancavel: so strings, inteiros e ponteiros para eles
		panic("appconfig: " + err.Error())
	}
	length := strconv.Itoa(len(body))
	return func(w http.ResponseWriter, _ *http.Request) {
		h := w.Header()
		h.Set("Content-Type", "application/json")
		h.Set("Content-Length", length)
		// Sem Cache-Control aqui: o httpx.SpringHeaders poe o no-cache de todas
		// as rotas, e uma versao minima nova vale na proxima abertura do app.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}

func ptr[T any](v T) *T { return &v }
