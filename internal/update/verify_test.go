package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

// 用测试密钥对验证 VerifySigned：正签通过、篡改拒绝、长度非法拒绝。
func TestVerifySigned(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	orig := SignPubKey
	SignPubKey = pubKeyB64(t, priv)
	defer func() { SignPubKey = orig }()

	data := []byte("fly-linux-amd64.tar.gz fake content")
	sig := ed25519.Sign(priv, data)

	if err := VerifySigned(data, sig); err != nil {
		t.Fatalf("正签应通过: %v", err)
	}
	tampered := append([]byte{}, data...)
	tampered[0] ^= 0xFF
	if err := VerifySigned(tampered, sig); err == nil {
		t.Fatal("篡改数据应拒绝")
	}
	wrongSig := make([]byte, ed25519.SignatureSize)
	if err := VerifySigned(data, wrongSig); err == nil {
		t.Fatal("错误签名应拒绝")
	}
	if err := VerifySigned(data, data[:16]); err == nil {
		t.Fatal("非法长度签名应拒绝")
	}
}

// 使用与 SignPubKey 不同的公钥时应拒绝（模拟密钥轮换/来源不符）。
func TestVerifySignedWrongKey(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("payload")
	sig := ed25519.Sign(priv, data)
	if err := VerifySigned(data, sig); err == nil {
		t.Fatal("非内嵌公钥对应的签名应拒绝")
	}
}

func pubKeyB64(t *testing.T, priv ed25519.PrivateKey) string {
	t.Helper()
	return base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
}

// AssetFor 应附带 .sig 资产；缺失时 SigURL 为空。
func TestAssetForSigURL(t *testing.T) {
	rel := &Release{
		Assets: []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		}{
			{Name: "fly-linux-amd64.tar.gz", BrowserDownloadURL: "https://x/fly-linux-amd64.tar.gz"},
			{Name: "fly-linux-amd64.tar.gz.sig", BrowserDownloadURL: "https://x/fly-linux-amd64.tar.gz.sig"},
		},
	}
	u := &Updater{}
	a, err := u.AssetFor("linux", "amd64", rel)
	if err != nil {
		t.Fatal(err)
	}
	if a.SigURL != "https://x/fly-linux-amd64.tar.gz.sig" {
		t.Fatalf("SigURL = %q", a.SigURL)
	}

	relNoSig := &Release{Assets: []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	}{{Name: "fly-linux-amd64.tar.gz", BrowserDownloadURL: "https://x/fly-linux-amd64.tar.gz"}}}
	a, err = u.AssetFor("linux", "amd64", relNoSig)
	if err != nil {
		t.Fatal(err)
	}
	if a.SigURL != "" {
		t.Fatalf("无 .sig 资产时 SigURL 应为空，实际 %q", a.SigURL)
	}
}
