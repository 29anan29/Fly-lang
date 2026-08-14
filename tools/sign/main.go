// tools/sign 用 ed25519 对发布产物签名（零第三方依赖，Go 标准库）。
//
// 用法：
//
//	go run ./tools/sign genkey            # 生成密钥对，输出公钥 base64 与私钥 PKCS8 PEM
//	go run ./tools/sign --key <file> dist/*.tar.gz   # 对每个文件输出 <file>.sig（64 字节原始 ed25519 签名）
//
// 私钥来源：--key 文件，或环境变量 SIGN_PRIVATE_KEY（PKCS8 PEM）。CI 中私钥存 GitHub Actions secret。
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	fs := flag.NewFlagSet("fly-sign", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: sign [genkey | pubkey | --key <priv.pem> <file>...]\n")
		fs.PrintDefaults()
	}
	keyPath := fs.String("key", "", "ed25519 私钥 PEM 文件路径（或设 SIGN_PRIVATE_KEY 环境变量）")
	if err := fs.Parse(os.Args[1:]); err != nil {
		fs.Usage()
		os.Exit(2)
	}
	args := fs.Args()
	if len(args) == 0 {
		fs.Usage()
		os.Exit(2)
	}
	switch args[0] {
	case "genkey":
		genkey()
	case "pubkey":
		priv, err := loadKey(*keyPath)
		if err != nil {
			fatal(err)
		}
		fmt.Println(base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey)))
	default:
		priv, err := loadKey(*keyPath)
		if err != nil {
			fatal(err)
		}
		for _, f := range args {
			if err := signFile(priv, f); err != nil {
				fatal(err)
			}
		}
	}
}

func genkey() {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fatal(err)
	}
	pub := priv.Public().(ed25519.PublicKey)
	fmt.Fprintf(os.Stderr, "公钥 base64（请内嵌到 internal/update/verify.go 的 SignPubKey）：\n")
	fmt.Fprintf(os.Stderr, "  %s\n", base64.StdEncoding.EncodeToString(pub))
	fmt.Fprintf(os.Stderr, "私钥 PEM（请存入 GitHub Actions secret SIGN_PRIVATE_KEY，勿提交仓库）：\n")
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		fatal(err)
	}
	os.Stdout.Write(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func loadKey(path string) (ed25519.PrivateKey, error) {
	var pemBytes []byte
	switch {
	case path != "":
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		pemBytes = b
	case os.Getenv("SIGN_PRIVATE_KEY") != "":
		pemBytes = []byte(os.Getenv("SIGN_PRIVATE_KEY"))
	default:
		return nil, fmt.Errorf("未找到私钥：请用 --key 指定或设 SIGN_PRIVATE_KEY")
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("私钥不是 PKCS8 PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("解析 PKCS8 私钥失败: %w", err)
	}
	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("私钥不是 ed25519（实际 %T）", key)
	}
	return priv, nil
}

func signFile(priv ed25519.PrivateKey, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sig := ed25519.Sign(priv, data)
	sigPath := path + ".sig"
	if err := os.WriteFile(sigPath, sig, 0644); err != nil {
		return err
	}
	rel, _ := filepath.Rel(".", path)
	fmt.Printf("%s <- %s\n", sigPath, rel)
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "sign:", err)
	os.Exit(1)
}
