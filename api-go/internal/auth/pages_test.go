package auth

import (
	"strings"
	"testing"
)

// fill troca os marcadores numa passada so, como a interpolacao do Kotlin: o
// valor de um marcador que contenha o nome de outro nao e expandido de novo.
func TestFill(t *testing.T) {
	got := fill("Conta: $email / token: $token", "$email", "x$token@loobby.app", "$token", "abc")
	if got != "Conta: x$token@loobby.app / token: abc" {
		t.Errorf("veio %q", got)
	}
	for name, page := range map[string]string{
		"verify-error": verifyErrorPage, "reset-error": resetErrorPage,
	} {
		if !strings.Contains(page, "$message") {
			t.Errorf("%s perdeu o marcador $message", name)
		}
	}
	if !strings.Contains(resetFormPage, "$email") || !strings.Contains(resetFormPage, "$token") ||
		!strings.Contains(verificationEmail, "$verifyUrl") || !strings.Contains(resetEmail, "$resetUrl") {
		t.Errorf("algum template perdeu o marcador")
	}
}
