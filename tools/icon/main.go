// main.go：图标生成器——生成 assets/icon.png 产物。
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

type pt struct{ x, y float64 }

type poly struct {
	pts   []pt
	color color.RGBA
	alpha float64
}

var polys = []poly{
	{
		pts:   []pt{{72, 64}, {440, 64}, {386, 158}, {232, 158}, {232, 218}, {350, 218}, {298, 308}, {232, 308}, {232, 448}, {126, 448}, {126, 158}},
		color: color.RGBA{0x35, 0xC9, 0x8B, 255},
		alpha: 1,
	},
	{
		pts:   []pt{{126, 64}, {350, 64}, {302, 146}, {214, 146}, {214, 238}, {300, 238}, {258, 310}, {214, 310}, {214, 384}, {174, 448}, {174, 128}},
		color: color.RGBA{0x18, 0x3B, 0x56, 255},
		alpha: 1,
	},
	{
		pts:   []pt{{126, 64}, {174, 64}, {174, 448}, {126, 416}, {126, 158}, {72, 64}},
		color: color.RGBA{0xA7, 0xF3, 0xD0, 255},
		alpha: 0.92,
	},
}

func inPoly(x, y float64, p poly) bool {
	inside := false
	n := len(p.pts)
	j := n - 1
	for i := 0; i < n; i++ {
		xi, yi := p.pts[i].x, p.pts[i].y
		xj, yj := p.pts[j].x, p.pts[j].y
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			inside = !inside
		}
		j = i
	}
	return inside
}

func sample(px, py float64) (float64, float64, float64, float64) {
	var r, g, b, a float64
	for i := range polys {
		if !inPoly(px, py, polys[i]) {
			continue
		}
		al := polys[i].alpha
		oc := polys[i].color
		r = r*(1-al) + float64(oc.R)*al
		g = g*(1-al) + float64(oc.G)*al
		b = b*(1-al) + float64(oc.B)*al
		a = a*(1-al) + al
	}
	return r, g, b, a
}

func main() {
	const size = 256
	const ss = 4
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	scale := float64(size) / 512
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := (float64(x) + (float64(sx)+0.5)/ss) / scale
					py := (float64(y) + (float64(sy)+0.5)/ss) / scale
					pr, pg, pb, pa := sample(px, py)
					r += pr
					g += pg
					b += pb
					a += pa
				}
			}
			n := float64(ss * ss)
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(r / n),
				G: uint8(g / n),
				B: uint8(b / n),
				A: uint8(a / n * 255),
			})
		}
	}
	out := "assets/icon.png"
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		panic(err)
	}
	f, err := os.Create(out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}
