package httpx

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ParseJavaUUID reproduz o java.util.UUID.fromString, que e mais tolerante do
// que parece e e o que o Kotlin usa para o `sub` do token e (pelo conversor do
// Spring) para os {id} do caminho. Medido: "1-1-1-1-1" e aceito e vira
// 00000001-0001-0001-0001-000000000001; maiusculas sao aceitas.
//
// A regra do Java: ate 36 caracteres, exatamente quatro hifens, cada um dos
// cinco grupos lido como hexadecimal (Long.parseLong) e TRUNCADO para o tamanho
// do grupo -- grupo maior que o normal perde os digitos da esquerda.
//
// Devolve a forma canonica (minusculas, 8-4-4-4-12), que e o UUID.toString().
func ParseJavaUUID(s string) (string, bool) {
	if len(s) > 36 {
		return "", false
	}
	parts := strings.Split(s, "-")
	if len(parts) != 5 {
		return "", false
	}
	masks := [5]uint64{0xffffffff, 0xffff, 0xffff, 0xffff, 0xffffffffffff}
	widths := [5]int{8, 4, 4, 4, 12}
	var b strings.Builder
	for i, p := range parts {
		v, ok := javaParseHexLong(p)
		if !ok {
			return "", false
		}
		if i > 0 {
			b.WriteByte('-')
		}
		fmt.Fprintf(&b, "%0*x", widths[i], v&masks[i])
	}
	return b.String(), true
}

// javaParseHexLong: Long.parseLong(s, 16) -- sinal opcional, pelo menos um
// digito, e o valor precisa caber em 64 bits com sinal.
func javaParseHexLong(s string) (uint64, bool) {
	if strings.HasPrefix(s, "+") {
		s = s[1:]
		if strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
			return 0, false
		}
	}
	v, err := strconv.ParseInt(s, 16, 64)
	if err != nil {
		return 0, false
	}
	return uint64(v), true
}

// PathUUID le um {id} do caminho como o @PathVariable UUID do Spring: tira os
// espacos das pontas e aplica o UUID.fromString. Quando falha, o Spring lanca
// MethodArgumentTypeMismatchException, que cai no catch-all: 500 "Unexpected
// error" (medido com /groups/xyz). O chamador responde com Unexpected.
func PathUUID(raw string) (string, bool) {
	return ParseJavaUUID(strings.TrimSpace(raw))
}

// PathID le o {name} do caminho como UUID. Se nao for um, ja escreve o 500 e
// devolve ok=false -- o handler so precisa retornar.
func PathID(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	id, ok := PathUUID(r.PathValue(name))
	if !ok {
		Unexpected(w, r)
	}
	return id, ok
}

// parseJSONUUID e o UUIDDeserializer do Jackson: so a forma canonica de 36
// caracteres, com os hifens no lugar. Bem mais estrito que o UUID.fromString.
func parseJSONUUID(s string) (string, bool) {
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return "", false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return "", false
		}
	}
	return strings.ToLower(s), true
}
