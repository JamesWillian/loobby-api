package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrBody: o corpo nao pode ser lido como o DTO esperado. No Kotlin isso e um
// HttpMessageNotReadableException (ou HttpMediaTypeNotSupportedException), que
// nenhum handler trata e por isso cai no catch-all: 500 "Unexpected error".
// Medido -- nao e 400. Entregar a Fail produz exatamente essa resposta.
var ErrBody = errors.New("corpo da requisicao ilegivel")

// maxBody limita o que o servidor aceita ler de um corpo JSON.
const maxBody = 1 << 20

// Object e um objeto JSON lido com as regras do Jackson (com o modulo Kotlin)
// em vez das do encoding/json. As diferencas que importam, todas medidas contra
// o Kotlin:
//
//   - chave e sensivel a maiusculas ("Name" nao preenche name);
//   - chave repetida: vale a ultima; chave desconhecida: ignorada;
//   - texto depois do objeto e ignorado ({"a":1} lixo);
//   - escalares sao convertidos entre si: 123 e true viram "123" e "true" num
//     campo String; "90" e 90.7 viram 90 num Int; "true" e 1 viram true;
//   - o que nao converte (objeto num String, "sim" num Boolean, "FOO" num
//     enum) invalida o corpo inteiro -> ErrBody.
//
// Os acessores nunca devolvem erro: a primeira falha fica guardada e aparece
// em Err(). O handler le todos os campos e confere uma vez so.
type Object struct {
	fields map[string]json.RawMessage
	err    *error
}

// ReadObject le o corpo como objeto JSON. Falha (ErrBody) quando o Content-Type
// nao e JSON, o corpo esta vazio ou malformado, ou a raiz nao e um objeto.
func ReadObject(r *http.Request) (*Object, error) {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !isJSONMediaType(mt) {
		return nil, ErrBody
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil || len(raw) > maxBody {
		return nil, ErrBody
	}
	var fields map[string]json.RawMessage
	// Decoder, e nao Unmarshal: le so o primeiro valor e ignora o resto.
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&fields); err != nil || fields == nil {
		return nil, ErrBody
	}
	var sticky error
	return &Object{fields: fields, err: &sticky}, nil
}

// application/json e application/*+json, como o conversor do Spring.
func isJSONMediaType(mt string) bool {
	return mt == "application/json" ||
		(strings.HasPrefix(mt, "application/") && strings.HasSuffix(mt, "+json"))
}

// Err devolve ErrBody se algum campo lido ate aqui era inconversivel.
func (o *Object) Err() error { return *o.err }

func (o *Object) fail() { *o.err = ErrBody }

// Has informa se a chave veio no corpo, mesmo com valor null. E a diferenca
// entre "ausente" (vale o default do data class) e "null explicito".
func (o *Object) Has(key string) bool {
	_, ok := o.fields[key]
	return ok
}

// value devolve o JSON cru do campo; ok=false para ausente ou null.
func (o *Object) value(key string) (json.RawMessage, bool) {
	v, ok := o.fields[key]
	if !ok {
		return nil, false
	}
	v = bytes.TrimSpace(v)
	if len(v) == 0 || bytes.Equal(v, []byte("null")) {
		return nil, false
	}
	return v, true
}

// Req e o parametro nao-nulo e sem default do Kotlin: ausente ou null invalida
// o corpo. Uso: name := httpx.Req(o, o.String("name")).
func Req[T any](o *Object, v *T) T {
	if v == nil {
		o.fail()
		var zero T
		return zero
	}
	return *v
}

func kind(v json.RawMessage) byte {
	switch c := v[0]; {
	case c == '"':
		return 's'
	case c == '{':
		return 'o'
	case c == '[':
		return 'a'
	case c == 't' || c == 'f':
		return 'b'
	default:
		return 'n'
	}
}

func text(v json.RawMessage) string {
	var s string
	_ = json.Unmarshal(v, &s)
	return s
}

// String: StringDeserializer. Numero e booleano entram pelo texto literal.
func (o *Object) String(key string) *string {
	v, ok := o.value(key)
	if !ok {
		return nil
	}
	var s string
	switch kind(v) {
	case 's':
		s = text(v)
	case 'n', 'b':
		s = string(v)
	default:
		o.fail()
		return nil
	}
	return &s
}

var intText = regexp.MustCompile(`^[+-]?[0-9]+$`)

// Int: Integer do Java. Real e truncado (ACCEPT_FLOAT_AS_INT), texto e aparado
// e lido como inteiro, "" vira null, booleano e recusado.
func (o *Object) Int(key string) *int32 {
	v, ok := o.value(key)
	if !ok {
		return nil
	}
	var n int64
	switch kind(v) {
	case 'n':
		lit := string(v)
		if !strings.ContainsAny(lit, ".eE") {
			i, err := strconv.ParseInt(lit, 10, 32)
			if err != nil {
				o.fail()
				return nil
			}
			n = i
		} else {
			f, err := strconv.ParseFloat(lit, 64)
			if err != nil || f <= -2147483649 || f >= 2147483648 {
				o.fail()
				return nil
			}
			n = int64(f) // trunca em direcao a zero, como o (int) do Java
		}
	case 's':
		s := strings.TrimSpace(text(v))
		if s == "" || s == "null" {
			return nil
		}
		if !intText.MatchString(s) {
			o.fail()
			return nil
		}
		i, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			o.fail()
			return nil
		}
		n = i
	default:
		o.fail()
		return nil
	}
	out := int32(n)
	return &out
}

// Bool: Boolean do Java. Aceita true/false, os textos "true"/"True"/"TRUE" e
// "false"/"False"/"FALSE" (so essas grafias), e inteiro (0 = false, resto =
// true). "" vira null.
func (o *Object) Bool(key string) *bool {
	v, ok := o.value(key)
	if !ok {
		return nil
	}
	var b bool
	switch kind(v) {
	case 'b':
		b = v[0] == 't'
	case 'n':
		lit := string(v)
		if strings.ContainsAny(lit, ".eE") {
			o.fail()
			return nil
		}
		b = lit != "0"
	case 's':
		switch s := strings.TrimSpace(text(v)); s {
		case "", "null":
			return nil
		case "true", "True", "TRUE":
			b = true
		case "false", "False", "FALSE":
			b = false
		default:
			o.fail()
			return nil
		}
	default:
		o.fail()
		return nil
	}
	return &b
}

var decimalText = regexp.MustCompile(`^[+-]?([0-9]+\.?[0-9]*|\.[0-9]+)([eE][+-]?[0-9]+)?$`)

// Decimal: BigDecimal do Java, devolvido como o TEXTO que entrou (so sem
// espacos nas pontas). O texto serve tanto de parametro numeric para o Postgres
// quanto de base para o BigDecimal.toString() -- "25.50" continua "25.50".
func (o *Object) Decimal(key string) *string {
	v, ok := o.value(key)
	if !ok {
		return nil
	}
	var s string
	switch kind(v) {
	case 'n':
		s = string(v)
	case 's':
		s = strings.TrimSpace(text(v))
		if s == "" || s == "null" {
			return nil
		}
		if !decimalText.MatchString(s) {
			o.fail()
			return nil
		}
	default:
		o.fail()
		return nil
	}
	return &s
}

// Enum: nome exato (sensivel a maiusculas) ou o INDICE da constante, como
// numero ou como texto de digitos -- o Jackson aceita os dois por padrao
// (medido: "eventType":2 cria um evento PARTY).
func (o *Object) Enum(key string, names ...string) *string {
	v, ok := o.value(key)
	if !ok {
		return nil
	}
	byIndex := func(lit string) *string {
		i, err := strconv.ParseInt(lit, 10, 32)
		if err != nil || i < 0 || int(i) >= len(names) {
			o.fail()
			return nil
		}
		return &names[i]
	}
	switch kind(v) {
	case 's':
		s := text(v)
		for i := range names {
			if names[i] == s {
				return &names[i]
			}
		}
		if t := strings.TrimSpace(s); t != "" && t[0] >= '0' && t[0] <= '9' {
			return byIndex(t)
		}
	case 'n':
		if lit := string(v); !strings.ContainsAny(lit, ".eE") {
			return byIndex(lit)
		}
	}
	o.fail()
	return nil
}

// UUID: UUIDDeserializer. So a forma canonica; "" vira null.
func (o *Object) UUID(key string) *string {
	v, ok := o.value(key)
	if !ok {
		return nil
	}
	if kind(v) != 's' {
		o.fail()
		return nil
	}
	s := strings.TrimSpace(text(v))
	if s == "" {
		return nil
	}
	id, ok := parseJSONUUID(s)
	if !ok {
		o.fail()
		return nil
	}
	return &id
}

// Object devolve um objeto aninhado; nil para ausente ou null.
func (o *Object) Object(key string) *Object {
	v, ok := o.value(key)
	if !ok {
		return nil
	}
	var fields map[string]json.RawMessage
	if kind(v) != 'o' || json.Unmarshal(v, &fields) != nil {
		o.fail()
		return nil
	}
	return &Object{fields: fields, err: o.err}
}

// Objects le uma lista de objetos. present=false quando a chave nao veio (vale
// o default do data class). Um null explicito numa List nao-nula invalida o
// corpo, assim como um elemento que nao seja objeto.
func (o *Object) Objects(key string) (items []*Object, present bool) {
	if !o.Has(key) {
		return nil, false
	}
	v, ok := o.value(key)
	var raws []json.RawMessage
	if !ok || kind(v) != 'a' || json.Unmarshal(v, &raws) != nil {
		o.fail()
		return nil, true
	}
	items = make([]*Object, 0, len(raws))
	for _, raw := range raws {
		var fields map[string]json.RawMessage
		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 || kind(raw) != 'o' || json.Unmarshal(raw, &fields) != nil {
			o.fail()
			return nil, true
		}
		items = append(items, &Object{fields: fields, err: o.err})
	}
	return items, true
}

// Instant: InstantDeserializer do jackson-datatype-jsr310. Formas aceitas,
// todas medidas:
//
//	"2030-01-01T10:00:00Z"            ISO-8601 com Z (t e z minusculos tambem)
//	"2030-01-01T10:00:00+02:00"       com deslocamento
//	"2030-01-01T10:00:00.123456789Z"  fracao de 0 a 9 digitos
//	1893456000 / "1893456000"         segundos desde 1970
//	1893456000.5                      segundos com fracao
//
// Recusadas (500): sem segundos ("10:00Z"), sem fuso, so a data. "" vira null.
func (o *Object) Instant(key string) *time.Time {
	v, ok := o.value(key)
	if !ok {
		return nil
	}
	var t time.Time
	switch kind(v) {
	case 'n':
		t, ok = epochInstant(string(v))
	case 's':
		s := strings.TrimSpace(text(v))
		if s == "" {
			return nil
		}
		if dots := countPeriods(s); dots == 0 || dots == 1 {
			t, ok = epochInstant(s)
		} else {
			ok = false
		}
		if !ok {
			t, ok = parseISOInstant(s)
		}
	default:
		ok = false
	}
	if !ok {
		o.fail()
		return nil
	}
	return &t
}

// countPeriods: -1 se o texto nao for um numero simples (sinal, digitos e
// pontos); senao, quantos pontos tem.
func countPeriods(s string) int {
	dots := 0
	for i, c := range s {
		switch {
		case i == 0 && (c == '-' || c == '+'):
		case c == '.':
			dots++
		case c < '0' || c > '9':
			return -1
		}
	}
	return dots
}

var billion = big.NewInt(1_000_000_000)

// Faixa em que o Instant cabe num timestamptz e sai formatado com ano de 4
// digitos: de 0001-01-01 a 9999-12-31.
const minEpoch, maxEpoch = -62135596800, 253402300799

func epochInstant(lit string) (time.Time, bool) {
	r, ok := new(big.Rat).SetString(lit)
	if !ok {
		return time.Time{}, false
	}
	// segundos = parte inteira (truncada); nanos = o resto, truncado.
	secs := new(big.Int).Quo(r.Num(), r.Denom())
	rest := new(big.Rat).Sub(r, new(big.Rat).SetInt(secs))
	nanos := new(big.Int).Quo(new(big.Int).Mul(rest.Num(), billion), rest.Denom())
	if !secs.IsInt64() || secs.Int64() < minEpoch || secs.Int64() > maxEpoch {
		return time.Time{}, false
	}
	return time.Unix(secs.Int64(), nanos.Int64()).UTC(), true
}

var isoInstant = regexp.MustCompile(
	`^([0-9]{4})-([0-9]{2})-([0-9]{2})[Tt]([0-9]{2}):([0-9]{2}):([0-9]{2})(?:\.([0-9]{0,9}))?` +
		`([Zz]|[+-][0-9]{2}:[0-9]{2}(?::[0-9]{2})?)$`)

var zeroOffset = regexp.MustCompile(`[+-]00:?00$`)

func parseISOInstant(s string) (time.Time, bool) {
	s = zeroOffset.ReplaceAllString(s, "Z")
	m := isoInstant.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, false
	}
	num := func(x string) int { n, _ := strconv.Atoi(x); return n }
	year, month, day := num(m[1]), num(m[2]), num(m[3])
	hour, min, sec := num(m[4]), num(m[5]), num(m[6])
	nanos := 0
	if m[7] != "" {
		nanos = num(m[7] + strings.Repeat("0", 9-len(m[7])))
	}
	addDays := 0
	switch {
	case hour == 24 && min == 0 && sec == 0 && nanos == 0:
		hour, addDays = 0, 1 // fim do dia, que o Java aceita
	case hour == 23 && min == 59 && sec == 60:
		sec = 59 // segundo bissexto
	}
	if year < 1 || month < 1 || month > 12 || day < 1 || hour > 23 || min > 59 || sec > 59 {
		return time.Time{}, false
	}
	offset := 0
	if z := m[8]; z != "Z" && z != "z" {
		oh, om, os := num(z[1:3]), num(z[4:6]), 0
		if len(z) > 6 {
			os = num(z[7:9])
		}
		offset = oh*3600 + om*60 + os
		if om > 59 || os > 59 || offset > 18*3600 {
			return time.Time{}, false
		}
		if z[0] == '-' {
			offset = -offset
		}
	}
	t := time.Date(year, time.Month(month), day, hour, min, sec, nanos, time.UTC)
	if t.Day() != day || int(t.Month()) != month { // 30 de fevereiro
		return time.Time{}, false
	}
	t = t.AddDate(0, 0, addDays).Add(-time.Duration(offset) * time.Second)
	if t.Unix() < minEpoch || t.Unix() > maxEpoch {
		return time.Time{}, false
	}
	return t, true
}
