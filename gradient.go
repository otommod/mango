package main

import (
	"image/color"
	"sort"
)

// LinearGradient is a linear gradient.
type LinearGradient []color.Color

func blend(x, y color.Color, t float64) color.Color {
	xr, xg, xb, _ := x.RGBA()
	yr, yg, yb, _ := y.RGBA()

	w := uint32(0xffff * t)
	r := xr + (w * (yr - xr) >> 16)
	g := xg + (w * (yg - xg) >> 16)
	b := xb + (w * (yb - xb) >> 16)
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 0xff}
}

// At returns the value of the gradient at t percent (between 0 and 1)
func (lg LinearGradient) At(t float64) color.Color {
	N := float64(len(lg) - 1)
	i := sort.Search(len(lg), func(i int) bool {
		return float64(i)/N >= t
	})

	switch i {
	case 0:
		// The point is before the "start" of the gradient
		return lg[0]
	case len(lg):
		// That's how sort.Search represents a result not found
		// After the "end of the gradient
		return lg[i-1]
	}

	colorA := lg[i-1]
	colorB := lg[i]

	t = N*t - float64(i) + 1
	return blend(colorA, colorB, t)
}
