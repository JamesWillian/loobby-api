// Package legal serve os documentos legais publicos (privacidade, exclusao de
// conta, seguranca infantil). Porta do LegalController do Kotlin.
//
// Os HTML ficam embutidos no binario. Durante o strangler existem DUAS copias --
// esta e a de src/main/resources/legal -- porque o go:embed nao alcanca arquivos
// fora do modulo. legal_test.go falha se elas divergirem. Depois do cutover, a
// copia do Kotlin morre junto com o LegalController e esta passa a ser a unica.
package legal

import (
	"embed"
	"net/http"
	"strconv"
)

//go:embed static/*.html
var docs embed.FS

// Rotas do Kotlin: GET /legal/{privacy,account-deletion,child-safety}.
var slugs = []string{"privacy", "account-deletion", "child-safety"}

func Register(mux *http.ServeMux) {
	for _, slug := range slugs {
		body, err := docs.ReadFile("static/" + slug + ".html")
		if err != nil {
			// inalcancavel: o go:embed falha a compilacao se o arquivo faltar
			panic("legal: " + err.Error())
		}
		// O padrao "GET" do ServeMux tambem atende HEAD, como o Spring.
		mux.HandleFunc("GET /legal/"+slug, serve(body))
	}
}

func serve(body []byte) http.HandlerFunc {
	length := strconv.Itoa(len(body))
	return func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// Literal do Kotlin: sem espaco depois do ";" e com UTF-8 maiusculo --
		// nao o "text/html; charset=utf-8" que o net/http farejaria sozinho.
		h.Set("Content-Type", "text/html;charset=UTF-8")
		// CacheControl.maxAge(1, HOURS).cachePublic(). Como o handler define
		// cache, o SpringHeaders nao acrescenta o trio no-cache -- igual ao Spring.
		h.Set("Cache-Control", "max-age=3600, public")
		h.Set("Content-Length", length)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}
