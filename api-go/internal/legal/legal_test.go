package legal

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Durante o strangler os HTML existem duas vezes -- aqui e em
// src/main/resources/legal -- porque o go:embed nao alcanca fora do modulo. Este
// teste falha se alguem editar uma copia e esquecer a outra: o Kotlin e o Go
// passariam a servir textos legais diferentes conforme a rota do nginx.
//
// Depois do cutover, o LegalController e a copia Kotlin devem ser apagados
// juntos; a partir dai o teste se pula e esta passa a ser a unica copia.
func TestCopiaIgualAoKotlin(t *testing.T) {
	for _, slug := range slugs {
		ours, err := docs.ReadFile("static/" + slug + ".html")
		if err != nil {
			t.Fatal(err)
		}
		kotlin := filepath.Join("..", "..", "..", "src", "main", "resources", "legal", slug+".html")
		theirs, err := os.ReadFile(kotlin)
		if os.IsNotExist(err) {
			t.Skip("copia Kotlin removida -- fim do strangler para legal")
		}
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(ours, theirs) {
			t.Errorf("%s.html divergiu entre Go e Kotlin -- edite os dois, ou remova o do Kotlin "+
				"se o cutover de legal ja aconteceu", slug)
		}
	}
}
