package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"fmt"
	"io"
	"strings"
	"testing"
)

func archive(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0666, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		tw.Write(data)
	}
	tw.Close()
	gz.Close()
	return b.Bytes()
}
func readArchive(t *testing.T, b []byte) (map[string][]byte, map[string]int64) {
	t.Helper()
	gz, e := gzip.NewReader(bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	files := map[string][]byte{}
	modes := map[string]int64{}
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		files[h.Name], e = io.ReadAll(tr)
		if e != nil {
			t.Fatal(e)
		}
		modes[h.Name] = h.Mode
	}
	return files, modes
}
func TestUniversalArchivePermissionsAndChecksum(t *testing.T) {
	app := archive(t, map[string][]byte{"bin/amd64/miair-plus": []byte("amd64 fixture"), "bin/arm64/miair-plus": []byte("arm64 fixture"), "runtime/amd64/bin/shairport-sync": []byte("#!/bin/sh\n"), "runtime/arm64/bin/nqptp": []byte("ELF"), "runtime/amd64/lib/libtest.so": []byte("library")})
	fpk := archive(t, map[string][]byte{"app.tgz": app, "manifest": []byte("platform=all\r\nchecksum=old\r\n"), "cmd/main": []byte("#!/bin/sh\r\nexit 0\r\n")})
	normalized, e := normalizeTar(fpk, false)
	if e != nil {
		t.Fatal(e)
	}
	files, modes := readArchive(t, normalized)
	if modes["cmd/main"] != 0755 || bytes.Contains(files["cmd/main"], []byte("\r")) {
		t.Fatal("startup script is not executable LF")
	}
	sum := fmt.Sprintf("%x", md5.Sum(files["app.tgz"]))
	if !strings.Contains(string(files["manifest"]), "checksum = "+sum) {
		t.Fatal("checksum not refreshed")
	}
	_, modes = readArchive(t, files["app.tgz"])
	if modes["runtime/amd64/bin/shairport-sync"] != 0755 || modes["runtime/arm64/bin/nqptp"] != 0755 || modes["runtime/amd64/lib/libtest.so"] != 0644 {
		t.Fatal("invalid native runtime permissions")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		if modes["bin/"+arch+"/miair-plus"] != 0755 {
			t.Fatal("missing executable bit", arch)
		}
	}
}
