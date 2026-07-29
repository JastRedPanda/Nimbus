//go:build windows

package icons

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
)

func encodeIcon(imgs ...*image.RGBA) []byte {
	buf := &bytes.Buffer{}
	binary.Write(buf, binary.LittleEndian, uint16(0))
	binary.Write(buf, binary.LittleEndian, uint16(1))
	binary.Write(buf, binary.LittleEndian, uint16(len(imgs)))

	var off = uint32(6 + len(imgs)*16)
	var allPNG [][]byte

	for _, img := range imgs {
		var pngBuf bytes.Buffer
		png.Encode(&pngBuf, img)
		data := pngBuf.Bytes()
		allPNG = append(allPNG, data)

		b := img.Bounds()
		w, h := b.Dx(), b.Dy()
		iw := uint8(w)
		if w > 255 {
			iw = 0
		}
		ih := uint8(h)
		if h > 255 {
			ih = 0
		}
		binary.Write(buf, binary.LittleEndian, iw)
		binary.Write(buf, binary.LittleEndian, ih)
		binary.Write(buf, binary.LittleEndian, uint8(0))
		binary.Write(buf, binary.LittleEndian, uint8(0))
		binary.Write(buf, binary.LittleEndian, uint16(1))
		binary.Write(buf, binary.LittleEndian, uint16(32))
		binary.Write(buf, binary.LittleEndian, uint32(len(data)))
		binary.Write(buf, binary.LittleEndian, off)
		off += uint32(len(data))
	}

	for _, data := range allPNG {
		buf.Write(data)
	}
	return buf.Bytes()
}
