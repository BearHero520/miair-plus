// Package produces native fnOS packages using the official fnpack tool.
// It includes corresponding source and normalizes Unix permissions and LF bytes.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	fnpack := flag.String("fnpack", "fnpack", "official fnpack executable")
	native := flag.String("runtime-dir", "", "directory containing verified amd64 and arm64 native audio runtimes")
	flag.Parse()
	if err := build(*fnpack, *native); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func copyTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(source, path)
		dest := filepath.Join(target, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		mode := fs.FileMode(0644)
		if strings.HasPrefix(filepath.ToSlash(rel), "cmd/") {
			mode = 0755
			b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
		}
		return os.WriteFile(dest, b, mode)
	})
}
func sourceArchive(root, target string) error {
	f, err := os.Create(target)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	skip := map[string]bool{".git": true, "node_modules": true, "dist": true, "build": true, "data": true, ".vite": true}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}
		top := strings.Split(filepath.ToSlash(rel), "/")[0]
		allowed := map[string]bool{"cmd": true, "internal": true, "frontend": true, "scripts": true, "packaging": true, "docs": true, ".github": true, "go.mod": true, "go.sum": true, "LICENSE": true, "README.md": true, "UPSTREAM.md": true, ".gitignore": true, ".gitattributes": true}
		if !allowed[top] || skip[d.Name()] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if strings.HasSuffix(rel, ".fpk") || strings.HasSuffix(rel, ".exe") || strings.HasSuffix(rel, ".log") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		mode := int64(0644)
		if strings.HasSuffix(rel, ".sh") || strings.HasPrefix(filepath.ToSlash(rel), "packaging/fpk/cmd/") {
			mode = 0755
			b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
		}
		h := &tar.Header{Name: filepath.ToSlash(rel), Mode: mode, Size: int64(len(b))}
		if err = tw.WriteHeader(h); err != nil {
			return err
		}
		_, err = tw.Write(b)
		return err
	})
	if e := tw.Close(); err == nil {
		err = e
	}
	if e := gz.Close(); err == nil {
		err = e
	}
	return err
}
func repack(input, output string) error {
	b, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	b, err = normalizeTar(b, false)
	if err != nil {
		return err
	}
	return os.WriteFile(output, b, 0644)
}
func normalizeTar(input []byte, app bool) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(input))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	type item struct {
		h tar.Header
		b []byte
	}
	items := []item{}
	checksum := ""
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		name := strings.TrimPrefix(h.Name, "./")
		if name == "app.tgz" && !app {
			b, err = normalizeTar(b, true)
			if err != nil {
				return nil, err
			}
			sum := md5.Sum(b)
			checksum = hex.EncodeToString(sum[:])
		}
		if h.Typeflag == tar.TypeDir || strings.HasPrefix(name, "cmd/") || app && (strings.HasPrefix(name, "bin/") || strings.HasPrefix(name, "runtime/") && strings.Contains(name, "/bin/")) {
			h.Mode = 0755
		} else {
			h.Mode = 0644
		}
		if strings.HasPrefix(name, "cmd/") {
			b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
		}
		h.Size = int64(len(b))
		items = append(items, item{*h, b})
	}
	var out bytes.Buffer
	zw := gzip.NewWriter(&out)
	tw := tar.NewWriter(zw)
	for _, entry := range items {
		if strings.TrimPrefix(entry.h.Name, "./") == "manifest" && !app {
			lines := strings.Split(strings.ReplaceAll(string(entry.b), "\r", ""), "\n")
			for i, line := range lines {
				k, _, ok := strings.Cut(line, "=")
				if ok && strings.TrimSpace(k) == "checksum" {
					lines[i] = "checksum = " + checksum
				}
			}
			entry.b = []byte(strings.Join(lines, "\n"))
			entry.h.Size = int64(len(entry.b))
		}
		if err = tw.WriteHeader(&entry.h); err != nil {
			return nil, err
		}
		if _, err = tw.Write(entry.b); err != nil {
			return nil, err
		}
	}
	if err = tw.Close(); err != nil {
		return nil, err
	}
	if err = zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
func build(fnpack, native string) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err = os.Stat(filepath.Join(root, "internal/web/dist/index.html")); err != nil {
		return fmt.Errorf("build frontend first")
	}
	fnpack, err = exec.LookPath(fnpack)
	if err != nil {
		return err
	}
	fnpack, err = filepath.Abs(fnpack)
	if err != nil {
		return err
	}
	out := filepath.Join(root, "dist")
	if err = os.MkdirAll(out, 0755); err != nil {
		return err
	}
	var sums strings.Builder
	for _, platform := range []string{"all"} {
		stage, err := os.MkdirTemp(filepath.Join(root, "build"), "fpk-"+platform+"-")
		if os.IsNotExist(err) {
			if err = os.MkdirAll(filepath.Join(root, "build"), 0755); err != nil {
				return err
			}
			stage, err = os.MkdirTemp(filepath.Join(root, "build"), "fpk-"+platform+"-")
		}
		if err != nil {
			return err
		}
		if err = copyTree(filepath.Join(root, "packaging/fpk"), stage); err != nil {
			return err
		}
		for _, arch := range []string{"amd64", "arm64"} {
			if native == "" {
				return fmt.Errorf("AirPlay 2 package requires --runtime-dir with both architectures")
			}
			runtimeSource := filepath.Join(native, arch)
			if err = verifyRuntime(runtimeSource, arch); err != nil {
				return err
			}
			for _, dir := range []string{"bin", "lib", "licenses"} {
				if err = copyTree(filepath.Join(runtimeSource, dir), filepath.Join(stage, "app/runtime", arch, dir)); err != nil {
					return err
				}
			}
			bin := filepath.Join(stage, "app/bin", arch)
			if err = os.MkdirAll(bin, 0755); err != nil {
				return err
			}
			command := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(bin, "miair-plus"), "./cmd/miair-plus")
			command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+arch)
			command.Stdout = os.Stdout
			command.Stderr = os.Stderr
			if err = command.Run(); err != nil {
				return err
			}
			_ = os.Chmod(filepath.Join(bin, "miair-plus"), 0755)
		}
		if err = sourceArchive(root, filepath.Join(stage, "app/source.tar.gz")); err != nil {
			return err
		}
		license, _ := os.ReadFile(filepath.Join(root, "LICENSE"))
		_ = os.WriteFile(filepath.Join(stage, "app/LICENSE"), license, 0644)
		command := exec.Command(fnpack, "build", "--directory", stage)
		command.Dir = stage
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err = command.Run(); err != nil {
			return err
		}
		matches, _ := filepath.Glob(filepath.Join(stage, "*.fpk"))
		if len(matches) != 1 {
			return fmt.Errorf("fnpack produced %d packages", len(matches))
		}
		name := "miair-plus-2.0.0-alpha.5-" + platform + ".fpk"
		target := filepath.Join(out, name)
		if err = repack(matches[0], target); err != nil {
			return err
		}
		b, err := os.ReadFile(target)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(sum[:]), name)
		fmt.Println(target)
	}
	return os.WriteFile(filepath.Join(out, "SHA256SUMS"), []byte(sums.String()), 0644)
}
