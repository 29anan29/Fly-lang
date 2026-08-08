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
	Name string
	URL  string
}

type Release struct {
	TagName string `json:"tag_name"`
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
			return &Asset{Name: a.Name, URL: a.BrowserDownloadURL}, nil
		}
	}
	return nil, fmt.Errorf("当前平台 %s/%s 暂无更新包（需要 %s）", goos, goarch, want)
}

func (u *Updater) IsOutdated(latest string) bool {
	cur := version.TrimTag(u.Current)
	rel := version.TrimTag(latest)
	if cur == "dev" || cur == "" {
		return true
	}
	return cur != rel
}

func (u *Updater) Install(a *Asset) error {
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

	exe, err := u.executable()
	if err != nil {
		return err
	}
	exeReal, err := filepath.EvalSymlinks(exe)
	if err != nil {
		exeReal = exe
	}
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
	if err := os.WriteFile(newPath, data, 0755); err != nil {
		return fmt.Errorf("写入新版本失败: %w", err)
	}
	if err := os.Rename(newPath, exeReal); err != nil {
		os.Remove(newPath)
		if runtime.GOOS == "windows" {
			return errors.New("Windows 下无法替换正在运行的进程，请关闭后手动覆盖: " + exeReal)
		}
		return fmt.Errorf("替换可执行文件失败: %w", err)
	}
	return nil
}

func (u *Updater) executable() (string, error) {
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
