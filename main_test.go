package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func noEnv(string) (string, bool) { return "", false }

func invoke(t *testing.T, args []string, input string, env map[string]string) (string, string, error) {
	t.Helper()
	var out, warnings bytes.Buffer
	err := run(args, strings.NewReader(input), &out, &warnings, func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	})
	return strings.TrimSpace(out.String()), warnings.String(), err
}

func TestRoundTrip(t *testing.T) {
	for _, encoding := range []string{"base32", "base64", "base32768"} {
		for _, salt := range []string{"", "different optional salt"} {
			for _, source := range []string{"flags", "environment", "prompts"} {
				t.Run(encoding+"/"+salt+"/"+source, func(t *testing.T) {
					dir := t.TempDir()
					original := filepath.Join(dir, "example.txt")
					// More than two crypt blocks, including all byte values.
					content := bytes.Repeat([]byte("\x00\xffauthenticated contents\n"), 7000)
					if err := os.WriteFile(original, content, 0600); err != nil {
						t.Fatal(err)
					}
					args := []string{"encrypt", "-i", original, "--filename-encoding", encoding}
					var env map[string]string
					prompt := ""
					credentials := []string{}
					if source == "flags" {
						credentials = []string{"--password", "Testpassword1", "--salt", salt}
					}
					if source == "environment" {
						env = map[string]string{"RCLONE_CRYPT_PASSWORD": "Testpassword1", "RCLONE_CRYPT_SALT": salt}
					}
					if source == "prompts" {
						prompt = "Testpassword1\n" + salt + "\n"
					}
					args = append(args, credentials...)
					encrypted, warnings, err := invoke(t, args, prompt, env)
					if err != nil {
						t.Fatal(err)
					}
					if source == "flags" && !strings.Contains(warnings, "terminal history") {
						t.Fatal("missing security warning")
					}
					if source == "prompts" && (!strings.Contains(warnings, "Password:") || !strings.Contains(warnings, "Salt (optional")) {
						t.Fatal("missing prompts")
					}
					if strings.Contains(warnings, "Testpassword1") {
						t.Fatal("password leaked")
					}
					data, err := os.ReadFile(encrypted)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.HasPrefix(data, []byte("RCLONE\x00\x00")) || bytes.Contains(data, []byte("authenticated contents")) {
						t.Fatal("not encrypted in rclone format")
					}
					if err := os.Remove(original); err != nil {
						t.Fatal(err)
					}
					args = append([]string{"decrypt", "--input-file", encrypted, "--filename-encoding", encoding}, credentials...)
					decrypted, _, err := invoke(t, args, prompt, env)
					if err != nil {
						t.Fatal(err)
					}
					if decrypted != original {
						t.Fatalf("restored name %q, want %q", decrypted, original)
					}
					result, err := os.ReadFile(decrypted)
					if err != nil || !bytes.Equal(result, content) {
						t.Fatalf("contents differ: %v", err)
					}
				})
			}
		}
	}
}

func TestExplicitOutputAndFailures(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "input.txt")
	encrypted := filepath.Join(dir, "arbitrary-encrypted-name")
	output := filepath.Join(dir, "restored.txt")
	content := bytes.Repeat([]byte("test"), 40000)
	if err := os.WriteFile(plain, content, 0600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"RCLONE_CRYPT_PASSWORD": "right", "RCLONE_CRYPT_SALT": "salt"}
	if _, _, err := invoke(t, []string{"encrypt", "--input-file", plain, "--output-file", encrypted}, "", env); err != nil {
		t.Fatal(err)
	}
	if _, _, err := invoke(t, []string{"decrypt", "-i", encrypted, "-o", output}, "", env); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(output); err != nil || !bytes.Equal(data, content) {
		t.Fatal("explicit output failed")
	}
	for _, target := range []string{plain, output, encrypted} {
		before, _ := os.ReadFile(target)
		if _, _, err := invoke(t, []string{"decrypt", "-i", encrypted, "-o", target}, "", env); err == nil {
			t.Fatal("overwrote existing file")
		}
		after, _ := os.ReadFile(target)
		if !bytes.Equal(before, after) {
			t.Fatal("changed existing file")
		}
	}
	for _, credentials := range []map[string]string{
		{"RCLONE_CRYPT_PASSWORD": "wrong", "RCLONE_CRYPT_SALT": "salt"},
		{"RCLONE_CRYPT_PASSWORD": "right", "RCLONE_CRYPT_SALT": "wrong"},
	} {
		failed := filepath.Join(dir, "failed.txt")
		if _, _, err := invoke(t, []string{"decrypt", "-i", encrypted, "-o", failed}, "", credentials); err == nil {
			t.Fatal("accepted wrong credentials")
		}
		if _, err := os.Stat(failed); !os.IsNotExist(err) {
			t.Fatal("failed output remains")
		}
	}
	data, _ := os.ReadFile(encrypted)
	data[len(data)-1] ^= 1
	if err := os.WriteFile(encrypted, data, 0600); err != nil {
		t.Fatal(err)
	}
	failed := filepath.Join(dir, "tampered.txt")
	if _, _, err := invoke(t, []string{"decrypt", "-i", encrypted, "-o", failed}, "", env); err == nil {
		t.Fatal("accepted corrupted block")
	}
	if _, err := os.Stat(failed); !os.IsNotExist(err) {
		t.Fatal("partially decrypted output remains")
	}
}

func TestValidationAndPrecedence(t *testing.T) {
	for _, args := range [][]string{
		{"unknown"}, {"encrypt"}, {"decrypt", "positional"},
		{"encrypt", "-i", "missing", "--filename-encoding", "invalid"},
		{"encrypt", "-i", "missing"}, {"encrypt", "--unknown"},
		{"encrypt", "-i", t.TempDir()},
	} {
		if _, _, err := invoke(t, args, "", nil); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	for _, args := range [][]string{nil, {"--help"}, {"--version"}, {"encrypt", "--help"}} {
		if _, _, err := invoke(t, args, "", nil); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(input, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, prompt := range []string{"", "\n\n", "password\n"} {
		if _, _, err := invoke(t, []string{"encrypt", "-i", input}, prompt, nil); err == nil {
			t.Fatal("accepted missing credentials")
		}
	}
	env := map[string]string{"RCLONE_CRYPT_PASSWORD": "ignored", "RCLONE_CRYPT_SALT": "ignored"}
	encrypted, _, err := invoke(t, []string{"encrypt", "-i", input, "--password", "correct", "--salt="}, "", env)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(input); err != nil {
		t.Fatal(err)
	}
	if _, _, err := invoke(t, []string{"decrypt", "-i", encrypted}, "correct\r\n\r\n", nil); err != nil {
		t.Fatal(err)
	}
}

func TestUnsafeDecryptedName(t *testing.T) {
	cipher, err := newCipher("password", "", "base32")
	if err != nil {
		t.Fatal(err)
	}
	// Backslash and colon are legal Unix names but unsafe on Windows.
	for _, name := range []string{"..", "bad\\name", "C:escape"} {
		path := filepath.Join(t.TempDir(), cipher.EncryptFileName(name))
		stream, err := cipher.EncryptData(strings.NewReader("contents"))
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(stream)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		_, _, err = invoke(t, []string{"decrypt", "-i", path, "--password", "password", "--salt="}, "", nil)
		if err == nil || !strings.Contains(err.Error(), "safe local basename") {
			t.Fatalf("unsafe name %q: %v", name, err)
		}
	}
}

func TestRepositoryFixtures(t *testing.T) {
	for _, fixture := range []struct{ name, encoding, plainName string }{
		{"kr9tu4e1da4u3nifdd99g9tf5o", "base32", "TEST_FILE.txt"},
		{"Iyxcijgc9bp3o5Y0npW6xqUvwWNcc3MA4SadB0sR6cY", "base64", "TEST_FILE BASE64.txt"},
	} {
		t.Run(fixture.encoding, func(t *testing.T) {
			data, err := os.ReadFile(fixture.name)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			input := filepath.Join(dir, fixture.name)
			if err := os.WriteFile(input, data, 0600); err != nil {
				t.Fatal(err)
			}
			output, _, err := invoke(t, []string{"decrypt", "-i", input, "--filename-encoding", fixture.encoding, "--password", "Testpassword1", "--salt="}, "", nil)
			if err != nil {
				t.Fatal(err)
			}
			plain, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("fixture %s => %s; bytes=%d; SHA256=%x; prefix=%q", fixture.name, filepath.Base(output), len(plain), sha256.Sum256(plain), plain[:min(80, len(plain))])
			if filepath.Base(output) != fixture.plainName || len(plain) != 167 || fmt.Sprintf("%x", sha256.Sum256(plain)) != "f94d19d90613c129dc632adafd45b23fcca8c7fbcbdd149aa78990fe52821912" {
				t.Fatal("fixture filename or contents differ from known authentic fixture")
			}
		})
	}
}

func TestProcessPrompts(t *testing.T) {
	if os.Getenv("CLI_PROMPT_HELPER") == "1" {
		if err := run(os.Args[afterSeparator(os.Args):], os.Stdin, os.Stdout, os.Stderr, noEnv); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "process.txt")
	if err := os.WriteFile(input, []byte("prompted process"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, salt := range []string{"", "custom salt"} {
		encrypted := filepath.Join(dir, "encrypted"+fmt.Sprint(len(salt)))
		restored := filepath.Join(dir, "restored"+fmt.Sprint(len(salt)))
		for _, args := range [][]string{{"encrypt", "-i", input, "-o", encrypted}, {"decrypt", "-i", encrypted, "-o", restored}} {
			cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestProcessPrompts$", "--"}, args...)...)
			cmd.Env = append(os.Environ(), "CLI_PROMPT_HELPER=1")
			cmd.Stdin = strings.NewReader("Testpassword1\n" + salt + "\n")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("process: %v: %s", err, out)
			}
			if !strings.Contains(string(out), "Password:") || !strings.Contains(string(out), "Salt (optional") {
				t.Fatal("process did not prompt")
			}
		}
		data, _ := os.ReadFile(restored)
		if string(data) != "prompted process" {
			t.Fatal("process roundtrip failed")
		}
	}
}

func afterSeparator(args []string) int {
	for i, arg := range args {
		if arg == "--" {
			return i + 1
		}
	}
	return len(args)
}
