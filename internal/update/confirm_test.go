// confirm_test.go：交互式确认（终端输入）单测。
package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfirm(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", true},
		{"\n", true},
		{"n\n", false},
		{"N\n", false},
		{"no\n", false},
		{"maybe\nn\n", false},
		{"maybe\ny\n", true},
	}
	for _, c := range cases {
		got, err := Confirm(strings.NewReader(c.in))
		if err != nil {
			t.Fatalf("Confirm(%q) 报错: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("Confirm(%q) = %v, 期望 %v", c.in, got, c.want)
		}
	}
	got, err := Confirm(strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Fatal("EOF 应返回 false（不安装）")
	}
}

func TestCheckWritable(t *testing.T) {
	u := New()
	dir := t.TempDir()
	if err := u.CheckWritable(dir); err != nil {
		t.Fatalf("可写目录报错: %v", err)
	}
	ro := filepath.Join(dir, "ro")
	if err := os.MkdirAll(ro, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(ro, 0755)
	if err := u.CheckWritable(ro); err == nil {
		t.Fatal("只读目录应报错")
	}
}
