package nodeenroll

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCLI_TokenAndTokenFileMutuallyExclusive(t *testing.T) {
	err := RunCLI([]string{
		"--server", "https://x",
		"--node-id", "n",
		"--token", "abc",
		"--token-file", "/tmp/x",
		"--insecure",
	})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunCLI_MissingToken(t *testing.T) {
	err := RunCLI([]string{
		"--server", "https://x",
		"--node-id", "n",
		"--insecure",
	})
	if err == nil || !strings.Contains(err.Error(), "--token") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunCLI_MissingServer(t *testing.T) {
	err := RunCLI([]string{
		"--node-id", "n",
		"--token", "t",
		"--insecure",
	})
	if err == nil || !strings.Contains(err.Error(), "--server") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunCLI_RequiresInsecure(t *testing.T) {
	err := RunCLI([]string{
		"--server", "https://x",
		"--node-id", "n",
		"--token", "t",
	})
	if err == nil || !strings.Contains(err.Error(), "--insecure") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunCLI_FingerprintNotImplemented(t *testing.T) {
	err := RunCLI([]string{
		"--server", "https://x",
		"--node-id", "n",
		"--token", "t",
		"--server-fingerprint", "deadbeef",
	})
	if err == nil || !strings.Contains(err.Error(), "not yet implemented") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunCLI_InsecureFingerprintExclusive(t *testing.T) {
	err := RunCLI([]string{
		"--server", "https://x",
		"--node-id", "n",
		"--token", "t",
		"--insecure",
		"--server-fingerprint", "deadbeef",
	})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadToken_FromFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tok")
	if err := os.WriteFile(p, []byte("  hello-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readToken(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello-token" {
		t.Errorf("got %q", got)
	}
}

func TestReadToken_FromStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = orig }()

	go func() {
		_, _ = w.Write([]byte("stdin-token\n"))
		_ = w.Close()
	}()

	got, err := readToken("-")
	if err != nil {
		t.Fatal(err)
	}
	if got != "stdin-token" {
		t.Errorf("got %q", got)
	}
}
