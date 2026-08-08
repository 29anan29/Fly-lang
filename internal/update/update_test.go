package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func runSocks5Server(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 16)
				if _, err := io.ReadFull(c, buf[:2]); err != nil {
					return
				}
				n := int(buf[1])
				if _, err := io.ReadFull(c, buf[:n]); err != nil {
					return
				}
				if _, err := c.Write([]byte{0x05, 0x00}); err != nil {
					return
				}
				head := make([]byte, 4)
				if _, err := io.ReadFull(c, head); err != nil {
					return
				}
				var host string
				var port []byte
				switch head[3] {
				case 0x01:
					ip := make([]byte, 4)
					if _, err := io.ReadFull(c, ip); err != nil {
						return
					}
					host = net.IP(ip).String()
					port = make([]byte, 2)
					io.ReadFull(c, port)
				case 0x04:
					ip := make([]byte, 16)
					if _, err := io.ReadFull(c, ip); err != nil {
						return
					}
					host = net.IP(ip).String()
					port = make([]byte, 2)
					io.ReadFull(c, port)
				default:
					l := make([]byte, 1)
					if _, err := io.ReadFull(c, l); err != nil {
						return
					}
					dom := make([]byte, int(l[0]))
					if _, err := io.ReadFull(c, dom); err != nil {
						return
					}
					host = string(dom)
					port = make([]byte, 2)
					io.ReadFull(c, port)
				}
				target, err := net.Dial("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", int(port[0])<<8|int(port[1]))))
				if err != nil {
					return
				}
				defer target.Close()
				if _, err := c.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
					return
				}
				go io.Copy(target, c)
				io.Copy(c, target)
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func TestDialSocks5(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	echoDone := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		data := make([]byte, 4)
		if _, err := io.ReadFull(c, data); err != nil {
			return
		}
		echoDone <- string(data)
	}()

	addr := runSocks5Server(t)
	conn, err := dialSocks5("socks5://user:pass@"+addr, ln.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatalf("dial 失败: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	select {
	case got := <-echoDone:
		if got != "ping" {
			t.Fatalf("回显数据不匹配: %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("数据未到达目标服务器")
	}
}

func TestDialSocks5NoAuth(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	addr := runSocks5Server(t)
	conn, err := dialSocks5("socks5://"+addr, ln.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatalf("无认证 dial 失败: %v", err)
	}
	conn.Close()
}

func TestDialSocks5BadProxy(t *testing.T) {
	conn, err := dialSocks5("socks5://127.0.0.1:1", "127.0.0.1:80", time.Second)
	if err == nil {
		conn.Close()
		t.Fatal("期望连接失败，实际成功")
	}
}

func TestCheckAndInstall(t *testing.T) {
	var bin bytes.Buffer
	gw := gzip.NewWriter(&bin)
	tw := tar.NewWriter(gw)
	content := []byte("#!/bin/sh\necho new-version\n")
	if err := tw.WriteHeader(&tar.Header{Name: "fly", Mode: 0755, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	tw.Write(content)
	tw.Close()
	gw.Close()

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases/latest") {
			rel := Release{TagName: "v1.2.3"}
			rel.Assets = append(rel.Assets, struct {
				Name               string `json:"name"`
				BrowserDownloadURL string `json:"browser_download_url"`
			}{Name: "fly-linux-amd64.tar.gz", BrowserDownloadURL: srv.URL + "/dl/fly-linux-amd64.tar.gz"})
			json.NewEncoder(w).Encode(rel)
			return
		}
		w.Write(bin.Bytes())
	}))
	defer srv.Close()

	dir := t.TempDir()
	exe := filepath.Join(dir, "fly")
	os.WriteFile(exe, []byte("old"), 0755)

	u := New()
	u.BaseURL = srv.URL
	u.ExecPath = exe
	u.Current = "1.0.0"
	rel, err := u.Latest()
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if rel.TagName != "v1.2.3" {
		t.Fatalf("tag 不匹配: %s", rel.TagName)
	}
	if !u.IsOutdated(rel.TagName) {
		t.Fatal("1.0.0 相对 v1.2.3 应判定为过期")
	}
	asset, err := u.AssetFor("linux", "amd64", rel)
	if err != nil {
		t.Fatalf("AssetFor: %v", err)
	}
	if err := u.Install(asset); err != nil {
		t.Fatalf("Install: %v", err)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("二进制替换失败: %q", got)
	}
	if u.IsOutdated("1.0.0") || u.IsOutdated("v1.0.0") {
		t.Fatal("同版本应判定为最新")
	}
}

func TestAssetFor(t *testing.T) {
	rel := &Release{}
	for _, n := range []string{"fly-linux-amd64.tar.gz", "fly-linux-arm64.tar.gz", "fly-darwin-arm64.tar.gz", "fly-windows-amd64.zip", "fly-windows-arm64.zip", "fly-1.0.0.deb"} {
		rel.Assets = append(rel.Assets, struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		}{Name: n, BrowserDownloadURL: "https://x/" + n})
	}
	u := New()
	cases := []struct {
		goos, goarch, want string
	}{
		{"linux", "amd64", "fly-linux-amd64.tar.gz"},
		{"darwin", "arm64", "fly-darwin-arm64.tar.gz"},
		{"windows", "amd64", "fly-windows-amd64.zip"},
		{"windows", "arm64", "fly-windows-arm64.zip"},
	}
	for _, c := range cases {
		a, err := u.AssetFor(c.goos, c.goarch, rel)
		if err != nil {
			t.Fatalf("%s/%s: %v", c.goos, c.goarch, err)
		}
		if a.Name != c.want {
			t.Fatalf("%s/%s: 期望 %s 实际 %s", c.goos, c.goarch, c.want, a.Name)
		}
	}
	if _, err := u.AssetFor("plan9", "mips", rel); err == nil {
		t.Fatal("未知平台应报错")
	}
}

func TestExtractBinary(t *testing.T) {
	var bin bytes.Buffer
	gw := gzip.NewWriter(&bin)
	tw := tar.NewWriter(gw)
	tw.WriteHeader(&tar.Header{Name: "usr/bin/fly", Mode: 0755, Size: 3})
	tw.Write([]byte("new"))
	tw.Close()
	gw.Close()

	dir := t.TempDir()
	p := filepath.Join(dir, "a.tar.gz")
	os.WriteFile(p, bin.Bytes(), 0644)
	data, err := extractBinary(p, "a.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("解包内容不匹配: %q", data)
	}
}

func TestProxyModes(t *testing.T) {
	u := New()
	if err := u.SetProxy("socks5://127.0.0.1:1080"); err != nil {
		t.Fatalf("socks5 代理: %v", err)
	}
	if err := u.SetProxy("http://127.0.0.1:8080"); err != nil {
		t.Fatalf("http 代理: %v", err)
	}
	if err := u.SetProxy("ftp://x"); err == nil {
		t.Fatal("ftp 代理应报错")
	}
}
