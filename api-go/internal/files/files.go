// Package files guarda e serve os uploads (avatares e imagens de grupo). Porta
// do FileStorageService e do resource handler de /files/** do Kotlin.
//
// Os arquivos ficam no disco local, em <raiz>/<subpasta>/<dono>/<epochMillis>.<ext>.
// Durante o strangler isso importa: Kotlin e Go rodam em maquinas diferentes,
// cada um com o seu disco -- um arquivo enviado por um lado nao existe no outro.
// As rotas de upload e a de leitura precisam migrar juntas, com os arquivos
// antigos copiados (ver deploy/go/README.md).
package files

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

// Limites padrao do Spring Boot (spring.servlet.multipart.*), que o Kotlin nao
// altera. Medido: 1.000.000 bytes passa, 1.100.000 da 500.
const (
	maxFileSize    = 1 << 20
	maxRequestSize = 10 << 20
)

// Upload e a parte "file" de uma requisicao multipart.
type Upload struct {
	ContentType *string // exatamente como veio na parte; nil se nao veio
	Data        []byte
}

// ReadUpload extrai a parte de arquivo chamada "file", como o
// @RequestParam("file") MultipartFile. Qualquer falha -- nao e multipart, nao
// tem a parte, a parte e um campo de texto, passou do limite -- e no Kotlin uma
// excecao sem handler: 500 "Unexpected error" (medido). Com varias partes
// "file", vale a primeira.
func ReadUpload(w http.ResponseWriter, r *http.Request) (*Upload, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestSize)
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, httpx.ErrBody
	}
	var found *Upload
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, httpx.ErrBody
		}
		_, params, _ := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		_, isFile := params["filename"]
		if !isFile {
			continue
		}
		// O limite de 1 MB vale para todo arquivo da requisicao, nao so o "file".
		data, err := io.ReadAll(io.LimitReader(part, maxFileSize+1))
		if err != nil || len(data) > maxFileSize {
			return nil, httpx.ErrBody
		}
		if found == nil && part.FormName() == "file" {
			found = &Upload{Data: data}
			if ct, ok := part.Header["Content-Type"]; ok && len(ct) > 0 {
				found.ContentType = &ct[0]
			}
		}
	}
	if found == nil {
		return nil, httpx.ErrBody
	}
	return found, nil
}

var extensions = map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp"}

type Storage struct {
	root string
	now  func() time.Time
}

// NewStorage cria a pasta raiz se ela nao existir, como o init do Kotlin.
func NewStorage(root string) (*Storage, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &Storage{root: abs, now: time.Now}, nil
}

// Store grava o arquivo e devolve a URL publica relativa:
// /files/<subpasta>/<dono>/<epochMillis>.<ext>.
//
// O tipo e comparado como texto exato, igual ao Kotlin: "IMAGE/PNG" e
// "image/png;charset=utf-8" sao recusados (medido).
func (s *Storage) Store(u *Upload, subfolder, ownerID string) (string, error) {
	if len(u.Data) == 0 {
		return "", httpx.BadRequest("Empty file")
	}
	ct := "null"
	if u.ContentType != nil {
		ct = *u.ContentType
	}
	ext, ok := extensions[ct]
	if !ok || u.ContentType == nil {
		return "", httpx.BadRequest("Invalid file type: " + ct)
	}
	name := strconv.FormatInt(s.now().UnixMilli(), 10) + "." + ext
	dir := filepath.Join(s.root, subfolder, ownerID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// O_EXCL: dois uploads no mesmo milissegundo colidem, e o segundo falha
	// (500) em vez de sobrescrever -- o Files.copy do Kotlin faz o mesmo.
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(u.Data); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return "/files/" + subfolder + "/" + ownerID + "/" + name, nil
}

var contentTypes = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".webp": "image/webp",
}

// Register publica GET /files/** (rota publica, sem token).
func (s *Storage) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /files/{path...}", s.serve)
	mux.HandleFunc("GET /files", s.serve) // sem isto o mux redirecionaria para /files/
}

// serve entrega um arquivo com cache de 1 hora, Last-Modified, 304 e Range --
// o que o ResourceHttpRequestHandler do Spring faz. Arquivo inexistente (ou
// diretorio) e NoResourceFoundException no Kotlin, que cai no catch-all: 500
// "Unexpected error", nao 404 (medido).
func (s *Storage) serve(w http.ResponseWriter, r *http.Request) {
	rel := r.PathValue("path")
	full := filepath.Join(s.root, filepath.FromSlash(rel))
	if rel == "" || !strings.HasPrefix(full, s.root+string(filepath.Separator)) {
		httpx.Unexpected(w, r)
		return
	}
	f, err := os.Open(full)
	if err != nil {
		httpx.Unexpected(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		httpx.Unexpected(w, r)
		return
	}
	ct, ok := contentTypes[strings.ToLower(filepath.Ext(full))]
	if !ok {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	// Com Cache-Control definido aqui, o SpringHeaders nao acrescenta o trio
	// no-cache -- igual ao Spring, e o que o golden files-servir-avatar grava.
	w.Header().Set("Cache-Control", "max-age=3600")
	http.ServeContent(w, r, "", info.ModTime(), f)
}
