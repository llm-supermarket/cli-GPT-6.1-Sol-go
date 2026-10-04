package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelease(t *testing.T) {
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	if err := os.Mkdir(dist, 0755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "binary")
	payload := "test executable\n"
	if err := os.WriteFile(binary, []byte(payload), 0755); err != nil {
		t.Fatal(err)
	}
	version := "v0.1.42"
	hashes := make(map[string]string)
	for _, target := range targets {
		windows := strings.HasPrefix(target, "windows/")
		name := app
		if windows {
			name += ".exe"
		}
		path := filepath.Join(dist, archiveName(version, target))
		if err := writeArchive(binary, name, path, windows); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		hashes[target] = fmt.Sprintf("%x", sha256.Sum256(data))
		var entry io.ReadCloser
		if windows {
			reader, err := zip.OpenReader(path)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			if len(reader.File) != 1 || reader.File[0].Name != name {
				t.Fatalf("unexpected zip entries: %v", reader.File)
			}
			entry, err = reader.File[0].Open()
			if err != nil {
				t.Fatal(err)
			}
		} else {
			file, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			gz, err := gzip.NewReader(file)
			if err != nil {
				t.Fatal(err)
			}
			defer gz.Close()
			reader := tar.NewReader(gz)
			header, err := reader.Next()
			if err != nil || header.Name != name || header.Mode != 0755 {
				t.Fatalf("unexpected tar header: %v, %v", header, err)
			}
			entry = io.NopCloser(reader)
		}
		content, err := io.ReadAll(entry)
		entry.Close()
		if err != nil || string(content) != payload {
			t.Fatalf("unexpected archive content: %q, %v", content, err)
		}
	}
	if err := release(version, dist, root, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "bucket", app+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version      string
		Bin          string
		Architecture map[string]struct{ URL, Hash string }
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "0.1.42" || manifest.Bin != app+".exe" || len(manifest.Architecture) != 2 {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	for scoopArch, goArch := range map[string]string{"64bit": "amd64", "arm64": "arm64"} {
		target := "windows/" + goArch
		entry := manifest.Architecture[scoopArch]
		wantURL := repository + "/releases/download/" + version + "/" + archiveName(version, target)
		if entry.Hash != hashes[target] || entry.URL != wantURL {
			t.Fatalf("incorrect Scoop architecture: %+v", entry)
		}
	}
	formula, err := os.ReadFile(filepath.Join(root, "Formula", "cli-gpt-6.1-sol-go.rb"))
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets[:4] {
		if !strings.Contains(string(formula), hashes[target]) || !strings.Contains(string(formula), archiveName(version, target)) {
			t.Fatalf("formula missing URL/hash for %s", target)
		}
	}
	if !strings.Contains(string(formula), "class CliGpt61SolGo < Formula") || !strings.Contains(string(formula), `bin.install "`+app+`"`) {
		t.Fatal("formula class or binary name mismatch")
	}
	checksums, err := os.ReadFile(filepath.Join(dist, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.Split(strings.TrimSpace(string(checksums)), "\n")) != len(targets) {
		t.Fatal("incorrect checksum count")
	}
	for _, target := range targets {
		if !strings.Contains(string(checksums), hashes[target]+"  "+archiveName(version, target)) {
			t.Fatalf("missing checksum for %s", target)
		}
	}
}

func TestReleaseRejectsInvalidVersion(t *testing.T) {
	for _, version := range []string{"", "0.1.1", "v1.0.0", "v0.1.1/../../file"} {
		if err := release(version, t.TempDir(), t.TempDir(), false); err == nil {
			t.Fatalf("accepted invalid tag %q", version)
		}
	}
}

func TestReleaseRequiresAllArchives(t *testing.T) {
	if err := release("v0.1.1", t.TempDir(), t.TempDir(), false); err == nil {
		t.Fatal("accepted missing archives")
	}
}
