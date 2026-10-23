package games

import "encoding/json"

// Number e o texto exato de um numero JSON, ou null quando vazio. E como o
// rating sai: o texto ja vem pronto -- do RAWG pela regra do Jackson ("0.0") ou
// do Postgres pela escala da coluna ("0.00") -- e nao pode ser reformatado.
type Number string

func (n Number) MarshalJSON() ([]byte, error) {
	if n == "" {
		return []byte("null"), nil
	}
	return []byte(n), nil
}

func numberOf(s *string) Number {
	if s == nil {
		return ""
	}
	return Number(*s)
}

// Os campos seguem a ordem dos data classes do Kotlin, que e a ordem do JSON.

type Summary struct {
	ID              string  `json:"id"`
	Slug            *string `json:"slug"`
	Name            string  `json:"name"`
	BackgroundImage *string `json:"backgroundImage"`
	Released        *string `json:"released"`
	Rating          Number  `json:"rating"`
	Metacritic      *int32  `json:"metacritic"`
}

type SearchResponse struct {
	Page    int       `json:"page"`
	Count   int       `json:"count"`
	Results []Summary `json:"results"`
}

type Details struct {
	ID              string          `json:"id"`
	Slug            *string         `json:"slug"`
	Name            string          `json:"name"`
	BackgroundImage *string         `json:"backgroundImage"`
	Released        *string         `json:"released"`
	Rating          Number          `json:"rating"`
	Metacritic      *int32          `json:"metacritic"`
	DescriptionRaw  *string         `json:"descriptionRaw"`
	Genres          json.RawMessage `json:"genres"`
	Platforms       json.RawMessage `json:"platforms"`
}
