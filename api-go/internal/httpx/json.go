package httpx

import (
	"bytes"
	"encoding/json"
)

// Marshal serializa como o Jackson: sem quebra de linha no fim e sem escapar
// HTML. As duas coisas divergem do encoding/json padrao do Go:
//
//   - json.Encoder.Encode acrescenta "\n" -- todo corpo JSON teria um byte a
//     mais que o golden ({"status":"UP"} vs {"status":"UP"}\n).
//   - json.Marshal escapa <, > e & -- "Tom & Jerry" viraria "Tom & Jerry".
//     JSON equivalente para um parser, mas byte a byte diferente do que o
//     Jackson emite, e a baseline de contrato compara bytes.
//
// Divergencia conhecida e aceita: o Go sempre escapa U+2028 e U+2029, que o
// Jackson emite crus. So aparece se alguem usar esses separadores num nome.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
