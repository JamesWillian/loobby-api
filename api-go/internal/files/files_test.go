package files

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

const owner = "54f29b67-cb8b-4435-8b72-43c17f2f4b8f"

type part struct {
	field, filename, contentType string
	data                         []byte
}

func upload(t *testing.T, parts ...part) (*Upload, error) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, p := range parts {
		h := textproto.MIMEHeader{}
		disposition := `form-data; name="` + p.field + `"`
		if p.filename != "" {
			disposition += `; filename="` + p.filename + `"`
		}
		h.Set("Content-Disposition", disposition)
		if p.contentType != "" {
			h.Set("Content-Type", p.contentType)
		}
		w, err := mw.CreatePart(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(p.data)
	}
	mw.Close()
	req := httptest.NewRequest("POST", "/users/me/avatar", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return ReadUpload(httptest.NewRecorder(), req)
}

func TestReadUpload(t *testing.T) {
	png := []byte("\x89PNG de mentira")
	u, err := upload(t, part{"file", "a.png", "image/png", png})
	if err != nil || !bytes.Equal(u.Data, png) || u.ContentType == nil || *u.ContentType != "image/png" {
		t.Fatalf("upload simples: %+v %v", u, err)
	}
	// varias partes "file": vale a primeira; campos de texto sao ignorados
	u, err = upload(t,
		part{"descricao", "", "", []byte("texto")},
		part{"outro", "x.png", "image/png", []byte("nao e este")},
		part{"file", "1.png", "image/png", []byte("primeiro")},
		part{"file", "2.png", "image/png", []byte("segundo")})
	if err != nil || string(u.Data) != "primeiro" {
		t.Errorf("deveria valer o primeiro \"file\": %+v %v", u, err)
	}
	// 1.000.000 bytes passa (medido)
	if _, err := upload(t, part{"file", "a.png", "image/png", make([]byte, 1_000_000)}); err != nil {
		t.Errorf("1.000.000 bytes deveria passar: %v", err)
	}
}

// Tudo isto e 500 no Kotlin (excecao sem handler), nao 400 -- medido.
func TestReadUploadRecusa(t *testing.T) {
	cases := map[string][]part{
		"sem a parte file":         {{"arquivo", "a.png", "image/png", []byte("x")}},
		"file como campo de texto": {{"file", "", "", []byte("x")}},
		"acima de 1 MB":            {{"file", "a.png", "image/png", make([]byte, maxFileSize+1)}},
		"outro arquivo acima de 1 MB": {
			{"file", "a.png", "image/png", []byte("x")},
			{"extra", "b.png", "image/png", make([]byte, maxFileSize+1)}},
	}
	for name, parts := range cases {
		if _, err := upload(t, parts...); !errors.Is(err, httpx.ErrBody) {
			t.Errorf("%s: esperava ErrBody, veio %v", name, err)
		}
	}
	req := httptest.NewRequest("POST", "/users/me/avatar", strings.NewReader(`{"file":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	if _, err := ReadUpload(httptest.NewRecorder(), req); !errors.Is(err, httpx.ErrBody) {
		t.Errorf("corpo que nao e multipart: esperava ErrBody, veio %v", err)
	}
}

func storage(t *testing.T) *Storage {
	t.Helper()
	s, err := NewStorage(filepath.Join(t.TempDir(), "uploads"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStore(t *testing.T) {
	s := storage(t)
	s.now = func() time.Time { return time.UnixMilli(1789488875123) }
	ct := "image/png"
	url, err := s.Store(&Upload{ContentType: &ct, Data: []byte("png")}, "avatars", owner)
	if err != nil || url != "/files/avatars/"+owner+"/1789488875123.png" {
		t.Fatalf("url %q, err %v", url, err)
	}
	got, err := os.ReadFile(filepath.Join(s.root, "avatars", owner, "1789488875123.png"))
	if err != nil || string(got) != "png" {
		t.Errorf("arquivo nao foi gravado onde a URL aponta: %v", err)
	}
	// mesmo milissegundo: nao sobrescreve, falha (o Files.copy do Kotlin tambem)
	if _, err := s.Store(&Upload{ContentType: &ct, Data: []byte("outro")}, "avatars", owner); err == nil {
		t.Errorf("segundo upload no mesmo milissegundo deveria falhar, nao sobrescrever")
	}

	for mime, ext := range map[string]string{"image/jpeg": ".jpg", "image/webp": ".webp"} {
		s.now = func() time.Time { return time.UnixMilli(1) }
		url, err := s.Store(&Upload{ContentType: &mime, Data: []byte("x")}, "groups", owner)
		if err != nil || !strings.HasSuffix(url, "/1"+ext) {
			t.Errorf("%s: url %q, err %v", mime, url, err)
		}
	}
}

func TestStoreRecusa(t *testing.T) {
	s := storage(t)
	str := func(v string) *string { return &v }
	cases := []struct {
		name string
		u    Upload
		want string
	}{
		{"vazio", Upload{ContentType: str("image/png")}, "Empty file"},
		{"texto", Upload{ContentType: str("text/plain"), Data: []byte("x")}, "Invalid file type: text/plain"},
		// a comparacao do tipo e por texto exato (medido)
		{"maiusculas", Upload{ContentType: str("IMAGE/PNG"), Data: []byte("x")}, "Invalid file type: IMAGE/PNG"},
		{"com parametro", Upload{ContentType: str("image/png;charset=utf-8"), Data: []byte("x")}, "Invalid file type: image/png;charset=utf-8"},
		{"sem tipo", Upload{Data: []byte("x")}, "Invalid file type: null"},
	}
	for _, c := range cases {
		_, err := s.Store(&c.u, "avatars", owner)
		var he *httpx.Error
		if !errors.As(err, &he) || he.Status != 400 || *he.Message != c.want {
			t.Errorf("%s: esperava 400 %q, veio %v", c.name, c.want, err)
		}
	}
}

func serve(s *Storage, target string, hdr map[string]string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	s.Register(mux)
	req := httptest.NewRequest("GET", target, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestServirArquivo(t *testing.T) {
	s := storage(t)
	ct := "image/png"
	url, err := s.Store(&Upload{ContentType: &ct, Data: []byte("0123456789")}, "avatars", owner)
	if err != nil {
		t.Fatal(err)
	}
	rec := serve(s, url, nil)
	if rec.Code != 200 || rec.Body.String() != "0123456789" {
		t.Fatalf("GET %s: %d %q", url, rec.Code, rec.Body)
	}
	h := rec.Header()
	if h.Get("Content-Type") != "image/png" || h.Get("Cache-Control") != "max-age=3600" ||
		h.Get("Accept-Ranges") != "bytes" || h.Get("Last-Modified") == "" {
		t.Errorf("headers: %v", h)
	}
	// revalidacao e leitura parcial, como o ResourceHttpRequestHandler do Spring
	if rec := serve(s, url, map[string]string{"If-Modified-Since": h.Get("Last-Modified")}); rec.Code != 304 {
		t.Errorf("If-Modified-Since: esperava 304, veio %d", rec.Code)
	}
	if rec := serve(s, url, map[string]string{"Range": "bytes=2-4"}); rec.Code != 206 || rec.Body.String() != "234" {
		t.Errorf("Range: esperava 206 \"234\", veio %d %q", rec.Code, rec.Body)
	}
}

// Arquivo que nao existe e 500 no Kotlin (NoResourceFoundException no
// catch-all), nao 404 -- medido. E nada fora da pasta de uploads e servido.
func TestServirOQueNaoExiste(t *testing.T) {
	s := storage(t)
	if err := os.WriteFile(filepath.Join(filepath.Dir(s.root), "segredo.txt"), []byte("fora"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(s.root, "avatars"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/files", "/files/", "/files/nao-existe.png", "/files/avatars", "/files/avatars/"} {
		rec := serve(s, target, nil)
		if rec.Code != 500 || !strings.Contains(rec.Body.String(), `"message":"Unexpected error"`) {
			t.Errorf("GET %s: esperava 500, veio %d %s", target, rec.Code, rec.Body)
		}
	}
	// o handler direto, sem a limpeza de caminho do mux
	req := httptest.NewRequest("GET", "/files/x", nil)
	req.SetPathValue("path", "../segredo.txt")
	rec := httptest.NewRecorder()
	s.serve(rec, req)
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "fora") {
		t.Errorf("caminho para fora da pasta deveria ser recusado, veio %d %s", rec.Code, rec.Body)
	}
}
