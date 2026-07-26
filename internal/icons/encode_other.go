//go:build !windows

package icons

import (
	"bytes"
	"image"
	"image/png"
)

// encodeIcon emits a bare PNG of the largest frame.
//
// StatusNotifierItem trays decode the bytes themselves rather than handing them
// to the OS, and the pure-Go implementations register only image/png - an ICO
// container decodes as "unknown format" and publishes a 0x0 pixmap, which the
// panel draws as nothing at all. The failure is silent, so the format matters.
func encodeIcon(imgs ...*image.RGBA) []byte {
	if len(imgs) == 0 {
		return nil
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, imgs[len(imgs)-1]); err != nil {
		return nil
	}
	return buf.Bytes()
}
