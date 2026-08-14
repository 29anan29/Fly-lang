package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"flylang/internal/version"
)

type Asset struct {
	Name   string
	URL    string
	SigURL string
}

type Release struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

type Updater struct {
	Client   *http.Client
	Repo     string
	BaseURL  string
	Current  string
	ExecPath string
	// Insecure 跳过产物签名验证（危险：仅用于无签名来源/测试服务器）。
	Insecure bool
}

func New() *Updater {
	return &Updater{
		Client:  &http.Client{Timeout: 60 * time.Second},
		Repo:    version.Repo,
		BaseURL: "https://api.github.com",
		Current: version.Version,
	}
}

func (u *Updater) SetProxy(proxy string) error {
	tr := &http.Transport{}
	switch {
	case strings.HasPrefix(proxy, "http://"), strings.HasPrefix(proxy, "https://"):
		pu, err := url.Parse(proxy)
		if err != nil {
			return fmt.Errorf("代理地址无效: %w", err)
		}
		tr.Proxy = http.ProxyURL(pu)
	case isSocksProxy(proxy):
		tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialSocks5(proxy, addr, 30*time.Second)
		}
	default:
		return fmt.Errorf("不支持的代理协议 %q（支持 http://、https://、socks5://）", proxy)
	}
	u.Client.Transport = tr
	return nil
}

func (u *Updater) Latest() (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", u.BaseURL, u.Repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "fly-lang/"+version.String())
	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("检查更新失败: HTTP %d", resp.StatusCode)
	}
	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func (u *Updater) AssetFor(goos, goarch string, rel *Release) (*Asset, error) {
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	want := fmt.Sprintf("fly-%s-%s.%s", goos, goarch, ext)
	for _, a := range rel.Assets {
		if a.Name == want {
			return &Asset{Name: a.Name, URL: a.BrowserDownloadURL, SigURL: u.sigURLFor(rel, want)}, nil
		}
	}
	return nil, fmt.Errorf("当前平台 %s/%s 暂无更新包（需要 %s）", goos, goarch, want)
}

func (u *Updater) sigURLFor(rel *Release, name string) string {
	sigName := name + ".sig"
	for _, a := range rel.Assets {
		if a.Name == sigName {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

func (u *Updater) IsOutdated(latest string) bool {
	cur := version.TrimTag(u.Current)
	rel := version.TrimTag(latest)
	if cur == "dev" || cur == "" {
		return true
	}
	return cur != rel
}

// verifyAsset 下载产物对应的 .sig 并验证签名；缺少签名文件即拒绝安装（防降级/篡改）。
func (u *Updater) verifyAsset(a *Asset, artifactPath string, log func(string)) error {
	if a.SigURL == "" {
		return fmt.Errorf("缺少签名文件 %s.sig：为防篡改已拒绝安装，请从 GitHub Releases 手动下载", a.Name)
	}
	log("下载签名 " + a.Name + ".sig")
	sigReq, err := http.NewRequest(http.MethodGet, a.SigURL, nil)
	if err != nil {
		return err
	}
	sigReq.Header.Set("User-Agent", "fly-lang/"+version.String())
	sigResp, err := u.Client.Do(sigReq)
	if err != nil {
		return err
	}
	defer sigResp.Body.Close()
	if sigResp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载签名失败: HTTP %d", sigResp.StatusCode)
	}
	sig, err := io.ReadAll(sigResp.Body)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		return err
	}
	log("验证签名")
	if err := VerifySigned(data, sig); err != nil {
		return fmt.Errorf("安全校验失败：%v", err)
	}
	return nil
}

func (u *Updater) Install(a *Asset) error {
	return u.InstallVerbose(a, nil)
}

func (u *Updater) InstallVerbose(a *Asset, logf func(step string)) error {
	log := func(s string) {
		if logf != nil {
			logf(s)
		}
	}
	log("下载 " + a.Name)
	req, err := http.NewRequest(http.MethodGet, a.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "fly-lang/"+version.String())
	resp, err := u.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载 %s 失败: HTTP %d", a.Name, resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "fly-dl-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	if !u.Insecure {
		if err := u.verifyAsset(a, tmpPath, log); err != nil {
			return err
		}
	} else {
		log("跳过签名验证（--insecure，不推荐）")
	}

	exe, err := u.Executable()
	if err != nil {
		return err
	}
	exeReal, err := filepath.EvalSymlinks(exe)
	if err != nil {
		exeReal = exe
	}
	log("解包校验")
	data, err := extractBinary(tmpPath, a.Name)
	if err != nil {
		return err
	}
	binName := filepath.Base(exeReal)
	if runtime.GOOS == "windows" && !strings.HasSuffix(binName, ".exe") {
		binName += ".exe"
	}
	dir := filepath.Dir(exeReal)
	newPath := filepath.Join(dir, "."+binName+".new")
	log("写入 " + newPath)
	if err := os.WriteFile(newPath, data, 0755); err != nil {
		return fmt.Errorf("写入新版本失败: %w", err)
	}
	log("原子替换 " + exeReal)
	if err := os.Rename(newPath, exeReal); err != nil {
		os.Remove(newPath)
		if runtime.GOOS == "windows" {
			return errors.New("Windows 下无法替换正在运行的进程，请关闭后手动覆盖: " + exeReal)
		}
		return fmt.Errorf("替换可执行文件失败: %w", err)
	}
	return nil
}

func (u *Updater) CheckWritable(dir string) error {
	probe := filepath.Join(dir, fmt.Sprintf(".fly-wtest-%d", os.Getpid()))
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("安装目录 %s 不可写: %v", dir, err)
	}
	f.Close()
	os.Remove(probe)
	return nil
}

func (u *Updater) Executable() (string, error) {
	if u.ExecPath != "" {
		return u.ExecPath, nil
	}
	return os.Executable()
}

func extractBinary(archivePath, name string) ([]byte, error) {
	if strings.HasSuffix(name, ".zip") {
		zr, err := zip.OpenReader(archivePath)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		for _, f := range zr.File {
			if isBinaryName(f.Name) {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				data, err := io.ReadAll(rc)
				rc.Close()
				return data, err
			}
		}
		return nil, errors.New("安装包内未找到可执行文件")
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag == tar.TypeReg && isBinaryName(hdr.Name) {
			return io.ReadAll(tr)
		}
	}
	return nil, errors.New("安装包内未找到可执行文件")
}

func isBinaryName(name string) bool {
	base := filepath.Base(name)
	if base == "fly" || base == "fly.exe" {
		return true
	}
	return false
}
