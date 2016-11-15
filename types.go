package main

type File struct {
	Origin string   `json:"origin"`
	Link   string   `json:"link"`
	Flag   FlagType `json:"flag"`
	Size   int64    `json:"size"`
	Name   string   `json:"name"`
	Ext    string   `json:"ext"`
	Width  int      `json:"width"`
	Height int      `json:"height"`
}

type Files []File

type Tags struct {
	CreationTime   string `json:"creation_time"`
	QtCreationDate string `json:"com.apple.quicktime.creationdate"`
}

type Tagged struct {
	Tags Tags
}

type Ffprobe struct {
	Streams []Tagged
	Format  Tagged
}
