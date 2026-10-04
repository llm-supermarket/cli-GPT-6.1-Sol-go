package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rclone/rclone/backend/crypt"
	"github.com/rclone/rclone/fs/config/configmap"
	"github.com/rclone/rclone/fs/config/obscure"
	"golang.org/x/term"
)

const appName = "cli-GPT-6.1-Sol-go"

var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.LookupEnv); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) (string, bool)) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintf(stdout, "Usage: %s <encrypt|decrypt> -i FILE [-o FILE] [--filename-encoding base32|base64|base32768]\n\nPassword and optional salt are prompted for unless supplied via RCLONE_CRYPT_PASSWORD\nand RCLONE_CRYPT_SALT or --password/--salt. Use --salt '' for the default rclone salt.\nUse %s <encrypt|decrypt> --help for all options.\n", appName, appName)
		return nil
	}
	if args[0] == "--version" {
		fmt.Fprintln(stdout, appName, version)
		return nil
	}
	mode := args[0]
	if mode != "encrypt" && mode != "decrypt" {
		return fmt.Errorf("unknown command %q; expected encrypt or decrypt", mode)
	}
	flags := flag.NewFlagSet(mode, flag.ContinueOnError)
	flags.SetOutput(stderr)
	var input, output, password, salt, encoding string
	flags.StringVar(&input, "i", "", "input file (required)")
	flags.StringVar(&input, "input-file", "", "input file (required)")
	flags.StringVar(&output, "o", "", "output file (default: transformed basename beside input)")
	flags.StringVar(&output, "output-file", "", "output file (default: transformed basename beside input)")
	flags.StringVar(&password, "password", "", "password (unsafe: exposed in process arguments/history; prefer environment or prompt)")
	flags.StringVar(&salt, "salt", "", "optional salt; empty string selects rclone's built-in salt")
	flags.StringVar(&encoding, "filename-encoding", "base32", "filename encoding: base32, base64, base32768")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if input == "" || flags.NArg() != 0 {
		return errors.New("specify the input file with -i or --input-file; positional files are not accepted")
	}
	encoding = strings.ToLower(encoding)
	if _, err := crypt.NewNameEncoding(encoding); err != nil {
		return err
	}
	info, err := os.Stat(input)
	if err != nil {
		return fmt.Errorf("input file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("input must be a regular file")
	}
	provided := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { provided[f.Name] = true })
	if provided["password"] {
		fmt.Fprintln(stderr, "Warning: --password exposes secrets in process arguments and terminal history. Prefer RCLONE_CRYPT_PASSWORD or the hidden prompt. An env var expanded into --password still exposes arguments; remove the terminal history entry if used.")
	}
	reader := bufio.NewReader(stdin)
	prompt := func(label string) (string, error) {
		fmt.Fprint(stderr, label)
		if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
			secret, err := term.ReadPassword(int(f.Fd()))
			fmt.Fprintln(stderr)
			defer clear(secret)
			return string(secret), err
		}
		line, err := reader.ReadString('\n')
		if err != nil && !(errors.Is(err, io.EOF) && len(line) > 0) {
			return "", fmt.Errorf("reading prompt: %w", err)
		}
		return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
	}
	if !provided["password"] {
		if value, ok := getenv("RCLONE_CRYPT_PASSWORD"); ok {
			password = value
		} else if password, err = prompt("Password: "); err != nil {
			return err
		}
	}
	if password == "" {
		return errors.New("password must not be empty")
	}
	if !provided["salt"] {
		if value, ok := getenv("RCLONE_CRYPT_SALT"); ok {
			salt = value
		} else if salt, err = prompt("Salt (optional; Enter for rclone default): "); err != nil {
			return err
		}
	}
	cipher, err := newCipher(password, salt, encoding)
	if err != nil {
		return err
	}
	if output == "" {
		name := filepath.Base(input)
		if mode == "encrypt" {
			// EME accepts at most 128 AES blocks, including padding.
			if len(name) > 2047 {
				return errors.New("input filename is too long to encrypt")
			}
			name = cipher.EncryptFileName(name)
		} else {
			name, err = cipher.DecryptFileName(name)
			if err != nil {
				return fmt.Errorf("decrypt filename (check password, salt, and encoding): %w", err)
			}
		}
		// Never let a decrypted name escape the input directory, on any OS.
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00:") || !filepath.IsLocal(name) {
			return errors.New("transformed filename is not a safe local basename; specify -o explicitly")
		}
		output = filepath.Join(filepath.Dir(input), name)
	}
	if err := transformFile(mode, input, output, cipher); err != nil {
		return err
	}
	fmt.Fprintln(stdout, output)
	return nil
}

func newCipher(password, salt, encoding string) (*crypt.Cipher, error) {
	p, err := obscure.Obscure(password)
	if err != nil {
		return nil, err
	}
	s, err := obscure.Obscure(salt)
	if err != nil {
		return nil, err
	}
	return crypt.NewCipher(configmap.Simple{
		"password": p, "password2": s,
		"filename_encryption": "standard", "filename_encoding": encoding,
		"directory_name_encryption": "false", "suffix": ".bin",
	})
}

func transformFile(mode, input, output string, cipher *crypt.Cipher) error {
	in, err := os.Open(input)
	if err != nil {
		return err
	}
	defer in.Close()
	var stream io.Reader
	if mode == "encrypt" {
		stream, err = cipher.EncryptData(in)
	} else {
		var decrypted io.ReadCloser
		decrypted, err = cipher.DecryptData(in)
		if err == nil {
			defer decrypted.Close()
			stream = decrypted
		}
	}
	if err != nil {
		return fmt.Errorf("%s contents: %w", mode, err)
	}
	// Exclusive creation protects the input, existing files, and symlink targets.
	out, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create output (existing files are never overwritten): %w", err)
	}
	complete := false
	defer func() {
		out.Close()
		if !complete {
			os.Remove(output)
		}
	}()
	if _, err := io.Copy(out, stream); err != nil {
		return fmt.Errorf("%s contents: %w", mode, err)
	}
	if err := out.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}
