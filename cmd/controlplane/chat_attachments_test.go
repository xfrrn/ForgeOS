package main

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestCleanChatImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})

	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}
	mime, out, err := cleanChatImage("image/png", pngBuf.Bytes())
	if err != nil || mime != "image/png" || len(out) == 0 {
		t.Fatalf("png clean failed: mime=%q len=%d err=%v", mime, len(out), err)
	}

	var jpgBuf bytes.Buffer
	if err := jpeg.Encode(&jpgBuf, img, nil); err != nil {
		t.Fatal(err)
	}
	mime, out, err = cleanChatImage("image/jpeg", jpgBuf.Bytes())
	if err != nil || mime != "image/jpeg" || len(out) == 0 {
		t.Fatalf("jpeg clean failed: mime=%q len=%d err=%v", mime, len(out), err)
	}

	var gifBuf bytes.Buffer
	if err := gif.Encode(&gifBuf, img, nil); err != nil {
		t.Fatal(err)
	}
	mime, out, err = cleanChatImage("image/gif", gifBuf.Bytes())
	if err != nil || mime != "image/png" || len(out) == 0 {
		t.Fatalf("gif clean failed: mime=%q len=%d err=%v", mime, len(out), err)
	}

	webp := []byte{'R', 'I', 'F', 'F', 4, 0, 0, 0, 'W', 'E', 'B', 'P', 'V', 'P', '8', ' '}
	mime, out, err = cleanChatImage("image/webp", webp)
	if err != nil || mime != "image/webp" || !bytes.Equal(out, webp) {
		t.Fatalf("webp clean failed: mime=%q err=%v", mime, err)
	}

	if _, _, err := cleanChatImage("text/plain", []byte("hello")); err == nil {
		t.Fatal("accepted non-image payload")
	}
	if _, _, err := cleanChatImage("image/png", []byte("not-a-png")); err == nil {
		t.Fatal("accepted corrupted PNG")
	}
}
