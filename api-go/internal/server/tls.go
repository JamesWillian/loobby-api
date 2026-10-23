package server

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

// TLSConfig monta o TLS mutuo do servidor: certificado proprio para o nginx
// verificar, e certificado de cliente OBRIGATORIO, assinado pela CA privada.
//
// A CA so assina dois certificados -- o deste servidor (uso: serverAuth) e o do
// nginx (uso: clientAuth). O Go confere o uso estendido do certificado do
// cliente, entao nem o certificado do proprio servidor serve para conectar.
//
// TLS 1.2 como minimo, e nao 1.3, por compatibilidade: nao sabemos com qual
// OpenSSL o nginx do AWS foi compilado. As suites padrao do Go para 1.2 sao
// todas ECDHE com AEAD; nada fraco e negociado.
func TLSConfig(certFile, keyFile, clientCAFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("certificado do servidor: %w", err)
	}
	caPEM, err := os.ReadFile(clientCAFile)
	if err != nil {
		return nil, fmt.Errorf("CA dos clientes: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("CA dos clientes: nenhum certificado PEM valido no arquivo")
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
	}, nil
}
