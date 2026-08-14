package update

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
)

// SignPubKey 是 Fly-Lang 发布产物的 ed25519 公钥（base64）。
// 生成：go run ./tools/sign genkey；对应私钥存 GitHub Actions secret SIGN_PRIVATE_KEY，
// release.yml 用 tools/sign 对每个产物签名并上传 <产物>.sig。
// 轮换公钥：重新生成密钥对后更新此常量（客户端与产物绑定同一公钥）。
// var 而非 const：允许单测替换；生产二进制中无法从外部修改（internal 包）。
var SignPubKey = "YXpFqzZ8daPtFKwvpKTNoCkc8DIT2cG52cH3tvro9Go="

// VerifySigned 校验 data 是否由 SignPubKey 对应私钥签名（sig 为 64 字节原始 ed25519 签名）。
func VerifySigned(data, sig []byte) error {
	pubRaw, err := base64.StdEncoding.DecodeString(SignPubKey)
	if err != nil {
		return fmt.Errorf("内嵌公钥非法: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("签名长度 %d 非法（应为 %d）", len(sig), ed25519.SignatureSize)
	}
	if !ed25519.Verify(pubRaw, data, sig) {
		return fmt.Errorf("签名验证失败：产物可能被篡改或来源不可信")
	}
	return nil
}
