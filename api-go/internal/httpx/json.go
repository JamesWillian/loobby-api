package httpx

import (
	"bytes"
	"encoding/json"

	"github.com/JamesWillian/loobby-api/api-go/internal/jackson"
)

// Marshal serializa como o Jackson: sem quebra de linha no fim e sem escapar
// HTML. As duas coisas divergem do encoding/json padrao do Go:
//
//   - json.Encoder.Encode acrescenta "\n" -- todo corpo JSON teria um byte a
//     mais que o golden ({"status":"UP"} vs {"status":"UP"}\n).
//   - json.Marshal escapa <, > e & -- "Tom & Jerry" viraria "Tom \u0026 Jerry".
//     JSON equivalente para um parser, mas byte a byte diferente do que o
//     Jackson emite, e a baseline de contrato compara bytes.
//
// Strings tambem saem com o escape do Jackson (U+2028/U+2029 crus, controles
// em hexa maiusculo) -- ver jackson.FixEscapes. Antes disso, um displayname
// com U+2028 sairia diferente do Kotlin.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return jackson.FixEscapes(bytes.TrimSuffix(buf.Bytes(), []byte("\n"))), nil
}
