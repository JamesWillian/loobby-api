package httpx

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Tudo aqui foi medido contra o Kotlin (Jackson com o modulo Kotlin): o que ele
// aceita, o Go aceita igual; o que ele recusa vira ErrBody, que e o 500
// "Unexpected error" do catch-all de la.

func read(body, contentType string) (*Object, error) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return ReadObject(req)
}

func object(t *testing.T, body string) *Object {
	t.Helper()
	o, err := read(body, "application/json")
	if err != nil {
		t.Fatalf("corpo %s deveria ser lido: %v", body, err)
	}
	return o
}

func TestReadObjectRecusaOQueNaoEObjetoJSON(t *testing.T) {
	cases := []struct{ name, body, contentType string }{
		{"sem content-type", `{}`, ""},
		{"texto puro", `{}`, "text/plain"},
		{"formulario", `a=1`, "application/x-www-form-urlencoded"},
		{"vazio", ``, "application/json"},
		{"malformado", `{"a":`, "application/json"},
		{"lista", `[1]`, "application/json"},
		{"null", `null`, "application/json"},
		{"texto json", `"x"`, "application/json"},
		{"maior que 1 MB", `{"a":"` + strings.Repeat("x", maxBody) + `"}`, "application/json"},
	}
	for _, c := range cases {
		if _, err := read(c.body, c.contentType); !errors.Is(err, ErrBody) {
			t.Errorf("%s: esperava ErrBody, veio %v", c.name, err)
		}
	}
}

func TestReadObjectAceitaComoOJackson(t *testing.T) {
	for _, ct := range []string{"application/json", "application/json; charset=utf-8", "application/vnd.api+json", "APPLICATION/JSON"} {
		if _, err := read(`{"a":1}`, ct); err != nil {
			t.Errorf("content-type %q deveria ser aceito: %v", ct, err)
		}
	}
	// o que vem depois do objeto e ignorado
	o := object(t, `{"nome":"a"} isto nao e json`)
	if s := o.String("nome"); s == nil || *s != "a" {
		t.Errorf("texto depois do objeto deveria ser ignorado")
	}
	// chave repetida: vale a ultima; a chave e sensivel a maiusculas
	o = object(t, `{"nome":"primeiro","nome":"ultimo","Outro":"x"}`)
	if s := o.String("nome"); s == nil || *s != "ultimo" {
		t.Errorf("chave repetida: esperava o ultimo valor, veio %v", s)
	}
	if o.String("outro") != nil {
		t.Errorf(`"Outro" nao pode preencher "outro"`)
	}
}

// ok=false: o campo invalida o corpo. want=nil com ok=true: campo ausente/nulo.
func TestConversaoDeEscalares(t *testing.T) {
	str := func(v string) *string { return &v }
	cases := []struct {
		name, body string
		got        func(o *Object) *string
		want       *string
		ok         bool
	}{
		{"string", `{"v":"x"}`, func(o *Object) *string { return o.String("v") }, str("x"), true},
		{"string de numero", `{"v":123}`, func(o *Object) *string { return o.String("v") }, str("123"), true},
		{"string de decimal guarda o literal", `{"v":1.50}`, func(o *Object) *string { return o.String("v") }, str("1.50"), true},
		{"string de booleano", `{"v":true}`, func(o *Object) *string { return o.String("v") }, str("true"), true},
		{"string null", `{"v":null}`, func(o *Object) *string { return o.String("v") }, nil, true},
		{"string ausente", `{}`, func(o *Object) *string { return o.String("v") }, nil, true},
		{"string de objeto", `{"v":{}}`, func(o *Object) *string { return o.String("v") }, nil, false},
		{"string de lista", `{"v":[]}`, func(o *Object) *string { return o.String("v") }, nil, false},

		{"decimal numero guarda a escala", `{"v":25.50}`, func(o *Object) *string { return o.Decimal("v") }, str("25.50"), true},
		{"decimal texto", `{"v":" 25.50 "}`, func(o *Object) *string { return o.Decimal("v") }, str("25.50"), true},
		{"decimal vazio e null", `{"v":""}`, func(o *Object) *string { return o.Decimal("v") }, nil, true},
		{"decimal texto invalido", `{"v":"dez"}`, func(o *Object) *string { return o.Decimal("v") }, nil, false},
		{"decimal booleano", `{"v":true}`, func(o *Object) *string { return o.Decimal("v") }, nil, false},

		{"enum pelo nome", `{"v":"SPORT"}`, func(o *Object) *string { return o.Enum("v", "GAMEPLAY", "SPORT", "PARTY") }, str("SPORT"), true},
		{"enum pelo indice", `{"v":2}`, func(o *Object) *string { return o.Enum("v", "GAMEPLAY", "SPORT", "PARTY") }, str("PARTY"), true},
		{"enum pelo indice em texto", `{"v":"0"}`, func(o *Object) *string { return o.Enum("v", "GAMEPLAY", "SPORT", "PARTY") }, str("GAMEPLAY"), true},
		{"enum em minusculas", `{"v":"sport"}`, func(o *Object) *string { return o.Enum("v", "GAMEPLAY", "SPORT", "PARTY") }, nil, false},
		{"enum vazio", `{"v":""}`, func(o *Object) *string { return o.Enum("v", "GAMEPLAY", "SPORT", "PARTY") }, nil, false},
		{"enum indice fora", `{"v":3}`, func(o *Object) *string { return o.Enum("v", "GAMEPLAY", "SPORT", "PARTY") }, nil, false},

		{"uuid canonico", `{"v":"54F29B67-cb8b-4435-8b72-43c17f2f4b8f"}`, func(o *Object) *string { return o.UUID("v") }, str("54f29b67-cb8b-4435-8b72-43c17f2f4b8f"), true},
		{"uuid curto: o Jackson e estrito, o caminho nao", `{"v":"1-1-1-1-1"}`, func(o *Object) *string { return o.UUID("v") }, nil, false},
		{"uuid vazio e null", `{"v":""}`, func(o *Object) *string { return o.UUID("v") }, nil, true},
		{"uuid numero", `{"v":1}`, func(o *Object) *string { return o.UUID("v") }, nil, false},
	}
	for _, c := range cases {
		o := object(t, c.body)
		got := c.got(o)
		if (o.Err() == nil) != c.ok {
			t.Errorf("%s: corpo valido=%v, esperava %v", c.name, o.Err() == nil, c.ok)
		}
		switch {
		case got == nil && c.want == nil:
		case got == nil || c.want == nil || *got != *c.want:
			t.Errorf("%s: veio %v, esperava %v", c.name, deref(got), deref(c.want))
		}
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func TestInt(t *testing.T) {
	cases := []struct {
		body string
		want int32
		null bool
		ok   bool
	}{
		{`{"v":90}`, 90, false, true},
		{`{"v":"90"}`, 90, false, true},
		{`{"v":" 90 "}`, 90, false, true},
		{`{"v":90.7}`, 90, false, true},   // real e truncado
		{`{"v":-90.7}`, -90, false, true}, // em direcao a zero
		{`{"v":""}`, 0, true, true},
		{`{"v":null}`, 0, true, true},
		{`{}`, 0, true, true},
		{`{"v":"noventa"}`, 0, true, false},
		{`{"v":"9.5"}`, 0, true, false},
		{`{"v":true}`, 0, true, false},
		{`{"v":2147483648}`, 0, true, false},
	}
	for _, c := range cases {
		o := object(t, c.body)
		got := o.Int("v")
		if (o.Err() == nil) != c.ok {
			t.Errorf("%s: corpo valido=%v, esperava %v", c.body, o.Err() == nil, c.ok)
		}
		if (got == nil) != c.null || (got != nil && *got != c.want) {
			t.Errorf("%s: veio %v, esperava %d (null=%v)", c.body, got, c.want, c.null)
		}
	}
}

func TestBool(t *testing.T) {
	cases := []struct {
		body string
		want bool
		null bool
		ok   bool
	}{
		{`{"v":true}`, true, false, true},
		{`{"v":false}`, false, false, true},
		{`{"v":"true"}`, true, false, true},
		{`{"v":"TRUE"}`, true, false, true},
		{`{"v":"False"}`, false, false, true},
		{`{"v":1}`, true, false, true},
		{`{"v":0}`, false, false, true},
		{`{"v":""}`, false, true, true},
		{`{"v":"sim"}`, false, true, false},
		{`{"v":"tRuE"}`, false, true, false},
		{`{"v":1.5}`, false, true, false},
		{`{"v":{}}`, false, true, false},
	}
	for _, c := range cases {
		o := object(t, c.body)
		got := o.Bool("v")
		if (o.Err() == nil) != c.ok {
			t.Errorf("%s: corpo valido=%v, esperava %v", c.body, o.Err() == nil, c.ok)
		}
		if (got == nil) != c.null || (got != nil && *got != c.want) {
			t.Errorf("%s: veio %v, esperava %v (null=%v)", c.body, got, c.want, c.null)
		}
	}
}

// Req e o parametro nao-nulo sem default do Kotlin: faltou, o corpo e invalido.
func TestReq(t *testing.T) {
	o := object(t, `{"a":"x","b":null}`)
	if got := Req(o, o.String("a")); got != "x" || o.Err() != nil {
		t.Fatalf("campo presente: %q, %v", got, o.Err())
	}
	if got := Req(o, o.String("b")); got != "" || !errors.Is(o.Err(), ErrBody) {
		t.Errorf("null num campo obrigatorio deveria invalidar o corpo")
	}
	o = object(t, `{}`)
	Req(o, o.Int("falta"))
	if !errors.Is(o.Err(), ErrBody) {
		t.Errorf("campo obrigatorio ausente deveria invalidar o corpo")
	}
}

// A primeira falha fica guardada: o handler le tudo e confere uma vez.
func TestErroDoCorpoEPersistente(t *testing.T) {
	o := object(t, `{"ruim":{},"bom":"x"}`)
	o.String("ruim")
	if s := o.String("bom"); s == nil || *s != "x" {
		t.Errorf("os campos seguintes continuam legiveis")
	}
	if !errors.Is(o.Err(), ErrBody) {
		t.Errorf("a falha do primeiro campo deveria continuar em Err()")
	}
}

func TestObjetosAninhados(t *testing.T) {
	o := object(t, `{"sport":{"arena":"Quadra"},"players":[{"userId":"a"},{"userId":"b"}],"vazio":null}`)
	if s := o.Object("sport"); s == nil || *s.String("arena") != "Quadra" {
		t.Errorf("objeto aninhado nao foi lido")
	}
	if o.Object("vazio") != nil || o.Object("ausente") != nil {
		t.Errorf("null e ausente sao nil")
	}
	items, present := o.Objects("players")
	if !present || len(items) != 2 || *items[1].String("userId") != "b" {
		t.Errorf("lista de objetos: %d itens, presente=%v", len(items), present)
	}
	if _, present := o.Objects("ausente"); present {
		t.Errorf("lista ausente nao e presente")
	}
	// a falha de um campo aninhado invalida o corpo inteiro
	o.Object("sport").Int("arena")
	if !errors.Is(o.Err(), ErrBody) {
		t.Errorf("erro num objeto aninhado deveria subir para a raiz")
	}
	o = object(t, `{"sport":"texto"}`)
	if o.Object("sport") != nil || !errors.Is(o.Err(), ErrBody) {
		t.Errorf("texto onde vai um objeto deveria invalidar o corpo")
	}
}

func TestInstant(t *testing.T) {
	want := time.Date(2026, 9, 15, 16, 14, 35, 0, time.UTC)
	aceitos := map[string]time.Time{
		`"2026-09-15T16:14:35Z"`:           want,
		`"2026-09-15t16:14:35z"`:           want,
		`"2026-09-15T13:14:35-03:00"`:      want,
		`"2026-09-15T16:14:35.123456789Z"`: want.Add(123456789),
		`"2026-09-15T16:14:35.5Z"`:         want.Add(500 * time.Millisecond),
		`1789488875`:                       want,
		`1789488875.5`:                     want.Add(500 * time.Millisecond),
		`"1789488875"`:                     want,
	}
	for lit, expected := range aceitos {
		o := object(t, `{"v":`+lit+`}`)
		got := o.Instant("v")
		if got == nil || o.Err() != nil {
			t.Errorf("%s deveria ser aceito (err=%v)", lit, o.Err())
			continue
		}
		if !got.Equal(expected) {
			t.Errorf("%s: veio %s, esperava %s", lit, got.UTC(), expected)
		}
	}
	for _, lit := range []string{
		`"2026-09-15T16:14Z"`,   // sem segundos
		`"2026-09-15T16:14:35"`, // sem fuso
		`"2026-09-15"`,          // so a data
		`"amanha"`, `true`, `{}`,
	} {
		o := object(t, `{"v":`+lit+`}`)
		if got := o.Instant("v"); got != nil || !errors.Is(o.Err(), ErrBody) {
			t.Errorf("%s deveria invalidar o corpo (veio %v, err=%v)", lit, got, o.Err())
		}
	}
	// vazio e null: so invalidam se o campo for obrigatorio
	for _, lit := range []string{`""`, `null`} {
		o := object(t, `{"v":`+lit+`}`)
		if got := o.Instant("v"); got != nil || o.Err() != nil {
			t.Errorf("%s deveria ser nil sem erro (veio %v, err=%v)", lit, got, o.Err())
		}
	}
}
