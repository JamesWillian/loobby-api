// Package validation reproduz o Bean Validation do lado Kotlin (Hibernate
// Validator 8.0.3): as mensagens no idioma do Accept-Language, o validador de
// email e as regras de @NotBlank/@Size. O gabarito (testdata/oracle.json) foi
// gerado pelo validador real -- ver testdata/ValidationOracle.java.
package validation

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

var (
	localPartRe = regexp.MustCompile(localPartPattern)
	domainRe    = regexp.MustCompile(domainPattern)
)

// Bundle escolhe o idioma das mensagens como o Spring + Tomcat: o idioma de
// maior q do Accept-Language; sem cabecalho (ou sem idioma conhecido), o padrao
// da JVM de producao, en_US. Medido no Kotlin: pt-BR, pt e pt-PT respondem em
// portugues, "es" em espanhol, ausente em ingles.
func Bundle(acceptLanguage string) string {
	type entry struct {
		tag string
		q   float64
	}
	var entries []entry
	for _, part := range strings.Split(acceptLanguage, ",") {
		tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		tag = strings.TrimSpace(tag)
		if tag == "" || tag == "*" {
			continue
		}
		q := 1.0
		if _, v, ok := strings.Cut(strings.TrimSpace(params), "="); ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				q = f
			}
		}
		if q < 0.00005 {
			continue
		}
		entries = append(entries, entry{tag, q})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].q > entries[j].q })
	if len(entries) > 0 {
		sub := strings.Split(entries[0].tag, "-")
		lang := strings.ToLower(sub[0])
		for _, s := range sub[1:] {
			if len(s) == 2 || len(s) == 3 { // regiao (BR, 419); pula script (Hant)
				if _, ok := messages[lang+"_"+strings.ToUpper(s)]; ok {
					return lang + "_" + strings.ToUpper(s)
				}
				break
			}
		}
		if _, ok := messages[lang]; ok && lang != "" {
			return lang
		}
	}
	return "en"
}

// Message devolve o texto de uma restricao (NotBlank, Email, FutureOrPresent...).
func Message(bundle, constraint string) string { return messages[bundle][constraint] }

// SizeMessage interpola {min} e {max}, como o Hibernate Validator.
func SizeMessage(bundle string, min, max int) string {
	m := strings.ReplaceAll(messages[bundle]["Size"], "{min}", strconv.Itoa(min))
	return strings.ReplaceAll(m, "{max}", strconv.Itoa(max))
}

// NotBlank: trim() do Java (remove so caracteres <= U+0020) e tamanho > 0.
func NotBlank(s string) bool {
	return strings.TrimFunc(s, func(r rune) bool { return r <= ' ' }) != ""
}

// Len e o String.length() do Java: unidades UTF-16. E o que @Size e os
// "length < 6" do Kotlin contam -- um emoji vale 2.
func Len(s string) int { return len(utf16.Encode([]rune(s))) }

// IsEmail e o @Email do Hibernate Validator. Vazio e valido (quem recusa vazio
// e o @NotBlank).
func IsEmail(v string) bool {
	if v == "" {
		return true
	}
	i := strings.LastIndex(v, "@")
	if i < 0 {
		return false
	}
	local, domain := v[:i], v[i+1:]
	if Len(local) > 64 || !localPartRe.MatchString(local) {
		return false
	}
	if strings.HasSuffix(domain, ".") {
		return false
	}
	ascii, ok := toASCII(domain)
	return ok && len(ascii) <= 255 && domainRe.MatchString(ascii)
}

// toASCII aproxima o java.net.IDN.toASCII: rotulos ASCII passam como estao
// (maximo de 63 caracteres); rotulos com acento viram punycode. Caracteres fora
// do plano basico (emoji) sao recusados, como no IDNA2003 do Java.
func toASCII(domain string) (string, bool) {
	labels := strings.Split(domain, ".")
	for i, label := range labels {
		ascii := true
		for _, r := range label {
			if r > 0xFFFF {
				return "", false
			}
			if r >= 0x80 {
				ascii = false
			}
		}
		if !ascii {
			label = "xn--" + punycode(strings.ToLower(label))
			labels[i] = label
		}
		if len(label) > 63 {
			return "", false
		}
	}
	return strings.Join(labels, "."), true
}

// punycode: codificador da RFC 3492.
func punycode(s string) string {
	const base, tmin, tmax, skew, damp = 36, 1, 26, 38, 700
	runes := []rune(s)
	var out []byte
	for _, r := range runes {
		if r < 0x80 {
			out = append(out, byte(r))
		}
	}
	b := len(out)
	h := b
	if b > 0 {
		out = append(out, '-')
	}
	digit := func(d int) byte {
		if d < 26 {
			return byte('a' + d)
		}
		return byte('0' + d - 26)
	}
	n, delta, bias := 0x80, 0, 72
	for h < len(runes) {
		m := int(^uint(0) >> 1)
		for _, r := range runes {
			if int(r) >= n && int(r) < m {
				m = int(r)
			}
		}
		delta += (m - n) * (h + 1)
		n = m
		for _, r := range runes {
			if int(r) < n {
				delta++
			}
			if int(r) == n {
				q := delta
				for k := base; ; k += base {
					t := k - bias
					if t < tmin {
						t = tmin
					} else if t > tmax {
						t = tmax
					}
					if q < t {
						break
					}
					out = append(out, digit(t+(q-t)%(base-t)))
					q = (q - t) / (base - t)
				}
				out = append(out, digit(q))
				// adapt
				if h == b {
					delta /= damp
				} else {
					delta /= 2
				}
				delta += delta / (h + 1)
				k := 0
				for delta > ((base-tmin)*tmax)/2 {
					delta /= base - tmin
					k += base
				}
				bias = k + (base-tmin+1)*delta/(delta+skew)
				delta = 0
				h++
			}
		}
		delta++
		n++
	}
	return string(out)
}
