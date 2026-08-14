package format

import "testing"

func TestFormat(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"assign", "x=1+2\ny= x *  3\n", "x = 1 + 2\ny = x * 3\n"},
		{"comment_keep", "x=1  # 注释\n# 顶行注释\nprint(x)\n", "x = 1 # 注释\n# 顶行注释\nprint(x)\n"},
		{"call", "print( 1,2 , 3 )\n", "print(1, 2, 3)\n"},
		{"kwarg", "f(x=1,y=2)\n", "f(x=1, y=2)\n"},
		{"compare", "if a>=1 and b<2 :\n    x=1\n", "if a >= 1 and b < 2:\n    x = 1\n"},
		{"slice", "a[1:2]\nprint( a [0: -1] )\n", "a[1:2]\nprint(a[0:-1])\n"},
		{"unary", "x=-5+ +3\ny=not x\n", "x = -5 + +3\ny = not x\n"},
		{"blank_compress", "a=1\n\n\n\nb=2\n\n\n", "a = 1\n\nb = 2\n"},
		{"trailing_ws", "x=1   \n", "x = 1\n"},
		{"dict", "{ \"a\":1 ,\"b\": 2}\n", "{\"a\": 1, \"b\": 2}\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Format(c.in)
			if got != c.want {
				t.Fatalf("in=%q\ngot =%q\nwant=%q", c.in, got, c.want)
			}
			// 幂等
			if again := Format(got); again != got {
				t.Fatalf("非幂等: %q != %q", again, got)
			}
		})
	}
}
