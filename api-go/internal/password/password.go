// Package password e o BCryptPasswordEncoder do Spring Security: mesmo
// algoritmo, mesmo custo (10) e os mesmos limites, para que um hash gravado
// por um lado seja aceito pelo outro.
package password

import (
	"errors"
	"regexp"

	"golang.org/x/crypto/bcrypt"

	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

const cost = 10

// ErrTooLong: senha nova com mais de 72 bytes. No Kotlin e a
// IllegalArgumentException do BCrypt do Spring Security, devolvida como 400 com
// esta mensagem (medido).
var ErrTooLong = httpx.BadRequest("password cannot be more than 72 bytes")

// Hash gera o hash de uma senha nova; acima de 72 bytes devolve ErrTooLong.
func Hash(raw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(raw), cost)
	if errors.Is(err, bcrypt.ErrPasswordTooLong) {
		return "", ErrTooLong
	}
	if err != nil {
		return "", err
	}
	return string(h), nil
}

var bcryptPattern = regexp.MustCompile(`\A\$2(a|y|b)?\$(\d\d)\$[./0-9A-Za-z]{53}`)

// Matches confere uma senha contra o hash. Na conferencia (ao contrario da
// geracao) o Spring nao recusa senhas longas: usa so os 72 primeiros bytes.
func Matches(raw, hash string) bool {
	if !bcryptPattern.MatchString(hash) {
		return false
	}
	b := []byte(raw)
	if len(b) > 72 {
		b = b[:72]
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), b) == nil
}
