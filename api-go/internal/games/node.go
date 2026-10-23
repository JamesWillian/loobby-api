package games

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/jackson"
)

// node da acesso a um objeto JSON do RAWG com a semantica do JsonNode, que e o
// que o GameService do Kotlin usa: path().isTextual, isNumber, isInt, asText.
// A diferenca importa -- "rating":"4.5" (texto) nao e numero, e o Kotlin grava
// null; "metacritic":85.0 e numero mas nao isInt, e tambem vira null.
type node map[string]json.RawMessage

func parseNode(raw []byte) (node, bool) {
	var n node
	if err := json.Unmarshal(raw, &n); err != nil {
		return nil, false
	}
	return n, true
}

// kind: '"' texto, '0' numero, 't'/'f' booleano, 'n' null, '{' objeto, '['
// lista; 0 se a chave nao existe (MissingNode).
func (n node) kind(key string) byte {
	raw, ok := n[key]
	if !ok {
		return 0
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return 0
	}
	switch c := raw[0]; {
	case c == '-' || (c >= '0' && c <= '9'):
		return '0'
	default:
		return c
	}
}

// text: path(key).isTextual ? asText : nao.
func (n node) text(key string) (string, bool) {
	if n.kind(key) != '"' {
		return "", false
	}
	var s string
	if err := json.Unmarshal(n[key], &s); err != nil {
		return "", false
	}
	return s, true
}

func (n node) textPtr(key string) *string {
	if s, ok := n.text(key); ok {
		return &s
	}
	return nil
}

// number devolve o literal se path(key).isNumber.
func (n node) number(key string) (string, bool) {
	if n.kind(key) != '0' {
		return "", false
	}
	return string(bytes.TrimSpace(n[key])), true
}

// present: node.get(key) existe e nao e JSON null. E o teste que o Kotlin faz
// antes de gravar genres/platforms.
func (n node) present(key string) (json.RawMessage, bool) {
	k := n.kind(key)
	if k == 0 || k == 'n' {
		return nil, false
	}
	return n[key], true
}

// asText reproduz JsonNode.asText(): numero vira texto (real pelo
// Double.toString), null vira "null", ausente e objeto/lista viram "".
func (n node) asText(key string) string {
	switch n.kind(key) {
	case '"':
		s, _ := n.text(key)
		return s
	case '0':
		lit, _ := n.number(key)
		if !strings.ContainsAny(lit, ".eE") { // inteiro de qualquer tamanho
			if d, err := jackson.DecimalFromLiteral(lit); err == nil {
				return d
			}
			return lit
		}
		f, err := strconv.ParseFloat(lit, 64)
		if err != nil {
			return lit
		}
		return jackson.FormatDouble(f)
	case 't':
		return "true"
	case 'f':
		return "false"
	case 'n':
		return "null"
	default:
		return ""
	}
}

// asInt reproduz JsonNode.asInt(default) -- usado no "count" da busca.
func (n node) asInt(key string, def int) int {
	switch n.kind(key) {
	case '0':
		lit, _ := n.number(key)
		if jackson.IsIntLiteral(lit) {
			v, _ := strconv.Atoi(lit)
			return v
		}
		f, err := strconv.ParseFloat(lit, 64)
		if err != nil {
			return def
		}
		return int(int32(f)) // (int) do Java: trunca
	case '"':
		s, _ := n.text(key)
		if v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 32); err == nil {
			return int(v)
		}
		return def
	case 't':
		return 1
	case 'f':
		return 0
	default:
		return def
	}
}

// game e o que o Kotlin extrai de um no do RAWG (applyCommonFields).
type game struct {
	ID              string
	Name            *string // nil: o RAWG nao mandou nome textual; mantem o atual
	Slug            *string
	BackgroundImage *string
	Released        *time.Time
	Rating          *string // texto de JsonNode.decimalValue(): "0.0", "4.38", "4"
	Metacritic      *int32
	Genres          *string // JSON re-escrito como o Jackson faria
	Platforms       *string
	RawPayload      string
}

func parseGame(raw []byte) (game, bool) {
	n, ok := parseNode(raw)
	if !ok {
		return game{}, false
	}
	g := game{
		ID:              n.asText("id"),
		Name:            n.textPtr("name"),
		Slug:            n.textPtr("slug"),
		BackgroundImage: n.textPtr("background_image"),
		RawPayload:      string(raw),
	}
	if s, ok := n.text("released"); ok {
		// LocalDate.parse: ISO estrito; data invalida vira null, sem erro
		if t, err := time.Parse("2006-01-02", s); err == nil {
			g.Released = &t
		}
	}
	if lit, ok := n.number("rating"); ok {
		if d, err := jackson.DecimalFromLiteral(lit); err == nil {
			g.Rating = &d
		}
	}
	if lit, ok := n.number("metacritic"); ok && jackson.IsIntLiteral(lit) {
		if v, err := strconv.ParseInt(lit, 10, 32); err == nil {
			m := int32(v)
			g.Metacritic = &m
		}
	}
	g.Genres = normalized(n, "genres")
	g.Platforms = normalized(n, "platforms")
	return g, true
}

func normalized(n node, key string) *string {
	raw, ok := n.present(key)
	if !ok {
		return nil
	}
	b, err := jackson.Normalize(raw)
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}

// descriptionFromDetail: description_raw se for texto; senao description; senao null.
func descriptionFromDetail(raw []byte) *string {
	n, ok := parseNode(raw)
	if !ok {
		return nil
	}
	if s := n.textPtr("description_raw"); s != nil {
		return s
	}
	return n.textPtr("description")
}
