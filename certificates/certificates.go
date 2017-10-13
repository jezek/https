// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Generate a self-signed X.509 certificate for a TLS server. Outputs to
// 'cert.pem' and 'key.pem' and will overwrite existing files.

// Small modifications by kabukky
// Support cert generation without disk access by gerald1248

package certificates

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	validFrom  = ""
	validFor   = 365 * 24 * time.Hour
	isCA       = true
	rsaBits    = 2048
	ecdsaCurve = ""
)

func publicKey(priv interface{}) interface{} {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	default:
		return nil
	}
}

func pemBlockForKey(priv interface{}) *pem.Block {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to marshal ECDSA private key: %v", err)
			os.Exit(2)
		}
		return &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}
	default:
		return nil
	}
}

func Check(certPath string, keyPath string) error {
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		return err
	} else if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		return err
	}
	return nil
}

//generate cert and key byte arrays
//these can then be used to populate the server configuration
//in place of files on disk
func GenerateArrays(host string) ([]byte, []byte, error) {
	var priv interface{}
	var err error
	switch ecdsaCurve {
	case "":
		priv, err = rsa.GenerateKey(rand.Reader, rsaBits)
	case "P224":
		priv, err = ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	case "P256":
		priv, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "P384":
		priv, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case "P521":
		priv, err = ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	default:
		fmt.Fprintf(os.Stderr, "Unrecognized elliptic curve: %q", ecdsaCurve)
		os.Exit(1)
	}
	if err != nil {
		log.Printf("failed to generate private key: %s", err)
		return nil, nil, err
	}

	var notBefore time.Time
	if len(validFrom) == 0 {
		notBefore = time.Now()
	} else {
		notBefore, err = time.Parse("Jan 2 15:04:05 2006", validFrom)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to parse creation date: %s\n", err)
			return nil, nil, err
		}
	}

	notAfter := notBefore.Add(validFor)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		log.Printf("failed to generate serial number: %s", err)
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	hosts := strings.Split(host, ",")
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	if isCA {
		template.IsCA = true
		template.KeyUsage |= x509.KeyUsageCertSign
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey(priv), priv)
	if err != nil {
		log.Printf("Failed to create certificate: %s", err)
		return nil, nil, err
	}

	certArray := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyArray := pem.EncodeToMemory(pemBlockForKey(priv))

	return certArray, keyArray, nil
}

func Generate(certPath, keyPath, host string) error {
	certArray, keyArray, err := GenerateArrays(host)
	if err != nil {
		return err
	}

	certDir := filepath.Dir(certPath)
	if err := mkdirIfNecessary(certDir); err != nil {
		log.Printf("failed to create "+certDir+": %s", err)
		return err
	}

	err = ioutil.WriteFile(certPath, certArray, 0600)
	if err != nil {
		log.Printf("failed to open "+certPath+" for writing: %s", err)
		return err
	}
	log.Print("written cert.pem\n")

	keyDir := filepath.Dir(keyPath)
	if err := mkdirIfNecessary(keyDir); err != nil {
		log.Printf("failed to create "+keyDir+": %s", err)
		return err
	}

	err = ioutil.WriteFile(keyPath, keyArray, 0600)
	if err != nil {
		log.Printf("failed to open "+keyPath+" for writing: %s", err)
		return err
	}
	log.Print("written key.pem\n")

	return nil
}

func mkdirIfNecessary(dir string) error {
	fi, err := os.Stat(dir)

	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if err == nil && fi.IsDir() {
		return nil
	}

	return os.MkdirAll(dir, 0755)
}
