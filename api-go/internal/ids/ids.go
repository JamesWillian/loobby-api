// Package ids gera os identificadores e codigos aleatorios que o Kotlin gera
// com UUID.randomUUID() e com sorteio de caracteres.
package ids

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
)

// New devolve um UUID v4 no formato do UUID.toString() do Java.
func New() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("ids: sem entropia: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // versao 4
	b[8] = (b[8] & 0x3f) | 0x80 // variante RFC 4122
	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], b[10:16])
	return string(out[:])
}

const codeAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// Code sorteia n caracteres de A-Z e 0-9, o alfabeto dos codigos de convite.
func Code(n int) string {
	out := make([]byte, n)
	max := big.NewInt(int64(len(codeAlphabet)))
	for i := range out {
		k, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic("ids: sem entropia: " + err.Error())
		}
		out[i] = codeAlphabet[k.Int64()]
	}
	return string(out)
}
