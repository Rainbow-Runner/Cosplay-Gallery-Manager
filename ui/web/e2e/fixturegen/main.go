package main

import (
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"log"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("fixture destination is required")
	}
	root := os.Args[1]
	if err := os.MkdirAll(filepath.Join(root, "selfie"), 0o700); err != nil {
		log.Fatal(err)
	}
	if err := writeJPEG(filepath.Join(root, "01-photo.jpg"), false); err != nil {
		log.Fatal(err)
	}
	if err := writeJPEG(filepath.Join(root, "selfie", "02-selfie.jpg"), true); err != nil {
		log.Fatal(err)
	}
	if err := writeGIF(filepath.Join(root, "03-animated.gif")); err != nil {
		log.Fatal(err)
	}
}

func writeJPEG(path string, alternate bool) error {
	value := image.NewRGBA(image.Rect(0, 0, 96, 72))
	for y := 0; y < value.Bounds().Dy(); y++ {
		for x := 0; x < value.Bounds().Dx(); x++ {
			if alternate {
				value.Set(x, y, color.RGBA{R: uint8(40 + x), G: uint8(80 + y), B: 180, A: 255})
			} else {
				value.Set(x, y, color.RGBA{R: 190, G: uint8(30 + x), B: uint8(70 + y), A: 255})
			}
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	return jpeg.Encode(file, value, &jpeg.Options{Quality: 90})
}

func writeGIF(path string) error {
	palette := color.Palette{color.RGBA{R: 20, G: 20, B: 30, A: 255}, color.RGBA{R: 230, G: 110, B: 90, A: 255}}
	first := image.NewPaletted(image.Rect(0, 0, 64, 48), palette)
	second := image.NewPaletted(image.Rect(0, 0, 64, 48), palette)
	for y := 8; y < 40; y++ {
		for x := 8; x < 32; x++ {
			first.SetColorIndex(x, y, 1)
			second.SetColorIndex(x+24, y, 1)
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	return gif.EncodeAll(file, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{8, 8}, LoopCount: 0})
}
