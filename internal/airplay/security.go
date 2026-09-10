package airplay

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"net"
	"strings"
)

func decode64(s string) ([]byte, error) {
	return base64.RawStdEncoding.DecodeString(strings.TrimRight(strings.TrimSpace(s), "="))
}
func privateKey() *rsa.PrivateKey {
	p, _ := pem.Decode([]byte(airportKey))
	k, err := x509.ParsePKCS1PrivateKey(p.Bytes)
	if err != nil {
		panic(err)
	}
	return k
}
func decryptKey(s string) ([]byte, error) {
	b, err := decode64(s)
	if err != nil {
		return nil, err
	}
	return rsa.DecryptOAEP(sha1.New(), rand.Reader, privateKey(), b, nil)
}
func challenge(s string, ip net.IP, mac []byte) (string, error) {
	b, err := decode64(s)
	if err != nil || len(b) > 16 {
		return "", errors.New("invalid Apple-Challenge")
	}
	b = append(b, ip.To4()...)
	b = append(b, mac...)
	for len(b) < 32 {
		b = append(b, 0)
	}
	signed, err := rsa.SignPKCS1v15(rand.Reader, privateKey(), crypto.Hash(0), b)
	return base64.RawStdEncoding.EncodeToString(signed), err
}
