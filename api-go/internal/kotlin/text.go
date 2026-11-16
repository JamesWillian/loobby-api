// Package kotlin reproduz funcoes de texto da biblioteca padrao do Kotlin cujo
// resultado difere das equivalentes do Go em casos de borda.
package kotlin

import (
	"strings"
	"unicode"
)

// IsWhitespace: Char.isWhitespace() do Kotlin/JVM -- separadores Unicode
// (inclusive o espaco nao separavel U+00A0, que o strings.TrimSpace tambem
// trata) mais os controles U+0009..U+000D e U+001C..U+001F.
func IsWhitespace(r rune) bool {
	if r >= 0x09 && r <= 0x0D || r >= 0x1C && r <= 0x1F {
		return true
	}
	return unicode.In(r, unicode.Zs, unicode.Zl, unicode.Zp)
}

// Trim: CharSequence.trim().
func Trim(s string) string { return strings.TrimFunc(s, IsWhitespace) }

// IsBlank: CharSequence.isBlank() -- vazio ou so com espacos.
func IsBlank(s string) bool { return Trim(s) == "" }

// IfBlankNil: s.ifBlank { null }.
func IfBlankNil(s string) *string {
	if IsBlank(s) {
		return nil
	}
	return &s
}
