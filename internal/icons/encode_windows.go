//go:build windows

package icons

import "image"

// encodeIcon wraps the frames in an ICO container. The Windows notification
// area loads the icon through LoadImage, which picks the frame matching the
// current DPI, so both sizes are worth shipping.
func encodeIcon(imgs ...*image.RGBA) []byte { return encodeICO(imgs...) }
