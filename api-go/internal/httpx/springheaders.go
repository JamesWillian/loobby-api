package httpx

import "net/http"

// SpringHeaders reproduz os headers que o Spring Security e o suporte a CORS
// escrevem em TODA resposta do Kotlin -- inclusive no 401 que sai do filtro de
// autenticacao, antes de qualquer controller. Por isso este middleware precisa
// ser o mais externo da cadeia: ele envolve tambem o middleware de JWT.
//
// Medido em producao e gravado nos golden:
//
//	X-Content-Type-Options: nosniff
//	X-XSS-Protection: 0                   (default do Spring Security 6)
//	X-Frame-Options: DENY
//	Vary: Origin / Access-Control-Request-Method / Access-Control-Request-Headers
//	Cache-Control: no-cache, no-store, max-age=0, must-revalidate
//	Pragma: no-cache
//	Expires: 0
//
// Os tres ultimos so saem quando o handler nao definiu cache: o /legal/* manda
// "max-age=3600, public" e o Spring nao sobrescreve, nem acrescenta Pragma/Expires.
// O CacheControlHeadersWriter decide isso na hora de escrever o status, e aqui e
// igual: a decisao fica para o WriteHeader.
//
// Nao ha HSTS: o TLS termina no nginx e o processo recebe HTTP puro.
func SpringHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Add("Vary", "Origin")
		h.Add("Vary", "Access-Control-Request-Method")
		h.Add("Vary", "Access-Control-Request-Headers")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-XSS-Protection", "0")
		h.Set("X-Frame-Options", "DENY")

		sw := &springWriter{ResponseWriter: w}

		// O Kotlin nao tem nenhuma configuracao de CORS (.cors(withDefaults())
		// sem CorsConfigurationSource), entao TODO preflight e recusado com 403 e
		// corpo literal, sem Content-Type. Durante o strangler preflights vao
		// para o Kotlin pelo nginx; isto vale para quando o Go atender sozinho.
		if isPreflight(r) {
			h["Content-Type"] = nil // impede o net/http de farejar e inventar um
			sw.WriteHeader(http.StatusForbidden)
			_, _ = sw.Write([]byte("Invalid CORS request"))
			return
		}

		next.ServeHTTP(sw, r)
		if !sw.wroteHeader {
			// handler que nao escreveu nada: o net/http mandaria 200 por conta
			// propria, pulando a decisao de cache abaixo.
			sw.WriteHeader(http.StatusOK)
		}
	})
}

func isPreflight(r *http.Request) bool {
	return r.Method == http.MethodOptions &&
		r.Header.Get("Origin") != "" &&
		r.Header.Get("Access-Control-Request-Method") != ""
}

type springWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (s *springWriter) WriteHeader(code int) {
	if !s.wroteHeader {
		s.wroteHeader = true
		h := s.Header()
		if h.Get("Cache-Control") == "" && h.Get("Pragma") == "" && h.Get("Expires") == "" {
			h.Set("Cache-Control", "no-cache, no-store, max-age=0, must-revalidate")
			h.Set("Pragma", "no-cache")
			h.Set("Expires", "0")
		}
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *springWriter) Write(b []byte) (int, error) {
	if !s.wroteHeader {
		s.WriteHeader(http.StatusOK)
	}
	return s.ResponseWriter.Write(b)
}

// Unwrap deixa o http.ResponseController alcancar o writer original (Flush,
// deadlines), que este wrapper nao reimplementa.
func (s *springWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }
