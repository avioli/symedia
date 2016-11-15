package main

import (
	"path"
	"strconv"
	"time"
)

func ConstructPath(tm time.Time) string {
	return path.Join(strconv.Itoa(tm.Year()), tm.Format("01-Jan"), tm.Format("2006-01-02"))
}
