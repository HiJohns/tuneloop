package services

import (
	"bytes"
	"image"
	"image/jpeg"

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
	"golang.org/x/image/draw"
)

// GenerateThumbnail decodes an image, scales it to fit within maxSize (maintaining aspect ratio),
// and returns JPEG-encoded bytes (quality 85).
func GenerateThumbnail(data []byte, maxSize int) ([]byte, error) {
	src, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return nil, err
	}

	bounds := src.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	if w <= maxSize && h <= maxSize {
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

// GenerateThumbnailWebP resizes an image to fit within maxWidth × maxHeight (maintaining aspect ratio,
// no upscaling) and encodes as WebP with quality 0.8.
func GenerateThumbnailWebP(data []byte, maxWidth, maxHeight int) ([]byte, error) {
	src, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return nil, err
	}

	bounds := src.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	// No upscaling
	if w <= maxWidth && h <= maxHeight {
		var buf bytes.Buffer
		if err := webp.Encode(&buf, src, &webp.Options{Quality: 80}); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}

	// Scale proportionally to fit within max dimensions
	scaleW := float64(maxWidth) / float64(w)
	scaleH := float64(maxHeight) / float64(h)
	scale := scaleW
	if scaleH < scaleW {
		scale = scaleH
	}

	newW := int(float64(w) * scale)
	newH := int(float64(h) * scale)
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)

	var buf bytes.Buffer
	if err := webp.Encode(&buf, dst, &webp.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ResizeToCoverSquare crops the image to a centered square and resizes to coverSize × coverSize.
// If the image is smaller than coverSize, it is not upscaled. Encodes as WebP quality 0.8.
func ResizeToCoverSquare(data []byte, coverSize int) ([]byte, error) {
	src, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return nil, err
	}

	bounds := src.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	// Center crop to square
	size := w
	if h < w {
		size = h
	}
	cropRect := image.Rect(0, 0, size, size)
	cropRect = cropRect.Add(image.Pt((w-size)/2, (h-size)/2))
	cropped := src.(interface {
		SubImage(r image.Rectangle) image.Image
	}).SubImage(cropRect)

	// Resize if larger than coverSize
	if size <= coverSize {
		var buf bytes.Buffer
		if err := webp.Encode(&buf, cropped, &webp.Options{Quality: 80}); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}

	dst := image.NewRGBA(image.Rect(0, 0, coverSize, coverSize))
	draw.CatmullRom.Scale(dst, dst.Bounds(), cropped, cropped.Bounds(), draw.Over, nil)

	var buf bytes.Buffer
	if err := webp.Encode(&buf, dst, &webp.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
