package jackson

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"strconv"
	"testing"
)

// oracle.jsonl foi gerado pelo Jackson 2.19.2 de verdade (testdata/JacksonOracle.java),
// a mesma versao do app Kotlin. Cada caso aqui e um fato medido, nao uma
// expectativa escrita a mao.
type oracleCase struct {
	Kind  string `json:"kind"`
	Bits  string `json:"bits"`
	Java  string `json:"java"`
	Node  string `json:"node"`
	Lit   string `json:"lit"`
	Out   string `json:"out"`
	IsInt bool   `json:"isInt"`
	In    string `json:"in"`
}

func oracle(t *testing.T) []oracleCase {
	t.Helper()
	f, err := os.Open("testdata/oracle.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []oracleCase
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var c oracleCase
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
	}
	if len(cases) < 3000 {
		t.Fatalf("gabarito incompleto: %d casos", len(cases))
	}
	return cases
}

func TestFormatDoubleIgualAoJava(t *testing.T) {
	n := 0
	for _, c := range oracle(t) {
		if c.Kind != "double" {
			continue
		}
		bits, err := strconv.ParseUint(c.Bits, 16, 64)
		if err != nil {
			t.Fatal(err)
		}
		d := math.Float64frombits(bits)
		if got := FormatDouble(d); got != c.Java {
			t.Errorf("Double.toString(%v): java=%s go=%s", d, c.Java, got)
		}
		// um DoubleNode dentro de genres/platforms sai exatamente assim
		if got := FormatDouble(d); got != c.Node {
			t.Errorf("DoubleNode(%v): jackson=%s go=%s", d, c.Node, got)
		}
		n++
	}
	t.Logf("%d doubles identicos ao Java", n)
}

func TestDecimalFromLiteralIgualAoJackson(t *testing.T) {
	for _, c := range oracle(t) {
		if c.Kind != "decimal" {
			continue
		}
		got, err := DecimalFromLiteral(c.Lit)
		if err != nil {
			t.Errorf("%s: %v", c.Lit, err)
			continue
		}
		if got != c.Out {
			t.Errorf("decimalValue(%s): jackson=%s go=%s", c.Lit, c.Out, got)
		}
		if IsIntLiteral(c.Lit) != c.IsInt {
			t.Errorf("isInt(%s): jackson=%v go=%v", c.Lit, c.IsInt, IsIntLiteral(c.Lit))
		}
	}
}

func TestNormalizeIgualAoJackson(t *testing.T) {
	for _, c := range oracle(t) {
		if c.Kind != "normalize" {
			continue
		}
		got, err := Normalize([]byte(c.In))
		if err != nil {
			t.Errorf("%s: %v", c.In, err)
			continue
		}
		if string(got) != c.Out {
			t.Errorf("readTree(%s)\n  jackson: %s\n  go     : %s", c.In, c.Out, got)
		}
	}
}

func TestStringIgualAoJackson(t *testing.T) {
	for _, c := range oracle(t) {
		if c.Kind != "string" {
			continue
		}
		if got := String(c.In); got != c.Out {
			t.Errorf("string %q\n  jackson: %q\n  go     : %q", c.In, c.Out, got)
		}
		// e o encoding/json + FixEscapes chega no mesmo lugar
		std, _ := json.Marshal(c.In)
		if got := string(FixEscapes(unescapeHTML(std))); got != c.Out {
			t.Errorf("FixEscapes(json.Marshal(%q))\n  jackson: %q\n  go     : %q", c.In, c.Out, got)
		}
	}
}

// json.Marshal escapa <, > e & -- o httpx.Marshal desliga isso com SetEscapeHTML.
func unescapeHTML(b []byte) []byte {
	var buf []byte
	for i := 0; i < len(b); i++ {
		if i+5 < len(b) && b[i] == '\\' && b[i+1] == 'u' {
			switch string(b[i+2 : i+6]) {
			case "003c":
				buf = append(buf, '<')
				i += 5
				continue
			case "003e":
				buf = append(buf, '>')
				i += 5
				continue
			case "0026":
				buf = append(buf, '&')
				i += 5
				continue
			}
		}
		buf = append(buf, b[i])
	}
	return buf
}

// Uma barra invertida literal seguida do texto "u2028" nao e o caractere
// U+2028 -- o FixEscapes nao pode confundir os dois.
func TestFixEscapesNaoConfundeBarraLiteral(t *testing.T) {
	std, _ := json.Marshal(`\u2028`) // a string tem 6 caracteres: \ u 2 0 2 8
	if got := string(FixEscapes(std)); got != `"\\u2028"` {
		t.Errorf("barra literal foi convertida: %s", got)
	}
}

func TestNormalizeRecusaJSONInvalido(t *testing.T) {
	for _, in := range []string{`{"a":`, `{} {}`, ``} {
		if _, err := Normalize([]byte(in)); err == nil {
			t.Errorf("%q deveria falhar", in)
		}
	}
}
