package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type pair struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	der  []byte
}

func newCA(t *testing.T, name string) pair {
	t.Helper()
	return issue(t, &x509.Certificate{
		Subject:               pkix.Name{CommonName: name},
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}, nil)
}

func issue(t *testing.T, tmpl *x509.Certificate, parent *pair) pair {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl.SerialNumber = big.NewInt(time.Now().UnixNano())
	tmpl.NotBefore = time.Now().Add(-time.Hour)
	tmpl.NotAfter = time.Now().Add(time.Hour)
	signer, signerKey := tmpl, key
	if parent != nil {
		signer, signerKey = parent.cert, parent.key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, signer, &key.PublicKey, signerKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return pair{cert, key, der}
}

func leaf(t *testing.T, ca pair, cn string, usage x509.ExtKeyUsage) pair {
	t.Helper()
	return issue(t, &x509.Certificate{
		Subject:     pkix.Name{CommonName: cn},
		DNSNames:    []string{cn},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{usage},
	}, &ca)
}

func writePEM(t *testing.T, dir, name string, p pair) (certFile, keyFile string) {
	t.Helper()
	certFile = filepath.Join(dir, name+".crt")
	keyFile = filepath.Join(dir, name+".key")
	keyDER, err := x509.MarshalECPrivateKey(p.key)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: p.der}), 0o600)
	_ = os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600)
	return
}

func tlsPair(p pair) tls.Certificate {
	return tls.Certificate{Certificate: [][]byte{p.der}, PrivateKey: p.key}
}

// O servidor so pode completar o handshake com o certificado de cliente emitido
// pela CA privada. Qualquer outra tentativa tem que falhar ANTES de chegar no
// handler -- inclusive usar o certificado do proprio servidor como cliente.
func TestTLSMutuoSoAceitaONginx(t *testing.T) {
	dir := t.TempDir()
	ca := newCA(t, "Loobby strangler CA (teste)")
	server := leaf(t, ca, "loobby-go", x509.ExtKeyUsageServerAuth)
	nginx := leaf(t, ca, "loobby-nginx", x509.ExtKeyUsageClientAuth)
	rogueCA := newCA(t, "CA de outra pessoa")
	rogue := leaf(t, rogueCA, "loobby-nginx", x509.ExtKeyUsageClientAuth)

	certFile, keyFile := writePEM(t, dir, "server", server)
	caFile, _ := writePEM(t, dir, "ca", ca)

	cfg, err := TLSConfig(certFile, keyFile, caFile)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = cfg
	srv.StartTLS()
	defer srv.Close()

	roots := x509.NewCertPool()
	roots.AddCert(ca.cert)
	get := func(client *tls.Certificate) error {
		tc := &tls.Config{RootCAs: roots, ServerName: "loobby-go"}
		if client != nil {
			tc.Certificates = []tls.Certificate{*client}
		}
		c := &http.Client{Transport: &http.Transport{TLSClientConfig: tc}, Timeout: 5 * time.Second}
		resp, err := c.Get(srv.URL)
		if err != nil {
			return err
		}
		resp.Body.Close()
		return nil
	}

	nginxCert := tlsPair(nginx)
	if err := get(&nginxCert); err != nil {
		t.Fatalf("o nginx tem que conseguir conectar: %v", err)
	}

	rogueCert := tlsPair(rogue)
	serverAsClient := tlsPair(server)
	for name, cert := range map[string]*tls.Certificate{
		"sem certificado":                      nil,
		"certificado de outra CA":              &rogueCert,
		"certificado do servidor como cliente": &serverAsClient,
	} {
		if err := get(cert); err == nil {
			t.Errorf("%s: conexao aceita -- deveria ter sido recusada no handshake", name)
		}
	}
}

func TestTLSConfigRecusaArquivosInvalidos(t *testing.T) {
	dir := t.TempDir()
	ca := newCA(t, "ca")
	server := leaf(t, ca, "loobby-go", x509.ExtKeyUsageServerAuth)
	certFile, keyFile := writePEM(t, dir, "server", server)
	vazio := filepath.Join(dir, "vazio.pem")
	_ = os.WriteFile(vazio, []byte("nao e pem"), 0o600)

	if _, err := TLSConfig(certFile, keyFile, vazio); err == nil {
		t.Error("CA sem PEM valido deveria falhar, nao subir aceitando ninguem/qualquer um")
	}
	if _, err := TLSConfig(filepath.Join(dir, "nao-existe.crt"), keyFile, certFile); err == nil {
		t.Error("certificado inexistente deveria falhar")
	}
}
