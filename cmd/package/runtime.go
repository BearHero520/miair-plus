package main

import (
	"bufio"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func verifyRuntime(dir, arch string) error {
	b, err := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
	if err != nil {
		return err
	}
	checked := map[string]bool{}
	scan := bufio.NewScanner(strings.NewReader(string(b)))
	for scan.Scan() {
		fields := strings.Fields(scan.Text())
		if len(fields) != 2 {
			return fmt.Errorf("invalid native runtime checksum line")
		}
		name := fields[1]
		if strings.Contains(name, "\\") || strings.Contains(name, "..") || filepath.IsAbs(name) {
			return fmt.Errorf("invalid native runtime path")
		}
		content, e := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if e != nil {
			return e
		}
		sum := sha256.Sum256(content)
		if hex.EncodeToString(sum[:]) != fields[0] {
			return fmt.Errorf("native checksum mismatch: %s", name)
		}
		checked[name] = true
	}
	if err = scan.Err(); err != nil {
		return err
	}
	for _, name := range []string{"ffmpeg", "shairport-sync", "nqptp"} {
		if !checked["bin/"+name] {
			return fmt.Errorf("native runtime missing %s", name)
		}
	}
	for _, name := range []string{"ffmpeg.real", "shairport-sync.real", "nqptp"} {
		f, e := elf.Open(filepath.Join(dir, "bin", name))
		if e != nil {
			return e
		}
		machine := f.Machine
		f.Close()
		want := elf.EM_X86_64
		if arch == "arm64" {
			want = elf.EM_AARCH64
		}
		if machine != want {
			return fmt.Errorf("wrong architecture for %s/%s: %s", arch, name, machine)
		}
	}
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "bin/") || strings.HasPrefix(rel, "lib/") || strings.HasPrefix(rel, "licenses/") {
			if !checked[rel] {
				return fmt.Errorf("unchecked native runtime file: %s", rel)
			}
		}
		return nil
	})
}
