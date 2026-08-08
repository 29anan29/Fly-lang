package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLatestDev(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/x/y/releases/latest" {
			json.NewEncoder(w).Encode(Release{TagName: "v0.2.0"})
			return
		}
		json.NewEncoder(w).Encode([]Release{
			{TagName: "v0.3.0-dev", Prerelease: true},
			{TagName: "v0.2.0"},
			{TagName: "v0.3.0-dev.1", Prerelease: true},
		})
	}))
	defer srv.Close()

	u := New()
	u.BaseURL = srv.URL
	u.Repo = "x/y"

	rel, err := u.Latest()
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if rel.TagName != "v0.2.0" {
		t.Fatalf("Latest 应取正式版 v0.2.0，得到 %s", rel.TagName)
	}

	dev, err := u.LatestDev()
	if err != nil {
		t.Fatalf("LatestDev: %v", err)
	}
	if dev.TagName != "v0.3.0-dev" {
		t.Fatalf("LatestDev 应取最新预发布 v0.3.0-dev，得到 %s", dev.TagName)
	}
	if !dev.Prerelease {
		t.Fatal("LatestDev 返回的应为 prerelease")
	}
}

func TestLatestDevEmpty(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Release{{TagName: "v0.2.0"}})
	}))
	defer srv.Close()

	u := New()
	u.BaseURL = srv.URL
	u.Repo = "x/y"
	if _, err := u.LatestDev(); err == nil {
		t.Fatal("无预发布时应报错")
	}
}
