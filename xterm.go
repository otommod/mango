package main

import (
	"image/color"
)

var (
	XTerm256Palette = make(color.Palette, 0, 255)
)

func init() {
	// The XTerm 256-colour extension
	// ==============================
	//
	// There are two parts to this: first, an escape code to tell the terminal
	// which colour to select.  "Traditional" ANSI escape codes only support 8
	// colours (and some variations of these).  Secondly, the values of these
	// new colours.
	//
	// While most terminal emulators allow users to customize the full
	// 256-colour range, users generally only change the first 16.  The
	// remaining 240 can mostly be relied on to have predictable values.
	//
	// There is also the less well supported 88-colour extension.

	// Default terminal colours (index 0 - 15)
	// -----------------------------------------
	//
	// The first 8 colours are the default terminal colours.  They are
	// generally set to the user's preference but they should represent
	//
	//   0. black
	//   1. red
	//   2. green
	//   3. yellow
	//   4. blue
	//   5. magenta
	//   6. cyan
	//   7. white
	//
	// The next 8 (index 8 - 15) are so called "bright" variants of the
	// above.  They have no standardized values either.  Depending on the
	// emulator these are used instead of/in conjunction with the bold font.
	//
	// Since we can't know their values, we represent them as Transparent.
	//
	for i := 0; i < 16; i++ {
		XTerm256Palette = append(XTerm256Palette, color.Transparent)
	}

	// The "colourcube" (index 16 - 231)
	// -----------------------------------
	//
	// Each of these colours is defined by its Red, Green and Blue components,
	// but with only 6 distinct values (a total of 216 permutations)
	// corresponding to
	//
	//   {0, 95, 135, 175, 215, 255}
	//
	// in "full" 24-bit RGB.  They can be thought of as a 6*6*6 cube of colours.
	//
	cubelevels := []uint8{0x00, 0x5f, 0x87, 0xaf, 0xd7, 0xff}
	for _, r := range cubelevels {
		for _, g := range cubelevels {
			for _, b := range cubelevels {
				XTerm256Palette = append(XTerm256Palette, color.RGBA{r, g, b, 255})
			}
		}
	}

	// 24 shades of grey (index 232 - 255)
	// -------------------------------------
	//
	// These are pretty straightforward, they form the set
	//
	//   { Gray{c} | c = 8, 18, 28, ..., 238 }
	//
	for ic := 0; ic < 24; ic++ {
		XTerm256Palette = append(XTerm256Palette, color.Gray{uint8(8 + 10*ic)})
	}
}
