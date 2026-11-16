package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// As paginas e os emails existem duas vezes durante o strangler: no codigo
// Kotlin (texto entre """ ... """.trimIndent()) e em templates/. Este teste
// refaz a extracao e falha se alguem editar um lado e esquecer o outro.
//
// Quando o Kotlin for removido, o teste se pula e templates/ passa a ser a
// unica copia.
func TestTemplatesIguaisAoKotlin(t *testing.T) {
	base := filepath.Join("..", "..", "..", "src", "main", "kotlin", "app", "loobby", "auth")
	sources := []struct{ file, function, template string }{
		{"controller/AuthController.kt", "buildSuccessHtml", verifySuccessPage},
		{"controller/AuthController.kt", "buildErrorHtml", verifyErrorPage},
		{"controller/AuthController.kt", "buildResetPasswordFormHtml", resetFormPage},
		{"controller/AuthController.kt", "buildResetSuccessHtml", resetSuccessPage},
		{"controller/AuthController.kt", "buildResetErrorHtml", resetErrorPage},
		{"service/EmailVerificationService.kt", "buildVerificationEmailHtml", verificationEmail},
		{"service/PasswordResetService.kt", "buildResetEmailHtml", resetEmail},
	}
	for _, s := range sources {
		raw, err := os.ReadFile(filepath.Join(base, s.file))
		if os.IsNotExist(err) {
			t.Skip("codigo Kotlin removido -- fim do strangler para auth")
		}
		if err != nil {
			t.Fatal(err)
		}
		src := string(raw)
		start := strings.Index(src, "fun "+s.function+"(")
		if start < 0 {
			t.Fatalf("%s: funcao %s nao encontrada", s.file, s.function)
		}
		open := strings.Index(src[start:], `"""`)
		end := strings.Index(src[start+open+3:], `"""`)
		if open < 0 || end < 0 {
			t.Fatalf("%s: texto de %s nao encontrado", s.file, s.function)
		}
		kotlin := trimIndent(src[start+open+3 : start+open+3+end])
		if kotlin != s.template {
			t.Errorf("%s divergiu do template Go -- edite os dois (primeira diferenca perto de %q)",
				s.function, firstDifference(kotlin, s.template))
		}
	}
}

// trimIndent: o String.trimIndent() do Kotlin. Tira de todas as linhas o menor
// recuo comum entre as linhas nao vazias, e descarta a primeira e a ultima
// linha se estiverem em branco.
func trimIndent(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	indent := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if n := len(l) - len(strings.TrimLeft(l, " \t")); indent < 0 || n < indent {
			indent = n
		}
	}
	if indent < 0 {
		indent = 0
	}
	var out []string
	for i, l := range lines {
		if (i == 0 || i == len(lines)-1) && strings.TrimSpace(l) == "" {
			continue
		}
		if len(l) >= indent {
			l = l[indent:]
		} else {
			l = ""
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func firstDifference(a, b string) string {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	from := max(0, i-30)
	return a[from:min(len(a), i+30)]
}

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
