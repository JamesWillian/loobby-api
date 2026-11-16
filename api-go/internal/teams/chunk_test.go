package teams

import (
	"errors"
	"math"
	"testing"

	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

// A conta do sorteio de times e a do Kotlin, inclusive nos valores absurdos --
// todos medidos.
func TestChunkSize(t *testing.T) {
	i32 := func(v int32) *int32 { return &v }
	cases := []struct {
		name      string
		players   int
		teamCount *int32
		teamSize  *int32
		want      int
	}{
		{"2 times para 10", 10, i32(2), nil, 5},
		{"3 times para 10: o ultimo fica menor", 10, i32(3), nil, 4},
		{"mais times que jogadores", 3, i32(5), nil, 1},
		{"sem nada: times de 5", 12, nil, nil, 4}, // ceil(12/5)=3 times -> ceil(12/3)=4
		{"times de 4", 10, nil, i32(4), 4},        // ceil(10/4)=3 times -> ceil(10/3)=4
		{"teamSize zero ou negativo: um time so", 7, nil, i32(0), 7},
		{"teamCount vence teamSize", 10, i32(5), i32(4), 2},
		// divisao por zero em double da infinito, que vira o maior inteiro: um time com todos
		{"teamCount zero", 10, i32(0), nil, math.MaxInt32},
	}
	for _, c := range cases {
		got, err := chunkSize(c.players, autoGenerateRequest{TeamCount: c.teamCount, TeamSize: c.teamSize})
		if err != nil || got != c.want {
			t.Errorf("%s: veio %d (%v), esperava %d", c.name, got, err, c.want)
		}
	}

	// teamCount negativo: o chunked() do Kotlin recusa, e a mensagem dele vai ao cliente
	_, err := chunkSize(10, autoGenerateRequest{TeamCount: i32(-2)})
	var he *httpx.Error
	if !errors.As(err, &he) || he.Status != 400 || *he.Message != "size -5 must be greater than zero." {
		t.Errorf("teamCount negativo: veio %v", err)
	}
}
