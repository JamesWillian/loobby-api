package auth

import (
	"embed"
	"strings"
)

// As paginas HTML que o AuthController devolve ao navegador e os dois emails.
// Os arquivos em templates/ foram extraidos do codigo Kotlin (o texto entre
// """ ... """.trimIndent()), com os marcadores $message, $email, $token,
// $verifyUrl e $resetUrl mantidos. Com o Kotlin arquivado, esta e a unica copia.
//
//go:embed templates/*.html
var templates embed.FS

func template(name string) string {
	b, err := templates.ReadFile("templates/" + name + ".html")
	if err != nil {
		panic("auth: " + err.Error()) // inalcancavel: o go:embed falha a compilacao
	}
	return string(b)
}

var (
	verifySuccessPage = template("verify-success")
	verifyErrorPage   = template("verify-error")
	resetFormPage     = template("reset-form")
	resetSuccessPage  = template("reset-success")
	resetErrorPage    = template("reset-error")
	verificationEmail = template("email-verification")
	resetEmail        = template("email-reset")
)

// fill troca os marcadores numa unica passada, como a interpolacao do Kotlin:
// um valor que contenha o texto de outro marcador nao e expandido de novo. Os
// valores entram sem escape de HTML, igual ao Kotlin.
func fill(page string, pairs ...string) string {
	return strings.NewReplacer(pairs...).Replace(page)
}
