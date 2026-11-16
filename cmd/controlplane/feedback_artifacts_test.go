package main

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestScreenshotStoresVisiblePixels(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	im.SetNRGBA(0, 0, color.NRGBA{R: 85, G: 10, B: 99, A: 0})
	im.SetNRGBA(1, 0, color.NRGBA{R: 19, A: 255})
	var src bytes.Buffer
	if err := png.Encode(&src, im); err != nil {
		t.Fatal(err)
	}
	original := src.Bytes()
	tag := []byte("tEXtAuthor\x00private fixture")
	var input bytes.Buffer
	input.Write(original[:len(original)-12])
	binary.Write(&input, binary.BigEndian, uint32(len(tag)-4))
	input.Write(tag)
	binary.Write(&input, binary.BigEndian, crc32.ChecksumIEEE(tag))
	input.Write(original[len(original)-12:])
	content, err := cleanScreenshot(input.Bytes())
	if err != nil || bytes.Contains(content, []byte("private fixture")) {
		t.Fatal("source metadata retained or PNG lost", err)
	}
	result, err := png.Decode(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(result.At(0, 0)); got != (color.NRGBA{255, 255, 255, 255}) {
		t.Fatal("hidden color retained", got)
	}
	if got := color.NRGBAModel.Convert(result.At(1, 0)); got != (color.NRGBA{19, 0, 0, 255}) {
		t.Fatal("visible pixel changed", got)
	}
	if _, err := cleanScreenshot([]byte("not an image")); err == nil {
		t.Fatal("accepted an invalid screenshot")
	}
}
