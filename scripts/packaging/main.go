// Command packaging cross-compiles the root CLI and generates release packages.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const app = "cli-GPT-6.1-Sol-go"
const repository = "https://github.com/llm-supermarket/" + app

var targets = []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64", "windows/amd64", "windows/arm64"}

func main() {
	version := flag.String("version", "", "release tag, e.g. v0.1.1")
	dist := flag.String("dist", "dist", "release archive directory")
	output := flag.String("output", ".", "root for generated bucket and Formula directories")
	build := flag.Bool("build", true, "cross-compile and archive; false uses existing archives")
	flag.Parse()
	if err := release(*version, *dist, *output, *build); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func archiveName(version, target string) string {
	ext := ".tar.gz"
	if strings.HasPrefix(target, "windows/") {
		ext = ".zip"
	}
	return app + "_" + version + "_" + strings.ReplaceAll(target, "/", "_") + ext
}

func release(version, dist, output string, build bool) error {
	if !regexp.MustCompile(`^v0\.1\.[0-9]+$`).MatchString(version) {
		return fmt.Errorf("version must be a v0.1.<number> tag")
	}
	if err := os.MkdirAll(dist, 0755); err != nil {
		return err
	}
	hashes := make(map[string]string)
	var checksums strings.Builder
	for _, target := range targets {
		name := archiveName(version, target)
		path := filepath.Join(dist, name)
		if build {
			if err := buildArchive(version, target, path); err != nil {
				return err
			}
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		hashes[target] = fmt.Sprintf("%x", h.Sum(nil))
		fmt.Fprintf(&checksums, "%s  %s\n", hashes[target], name)
	}
	url := func(target string) string {
		return repository + "/releases/download/" + version + "/" + archiveName(version, target)
	}
	manifest := map[string]any{
		"version":     strings.TrimPrefix(version, "v"),
		"description": "Standalone cli-GPT-6.1-Sol-go CLI",
		"homepage":    repository,
		"bin":         app + ".exe",
		"architecture": map[string]any{
			"64bit": map[string]string{"url": url("windows/amd64"), "hash": hashes["windows/amd64"]},
			"arm64": map[string]string{"url": url("windows/arm64"), "hash": hashes["windows/arm64"]},
		},
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	var formula strings.Builder
	fmt.Fprintf(&formula, "class CliGpt61SolGo < Formula\n  desc \"Standalone cli-GPT-6.1-Sol-go CLI\"\n  homepage %q\n  version %q\n\n", repository, strings.TrimPrefix(version, "v"))
	for _, system := range []struct{ block, os string }{{"on_macos", "darwin"}, {"on_linux", "linux"}} {
		fmt.Fprintf(&formula, "  %s do\n", system.block)
		for _, arch := range []struct{ block, goarch string }{{"on_intel", "amd64"}, {"on_arm", "arm64"}} {
			target := system.os + "/" + arch.goarch
			fmt.Fprintf(&formula, "    %s do\n      url %q\n      sha256 %q\n    end\n", arch.block, url(target), hashes[target])
		}
		formula.WriteString("  end\n\n")
	}
	fmt.Fprintf(&formula, "  def install\n    bin.install %q\n  end\n\n  test do\n    assert_predicate bin/%q, :executable?\n  end\nend\n", app, app)
	for path, content := range map[string][]byte{
		filepath.Join(dist, "SHA256SUMS"):                         []byte(checksums.String()),
		filepath.Join(output, "bucket", app+".json"):              append(data, '\n'),
		filepath.Join(output, "Formula", "cli-gpt-6.1-sol-go.rb"): []byte(formula.String()),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, content, 0644); err != nil {
			return err
		}
	}
	return nil
}

func buildArchive(version, target, path string) error {
	temp, err := os.MkdirTemp("", "cli-release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	parts := strings.Split(target, "/")
	name := app
	if parts[0] == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(temp, name)
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w -X main.version="+version, "-o", binary, ".")
	// Remove inherited target settings before supplying the release target.
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, "GOOS") && !strings.EqualFold(key, "GOARCH") && !strings.EqualFold(key, "CGO_ENABLED") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "CGO_ENABLED=0", "GOOS="+parts[0], "GOARCH="+parts[1])
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build %s: %w", target, err)
	}
	return writeArchive(binary, name, path, parts[0] == "windows")
}

func writeArchive(binary, name, path string, windows bool) (err error) {
	input, err := os.Open(binary)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	output, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := output.Close(); err == nil {
			err = closeErr
		}
	}()
	if windows {
		writer := zip.NewWriter(output)
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0755)
		entry, writeErr := writer.CreateHeader(header)
		if writeErr == nil {
			_, writeErr = io.Copy(entry, input)
		}
		closeErr := writer.Close()
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}
	gz := gzip.NewWriter(output)
	writer := tar.NewWriter(gz)
	err = writer.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: info.Size(), ModTime: time.Unix(0, 0)})
	if err == nil {
		_, err = io.Copy(writer, input)
	}
	tarErr := writer.Close()
	gzErr := gz.Close()
	if err != nil {
		return err
	}
	if tarErr != nil {
		return tarErr
	}
	return gzErr
}
