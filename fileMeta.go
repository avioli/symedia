package main

import "time"

type FileMeta struct {
	Width  int
	Height int
	Time   time.Time
}

func (meta FileMeta) IsZero() bool {
	return meta.Width == 0 || meta.Height == 0 || meta.Time.IsZero()
}
