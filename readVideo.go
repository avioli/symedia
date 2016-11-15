package main

import (
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"time"
)

func ReadVideo(fpath string) (newPath string, meta FileMeta, err error) {
	var cmdOut []byte

	cmdName := "ffprobe"
	// -show_format        show format/container info
	// -show_streams       show streams info
	cmdArgs := []string{fpath, "-v", "quiet", "-print_format", "json", "-show_format", "-show_streams"}

	// @see: https://nathanleclaire.com/blog/2014/12/29/shelled-out-commands-in-golang/
	if cmdOut, err = exec.Command(cmdName, cmdArgs...).Output(); err != nil {
		return
	}

	jsonStream := string(cmdOut)

	// {
	// 	"streams": [ {
	// 		"tags": { "creation_time": "2016-07-18 02:29:36" }
	// 	}, ... ],
	// 	"format": {
	// 		"tags": {
	// 			"creation_time": "2016-07-18 02:29:36",
	// 			"com.apple.quicktime.creationdate": "2016-07-18T12:29:35+1000"
	// 		}
	// 	}
	// }

	dec := json.NewDecoder(strings.NewReader(jsonStream))

	var probe Ffprobe
	if err = dec.Decode(&probe); err != nil && err != io.EOF {
		return
	}

	var tm time.Time
	var _err error

	// check QtCreationDate first
	tm, _err = time.Parse(TzDateLayout, probe.Format.Tags.QtCreationDate)
	if _err != nil {
		// check Format's CreationTime second
		tm, _err = time.Parse(LocalDateLayout, probe.Format.Tags.CreationTime)
		if _err != nil {
			// then iterate over the Streams' CreationTimes
			for _, stream := range probe.Streams {
				tm, _err = time.Parse(LocalDateLayout, stream.Tags.CreationTime)
				if _err == nil {
					break
				}
			}
		}
	}

	if _err != nil || tm.IsZero() {
		err = SkipFile
		return
	}

	newPath = ConstructPath(tm)
	meta = FileMeta{
		Width:  0,
		Height: 0,
	}
	return
}
