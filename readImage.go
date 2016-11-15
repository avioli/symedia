package main

import (
	"github.com/rwcarlsen/goexif/exif"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
)

func ReadImage(fpath string) (newPath string, meta FileMeta, err error) {
	f, err := os.Open(fpath)
	if err != nil {
		return
	}
	defer f.Close()

	defer func() {
		// alt dimensions
		if meta.IsZero() {
			imgConfig, _, _err := image.DecodeConfig(f)
			if _err == nil {
				meta.Width = imgConfig.Width
				meta.Height = imgConfig.Height
			}
		}
		// TODO: PNGs don't get dimensions
	}()

	x, _err := exif.Decode(f)
	if _err != nil {
		err = SkipFile
		return
	}

	var width, height int
	_width, _err := x.Get(exif.PixelXDimension)
	if _err == nil {
		_height, _err := x.Get(exif.PixelYDimension)
		if _err == nil {
			width, _ = _width.Int(0)
			height, _ = _height.Int(0)
		}
	}

	meta.Width = width
	meta.Height = height

	tm, _err := x.DateTime()
	if _err != nil {
		err = SkipFile
		return
	}

	newPath = ConstructPath(tm)
	return
}
