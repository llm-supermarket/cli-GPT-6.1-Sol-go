# cli-GPT-6.1-Sol-go

A standalone Go CLI for encrypting and decrypting individual local files using
rclone's crypt format. It encrypts file contents and, by default, the basename.
It uses rclone's own crypt implementation, not a separate approximation.
No Go, Python, Node.js, rclone installation, or framework runtime is needed to
use the released binaries.

## Installation

Packages become available after the first implementation PR is merged to `main`
and the Release workflow finishes. Subsequent source changes on `main`
automatically publish versioned GitHub releases and update this repository's
Scoop bucket and Homebrew tap with actual SHA-256 archive checksums.

### Windows: Scoop

```bash
scoop bucket add llm-supermarket https://github.com/llm-supermarket/cli-GPT-6.1-Sol-go
scoop install llm-supermarket/cli-GPT-6.1-Sol-go
cli-GPT-6.1-Sol-go --version
```

### macOS / Linux: Homebrew

```bash
brew tap llm-supermarket/cli-gpt-6.1-sol-go https://github.com/llm-supermarket/cli-GPT-6.1-Sol-go
brew install llm-supermarket/cli-gpt-6.1-sol-go/cli-gpt-6.1-sol-go
cli-GPT-6.1-Sol-go --version
```

Homebrew normalizes formula identifiers to lowercase. Its package identifier is
the repository name lowercased, `cli-gpt-6.1-sol-go`; the installed executable and
Scoop package preserve the exact repository name, `cli-GPT-6.1-Sol-go`.

### GitHub Releases

Download the archive for your OS and CPU from
[GitHub Releases](https://github.com/llm-supermarket/cli-GPT-6.1-Sol-go/releases).
Windows archives are ZIP files; macOS (`darwin`) and Linux archives are tar.gz.
Both AMD64 (x86-64) and ARM64 are supported on all three operating systems.
Extract the executable into a directory on your `PATH`. Verify the downloaded
archive against the release's `SHA256SUMS` before running it, for example:

```bash
# Download the appropriate archive and SHA256SUMS into the current directory.
sha256sum --ignore-missing -c SHA256SUMS     # Linux
shasum -a 256 cli-GPT-6.1-Sol-go_v0.1.1_darwin_arm64.tar.gz  # macOS: compare to SHA256SUMS
```

Version numbers above are examples; use the version and architecture you downloaded.

## Usage

```bash
cli-GPT-6.1-Sol-go encrypt -i FILE [-o FILE] [OPTIONS]
cli-GPT-6.1-Sol-go decrypt -i FILE [-o FILE] [OPTIONS]
```

| Option | Meaning |
| --- | --- |
| `-i`, `--input-file` | Required input file; positional file arguments are rejected. |
| `-o`, `--output-file` | Optional exact output path. Without it, transform the input basename and write beside the input. |
| `--password` | Plaintext password. Prints a security warning; prefer the prompt or environment variable. |
| `--salt` | Optional plaintext salt (rclone's `password2`). `--salt ''` explicitly selects the default rclone salt. |
| `--filename-encoding` | `base32` (default), `base64`, or `base32768`. Select the same encoding when decrypting. |
| `-h`, `--help` | Show help (also supported after `encrypt` / `decrypt`). |
| `--version` | Show the installed version. |

Without `--password` or `RCLONE_CRYPT_PASSWORD`, the CLI prompts for a password.
Without `--salt` or `RCLONE_CRYPT_SALT`, it prompts for an optional salt; press
Enter to use rclone's built-in default. Both prompts hide input on a terminal.
Piped input is supported for automation but cannot conceal input at its source.
Precedence is explicit flag, then environment variable, then prompt. An empty
password is rejected. An empty salt uses the rclone default, not a random salt.
The same password and salt must be used for both operations; neither is stored.

### Interactive Encryption

```bash
cli-GPT-6.1-Sol-go encrypt -i 'notes.txt'
# Password: (hidden)
# Salt (optional; Enter for rclone default): (hidden)
# Prints the created encrypted path.

cli-GPT-6.1-Sol-go decrypt -i 'ENCRYPTED_FILENAME'
# Restores notes.txt if it does not already exist.
```

### Environment Variables and Custom Encoding

These are Bash examples. Read secrets interactively rather than putting literal
secrets into command history:

```bash
read -r -s -p 'Password: ' RCLONE_CRYPT_PASSWORD; printf '\n'
export RCLONE_CRYPT_PASSWORD
export RCLONE_CRYPT_SALT=''  # Explicitly use the default salt, without prompting.

encrypted=$(cli-GPT-6.1-Sol-go encrypt --input-file 'notes.txt' --filename-encoding base64)
cli-GPT-6.1-Sol-go decrypt --input-file "$encrypted" --output-file 'restored-notes.txt' --filename-encoding base64
cmp 'notes.txt' 'restored-notes.txt'
unset RCLONE_CRYPT_PASSWORD RCLONE_CRYPT_SALT
```

For a custom salt, read and export `RCLONE_CRYPT_SALT` in the same way as the password.

### Password Flag (Unsafe)

```bash
# Demonstration only. Prefer the hidden prompt or direct environment-variable use.
cli-GPT-6.1-Sol-go encrypt -i 'notes.txt' -o 'encrypted.bin' --password 'example-only' --salt ''
cli-GPT-6.1-Sol-go decrypt -i 'encrypted.bin' -o 'restored.txt' --password 'example-only' --salt ''
```

`--password` exposes your password in process arguments and potentially terminal
history, logs, and monitoring tools. Even `--password "$MY_PASSWORD"` exposes the
expanded secret in process arguments. Set `RCLONE_CRYPT_PASSWORD` directly instead.
If you use a literal password, remove that history entry using your shell's
history facilities (in Bash, `history -d ENTRY_NUMBER` followed by `history -w`).
History deletion cannot undo logs or exposure to another process. Environment
variables are safer than arguments/history but still accessible to privileged
processes; the hidden prompt is preferable. Treat custom salts as secrets too.

## Compatibility and Safety

- Uses standard rclone filename encryption (AES-EME with PKCS#7 padding), scrypt
  key derivation, and authenticated XSalsa20-Poly1305 content blocks.
- Base32 is rclone's lowercase, unpadded base32hex, not ordinary RFC base32.
  Base64 is unpadded URL-safe base64 and needs a case-sensitive filesystem.
  Base32768 uses Unicode filenames; filesystem support varies.
- Only the basename is transformed; directory names are left unchanged.
  An explicit `-o` is used verbatim and bypasses filename transformation, useful
  for renamed encrypted files or content-only recovery.
- Input is preserved. Existing output files, including the input itself, are
  never overwritten. Output parents must already exist. Successful operations
  print the output path to stdout; prompts, warnings, and errors go to stderr.
- Files are streamed in bounded memory. Outputs request owner-only permissions
  on Unix; Windows access is governed by inherited filesystem ACLs.
- Failed authentication or write errors remove the partial output. During an
  operation the output may be partial; interruption or abrupt termination can
  leave it behind. Decrypted basenames are checked against path traversal.
- Rclone's format authenticates content blocks but not filenames or file length.
  Whole-block truncation and empty-file headers cannot always be detected by the
  format. Do not treat it as a signed proof of provenance or completeness.

## Included Fixtures

With password `Testpassword1` and default salt, the supplied encrypted files
restore the following names. Both are 167 bytes, begin with
`## This is a test file`, and have plaintext SHA-256
`f94d19d90613c129dc632adafd45b23fcca8c7fbcbdd149aa78990fe52821912`.

```bash
export RCLONE_CRYPT_PASSWORD='Testpassword1'  # Public test credential only.
export RCLONE_CRYPT_SALT=''
cli-GPT-6.1-Sol-go decrypt -i 'kr9tu4e1da4u3nifdd99g9tf5o'
# TEST_FILE.txt
cli-GPT-6.1-Sol-go decrypt -i 'Iyxcijgc9bp3o5Y0npW6xqUvwWNcc3MA4SadB0sR6cY' --filename-encoding base64
# TEST_FILE BASE64.txt
sha256sum 'TEST_FILE.txt' 'TEST_FILE BASE64.txt'
unset RCLONE_CRYPT_PASSWORD RCLONE_CRYPT_SALT
```

## Development

Go 1.25 or later is needed only to build from source:

```bash
go mod download
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -o cli-GPT-6.1-Sol-go .
```

Tests cover encryption/decryption with default and custom salts, password flags,
environment variables, prompts including piped subprocess input, all filename
encodings, known authentic fixtures, multiblock and empty files, credential
precedence, invalid arguments, overwrite protection, unsafe decrypted names,
and removal of partial output after corruption or wrong credentials. Terminal
echo hiding uses `golang.org/x/term`; automated prompt tests use pipes rather
than a real terminal.

CI runs tests, vet, and builds on Windows, macOS, and Linux. The Release workflow
requires `contents: write` and permission to push generated package metadata to
`main`. See [release packaging](scripts/packaging/README.md) for details.

## Uninstall

```bash
scoop uninstall cli-GPT-6.1-Sol-go
scoop bucket rm llm-supermarket

# Or on macOS/Linux:
brew uninstall cli-gpt-6.1-sol-go
brew untap llm-supermarket/cli-gpt-6.1-sol-go
```
