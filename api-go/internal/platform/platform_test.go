package platform

import (
	"slices"
	"testing"
)

func TestFromRAWG(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"sem lista", `null`, []string{}},
		{"vazio", ``, []string{}},
		{"lista vazia", `[]`, []string{}},
		{"nao e lista", `{"platform":{"slug":"pc"}}`, []string{}},
		{"so retro", `[{"platform":{"slug":"nes"}},{"platform":{"slug":"sega-genesis"}}]`, []string{}},
		{
			"busca do RAWG, fora de ordem e repetindo",
			`[{"platform":{"id":187,"name":"PlayStation 5","slug":"playstation5"}},
			  {"platform":{"id":7,"name":"Nintendo Switch","slug":"nintendo-switch"}},
			  {"platform":{"id":4,"name":"PC","slug":"pc"}},
			  {"platform":{"id":18,"name":"PlayStation 4","slug":"playstation4"}},
			  {"platform":{"id":186,"name":"Xbox Series S/X","slug":"xbox-series-x"}}]`,
			[]string{PC, PlayStation, Xbox, Switch},
		},
		{
			"detalhe do RAWG, com requisitos",
			`[{"platform":{"id":5,"slug":"macos"},"released_at":"2013-09-17","requirements":{"minimum":"..."}},
			  {"platform":{"id":21,"slug":"android"},"requirements":null}]`,
			[]string{PC, Mobile},
		},
		{"portateis da Sony", `[{"platform":{"slug":"ps-vita"}},{"platform":{"slug":"psp"}}]`, []string{PlayStation}},
		{"Xbox antigos", `[{"platform":{"slug":"xbox360"}},{"platform":{"slug":"xbox-old"}}]`, []string{Xbox}},
		{"iOS e Linux", `[{"platform":{"slug":"ios"}},{"platform":{"slug":"linux"}}]`, []string{PC, Mobile}},
		{"outros Nintendo nao sao Switch", `[{"platform":{"slug":"nintendo-3ds"}},{"platform":{"slug":"wii-u"}}]`, []string{}},
	}
	for _, c := range cases {
		if got := FromRAWG([]byte(c.raw)); !slices.Equal(got, c.want) {
			t.Errorf("%s: queria %v, veio %v", c.name, c.want, got)
		}
	}
}

func TestEncodeDecode(t *testing.T) {
	if got := Encode(nil); got != `[]` {
		t.Errorf("Encode(nil) = %s", got)
	}
	if got := Encode([]string{PC, Switch}); got != `["PC","SWITCH"]` {
		t.Errorf("Encode = %s", got)
	}
	if got := Decode([]byte(`["XBOX"]`)); !slices.Equal(got, []string{Xbox}) {
		t.Errorf("Decode = %v", got)
	}
	if got := Decode([]byte(`null`)); got == nil || len(got) != 0 {
		t.Errorf("Decode(null) deveria ser lista vazia, veio %#v", got)
	}
}
