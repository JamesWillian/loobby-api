// Package platform: as 5 plataformas da 2.0 -- as da gameplay e as do "tenho o
// jogo" da biblioteca -- e a traducao das plataformas do RAWG para elas.
package platform

import (
	"encoding/json"
	"slices"
	"strings"
)

const (
	PC          = "PC"
	PlayStation = "PLAYSTATION"
	Xbox        = "XBOX"
	Switch      = "SWITCH"
	Mobile      = "MOBILE"
)

// All: as 5, na ordem em que o app mostra e em que as listas saem.
var All = []string{PC, PlayStation, Xbox, Switch, Mobile}

// Invalid e a mensagem do fieldError de uma plataforma fora das 5.
const Invalid = "Plataforma inválida: use PC, PLAYSTATION, XBOX, SWITCH ou MOBILE"

func Valid(p string) bool { return slices.Contains(All, p) }

// Encode devolve a lista como texto JSON, que e como o jsonb entra no SQL:
// (sqlc.arg(x)::text)::jsonb, por causa do simple_protocol da producao.
func Encode(list []string) string {
	if list == nil {
		list = []string{}
	}
	b, _ := json.Marshal(list) // lista de strings: nao falha
	return string(b)
}

// Decode le uma lista gravada. O que nao for lista vira lista vazia.
func Decode(raw []byte) []string {
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil || list == nil {
		return []string{}
	}
	return list
}

// FromRAWG: as plataformas em que o jogo existe, a partir de games.platforms,
// a lista do RAWG ([{"platform": {"slug": "pc", ...}}, ...]). Sai na ordem de
// All, sem repetir. Sem lista, ou so com plataformas fora das 5 (as retro), a
// lista vem vazia, e o app oferece as 5.
func FromRAWG(raw []byte) []string {
	var entries []struct {
		Platform struct {
			Slug string `json:"slug"`
		} `json:"platform"`
	}
	found := map[string]bool{}
	if json.Unmarshal(raw, &entries) == nil {
		for _, e := range entries {
			if p := ofSlug(e.Platform.Slug); p != "" {
				found[p] = true
			}
		}
	}
	out := []string{}
	for _, p := range All {
		if found[p] {
			out = append(out, p)
		}
	}
	return out
}

// ofSlug: a plataforma da 2.0 de um slug do RAWG, ou "".
func ofSlug(slug string) string {
	switch {
	case slug == "pc" || slug == "macos" || slug == "linux":
		return PC
	case strings.HasPrefix(slug, "playstation") || slug == "ps-vita" || slug == "psp":
		return PlayStation
	case strings.HasPrefix(slug, "xbox"):
		return Xbox
	case strings.HasPrefix(slug, "nintendo-switch"):
		return Switch
	case slug == "ios" || slug == "android":
		return Mobile
	}
	return ""
}
