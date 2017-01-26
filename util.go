package main

import (
	"path"
	"strconv"
	"time"
)

func ConstructPath(tm time.Time) string {
	return path.Join(strconv.Itoa(tm.Year()), tm.Format("01-Jan"), tm.Format("2006-01-02"))
}

func ConstructFilename(tm time.Time) string {
	// Format: Mon Jan 2 15:04:05.000 -0700 MST 2006 or 03:04:05.000 am
	return tm.Format("2006-01-02 15.04.05 -0700")
}
