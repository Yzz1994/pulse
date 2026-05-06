package nodeenroll

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// RunCLI parses CLI args and executes enrollment. args excludes program and
// subcommand name (e.g. for `pulse-node enroll --server=... ...`, pass
// os.Args[2:]).
func RunCLI(args []string) error {
	fs := flag.NewFlagSet("enroll", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "用法: pulse-node enroll --server=<URL> --node-id=<ID> --token=<TOKEN> [选项]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "选项:")
		fs.PrintDefaults()
	}

	var (
		serverURL   = fs.String("server", "", "控制面 URL，例如 https://controlplane.example.com:8080")
		nodeID      = fs.String("node-id", "", "节点 ID")
		token       = fs.String("token", "", "一次性 enroll token")
		tokenFile   = fs.String("token-file", "", "从文件读取 token，'-' 表示 stdin")
		outDir      = fs.String("out", ".", "输出目录，写入 node_cert.pem / node_key.pem / node_ca.pem")
		insecure    = fs.Bool("insecure", false, "跳过控制面 TLS 校验（首次 enroll 必需）")
		fingerprint = fs.String("server-fingerprint", "", "控制面证书 SHA256 指纹（暂未实现，与 --insecure 互斥）")
	)

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *serverURL == "" {
		return errors.New("--server is required")
	}
	if *nodeID == "" {
		return errors.New("--node-id is required")
	}
	if *outDir == "" {
		return errors.New("--out cannot be empty")
	}

	if *token != "" && *tokenFile != "" {
		return errors.New("--token and --token-file are mutually exclusive")
	}
	if *token == "" && *tokenFile == "" {
		return errors.New("either --token or --token-file is required")
	}

	if *insecure && *fingerprint != "" {
		return errors.New("--insecure and --server-fingerprint are mutually exclusive")
	}
	if *fingerprint != "" {
		// TODO: implement fingerprint pinning.
		return errors.New("--server-fingerprint is not yet implemented; use --insecure for now")
	}
	if !*insecure {
		return errors.New("--insecure must be set explicitly for first-time enrollment (fingerprint pinning is not yet implemented)")
	}

	resolvedToken := *token
	if *tokenFile != "" {
		t, err := readToken(*tokenFile)
		if err != nil {
			return fmt.Errorf("read token: %w", err)
		}
		resolvedToken = t
	}
	resolvedToken = strings.TrimSpace(resolvedToken)
	if resolvedToken == "" {
		return errors.New("token is empty")
	}

	fmt.Fprintln(os.Stdout, "✔ Generated RSA 4096 keypair")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: *insecure}, // #nosec G402
		},
	}

	res, err := Run(ctx, Request{
		ServerURL:  *serverURL,
		NodeID:     *nodeID,
		Token:      resolvedToken,
		OutDir:     *outDir,
		HTTPClient: client,
		Insecure:   *insecure,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "✔ Submitted CSR to %s\n", *serverURL)
	fmt.Fprintf(os.Stdout, "✔ Saved %s (0644)\n", res.CertPath)
	fmt.Fprintf(os.Stdout, "✔ Saved %s (0600)\n", res.KeyPath)
	fmt.Fprintf(os.Stdout, "✔ Saved %s  (0644)\n", res.CAPath)
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintf(os.Stdout, "GRPC server: %s\n", res.GRPCURL)
	return nil
}

func readToken(path string) (string, error) {
	if path == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
