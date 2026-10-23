// Package jackson reproduz, byte a byte, como o Jackson (lado Kotlin) escreve
// numeros e strings. A biblioteca padrao do Go escreve JSON valido e
// equivalente, mas nao IDENTICO -- e a baseline de contrato compara bytes.
//
// O caso que motivou o pacote: o mesmo GET /games/{id} devolve "rating":0.0
// quando o dado vem do RAWG na hora, e "rating":0.00 quando vem do cache no
// Postgres (medido em producao). O primeiro passa por double -> BigDecimal; o
// segundo, pela coluna numeric(4,2). Para reproduzir isso, o Go precisa saber
// exatamente o que o Java faz com cada numero.
package jackson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// FormatDouble reproduz java.lang.Double.toString (JDK 19+): os digitos mais
// curtos que identificam o double, sempre com ao menos uma casa decimal ("4.0",
// nao "4"), e notacao cientifica fora de [1e-3, 1e7) ("1.0E7", "1.234E-5").
//
// E o que o Jackson escreve para um DoubleNode -- por exemplo, um numero real
// dentro de genres/platforms.
func FormatDouble(f float64) string {
	if f == 0 {
		if math.Signbit(f) {
			return "-0.0"
		}
		return "0.0"
	}
	sign := ""
	if f < 0 {
		sign, f = "-", -f
	}
	digits, exp := javaDigits(f)
	if f >= 1e-3 && f < 1e7 {
		return sign + plain(digits, exp)
	}
	frac := digits[1:]
	if frac == "" {
		frac = "0"
	}
	return sign + digits[:1] + "." + frac + "E" + strconv.Itoa(exp)
}

// javaDigits devolve os digitos significativos e o expoente (valor =
// d1.d2d3... x 10^exp) que o Java escolhe. Coincide com o mais curto do Go,
// exceto numa regra do JDK: quando o mais curto tem UM digito, o Java considera
// tambem os de dois e fica com o mais proximo do valor real. Por isso
// Double.MIN_VALUE e "4.9E-324" no Java, e nao "5.0E-324".
func javaDigits(f float64) (string, int) {
	digits, exp := splitE(strconv.FormatFloat(f, 'e', -1, 64))
	if len(digits) == 1 {
		two := strconv.FormatFloat(f, 'e', 1, 64) // o de 2 digitos mais proximo
		if v, err := strconv.ParseFloat(two, 64); err == nil && v == f {
			d2, e2 := splitE(two)
			if d2 = strings.TrimRight(d2, "0"); d2 != digits || e2 != exp {
				return d2, e2
			}
		}
	}
	return digits, exp
}

// splitE: "4.938e-05" -> ("4938", -5)
func splitE(s string) (string, int) {
	mant, e, _ := strings.Cut(s, "e")
	exp, _ := strconv.Atoi(e)
	return strings.Replace(mant, ".", "", 1), exp
}

// plain escreve d1.d2... x 10^exp sem notacao cientifica, com ao menos uma casa.
func plain(digits string, exp int) string {
	if exp < 0 {
		return "0." + strings.Repeat("0", -exp-1) + digits
	}
	if len(digits) <= exp+1 {
		return digits + strings.Repeat("0", exp+1-len(digits)) + ".0"
	}
	return digits[:exp+1] + "." + digits[exp+1:]
}

// DecimalFromLiteral reproduz JsonNode.decimalValue().toString() para um
// numero JSON literal -- o caminho do "rating" que vem do RAWG:
//
//	inteiro ("4")  -> IntNode -> BigDecimal.valueOf(long)       -> "4"
//	real ("4.50")  -> DoubleNode 4.5 -> BigDecimal.valueOf(4.5) -> "4.5"
//
// Repare que o texto original nao sobrevive: "4.50" vira "4.5", porque o Java
// passa pelo double. Ja "0.0" continua "0.0", porque Double.toString(0.0) e
// "0.0".
func DecimalFromLiteral(lit string) (string, error) {
	if isIntegerLiteral(lit) {
		n, ok := new(big.Int).SetString(lit, 10)
		if !ok {
			return "", fmt.Errorf("inteiro invalido: %q", lit)
		}
		return n.String(), nil
	}
	f, err := strconv.ParseFloat(lit, 64)
	if err != nil {
		return "", fmt.Errorf("numero invalido: %q", lit)
	}
	return bigDecimalString(FormatDouble(f)), nil
}

// bigDecimalString aplica BigDecimal.toString() ao texto de Double.toString.
// Iguais na faixa plana; fora dela o BigDecimal tem regras proprias
// ("1.0E7" -> "1.0E+7", "1.234E-5" -> "0.00001234").
func bigDecimalString(s string) string {
	mant, expStr, hasExp := strings.Cut(s, "E")
	exp := 0
	if hasExp {
		exp, _ = strconv.Atoi(expStr)
	}
	neg := strings.HasPrefix(mant, "-")
	mant = strings.TrimPrefix(mant, "-")
	intPart, frac, _ := strings.Cut(mant, ".")
	digits := strings.TrimLeft(intPart+frac, "0")
	scale := len(frac) - exp
	if digits == "" { // zero: BigDecimal preserva a escala
		digits = "0"
	}
	sign := ""
	if neg && digits != "0" {
		sign = "-"
	}
	adjusted := len(digits) - 1 - scale
	if scale >= 0 && adjusted >= -6 {
		if scale == 0 {
			return sign + digits
		}
		if len(digits) <= scale {
			digits = strings.Repeat("0", scale-len(digits)+1) + digits
		}
		p := len(digits) - scale
		return sign + digits[:p] + "." + digits[p:]
	}
	out := digits[:1]
	if len(digits) > 1 {
		out += "." + digits[1:]
	}
	if adjusted >= 0 {
		return sign + out + "E+" + strconv.Itoa(adjusted)
	}
	return sign + out + "E" + strconv.Itoa(adjusted)
}

func isIntegerLiteral(lit string) bool {
	return !strings.ContainsAny(lit, ".eE")
}

// IsIntLiteral: o literal viraria IntNode no Jackson (inteiro que cabe em 32
// bits)? E o teste do JsonNode.isInt -- o Kotlin so aceita metacritic assim.
func IsIntLiteral(lit string) bool {
	if !isIntegerLiteral(lit) {
		return false
	}
	_, err := strconv.ParseInt(lit, 10, 32)
	return err == nil
}

// Normalize re-escreve um JSON como o Jackson faria depois de um readTree:
// compacto, chaves na ORDEM em que aparecem, numeros pela regra do JsonNode e
// strings com o escape do Jackson. Chave repetida fica na posicao da primeira
// ocorrencia com o valor da ultima, como no LinkedHashMap do ObjectNode.
//
// No lado Kotlin, genres/platforms passam por isso nos dois caminhos: do RAWG
// (ordem do RAWG) e do cache (ordem do jsonb, que o Postgres reordena por
// tamanho da chave). Preservar a ordem de entrada reproduz os dois.
func Normalize(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	v, err := readValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err == nil {
		return nil, errors.New("conteudo depois do valor JSON")
	}
	var buf bytes.Buffer
	writeValue(&buf, v)
	return buf.Bytes(), nil
}

type object struct {
	keys []string
	vals map[string]any
}

func readValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			o := &object{vals: map[string]any{}}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k := kt.(string)
				v, err := readValue(dec)
				if err != nil {
					return nil, err
				}
				if _, seen := o.vals[k]; !seen {
					o.keys = append(o.keys, k)
				}
				o.vals[k] = v
			}
			_, err := dec.Token() // '}'
			return o, err
		case '[':
			var arr []any
			for dec.More() {
				v, err := readValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			_, err := dec.Token() // ']'
			if arr == nil {
				arr = []any{}
			}
			return arr, err
		}
		return nil, fmt.Errorf("delimitador inesperado %v", t)
	default:
		return tok, nil // string, json.Number, bool, nil
	}
}

func writeValue(buf *bytes.Buffer, v any) {
	switch t := v.(type) {
	case *object:
		buf.WriteByte('{')
		for i, k := range t.keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			buf.WriteString(String(k))
			buf.WriteByte(':')
			writeValue(buf, t.vals[k])
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeValue(buf, e)
		}
		buf.WriteByte(']')
	case string:
		buf.WriteString(String(t))
	case json.Number:
		buf.WriteString(nodeNumber(string(t)))
	case bool:
		buf.WriteString(strconv.FormatBool(t))
	case nil:
		buf.WriteString("null")
	}
}

// nodeNumber: como o Jackson escreve o numero de um JsonNode. Inteiro sai como
// inteiro (de qualquer tamanho); real vira DoubleNode e sai por Double.toString.
func nodeNumber(lit string) string {
	if isIntegerLiteral(lit) {
		if n, ok := new(big.Int).SetString(lit, 10); ok {
			return n.String()
		}
		return lit
	}
	f, err := strconv.ParseFloat(lit, 64)
	if err != nil {
		return lit
	}
	return FormatDouble(f)
}

// String escreve uma string JSON com o escape do Jackson: aspas e barra
// invertida; \b \t \n \f \r na forma curta; demais controles (< 0x20) como
// \u00XX com hexa MAIUSCULO. Nada mais e escapado -- nem U+2028/U+2029, nem
// caracteres fora do ASCII.
func String(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			b.WriteString(`\"`)
		case c == '\\':
			b.WriteString(`\\`)
		case c == '\b':
			b.WriteString(`\b`)
		case c == '\t':
			b.WriteString(`\t`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\f':
			b.WriteString(`\f`)
		case c == '\r':
			b.WriteString(`\r`)
		case c < 0x20:
			b.WriteString(`\u00`)
			b.WriteByte(upperHex[c>>4])
			b.WriteByte(upperHex[c&0xF])
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

const upperHex = "0123456789ABCDEF"

// FixEscapes converte a saida do encoding/json para o escape do Jackson, sem
// tocar em mais nada. Duas diferencas, ambas so dentro de strings:
//
//   - o Go escreve U+2028 e U+2029 como \u2028/\u2029; o Jackson, crus;
//   - o Go escreve os demais controles como \u00xx minusculo; o Jackson, MAIUSCULO.
//
// Toda barra invertida na saida do encoding/json comeca uma sequencia de escape,
// entao percorrer de escape em escape e seguro: um "\\u2028" (barra literal
// seguida do texto u2028) e reconhecido como o escape \\ e fica intacto.
func FixEscapes(b []byte) []byte {
	if bytes.IndexByte(b, '\\') < 0 {
		return b
	}
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if b[i] != '\\' || i+1 >= len(b) {
			out = append(out, b[i])
			continue
		}
		if b[i+1] != 'u' || i+5 >= len(b) {
			out = append(out, b[i], b[i+1])
			i++
			continue
		}
		hex := string(b[i+2 : i+6])
		switch {
		case hex == "2028":
			out = append(out, "\u2028"...)
		case hex == "2029":
			out = append(out, "\u2029"...)
		case strings.HasPrefix(hex, "00"):
			out = append(out, '\\', 'u')
			out = append(out, strings.ToUpper(hex)...)
		default:
			out = append(out, b[i:i+6]...)
		}
		i += 5
	}
	return out
}
