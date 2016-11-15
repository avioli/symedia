package main

type FileMeta struct {
	Width  int
	Height int
}

func (meta FileMeta) IsZero() bool {
	return meta.Width == 0 || meta.Height == 0
}
