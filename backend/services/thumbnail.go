package services

import (
	"bytes"
	"image"
	"image/jpeg"

	"golang.org/x/image/draw"
)

// GenerateThumbnail decodes an image, scales it to fit within maxSize (maintaining aspect ratio),
// and returns JPEG-encoded bytes.
func GenerateThumbnail(data []byte, maxSize int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	bounds := src.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	if w <= maxSize && h <= maxSize {
		// Already small enough — encode without scaling
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 85}); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}

	var newW, newH int
	if w >= h {
		newW = maxSize
		newH = int(float64(h) * float64(maxSize) / float64(w))
	} else {
		newH = maxSize
		newW = int(float64(w) * float64(maxSize) / float64(h))
	}
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
