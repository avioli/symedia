package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hoisie/mustache"
	"github.com/rwcarlsen/goexif/exif"
	flag "github.com/spf13/pflag"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	LocalDateLayout = "2006-01-02 15:04:05"
	TzDateLayout    = "2006-01-02T15:04:05-0700"
)

var (
	Version string
	Build   string
	// Img = regexp.MustCompile(`(?i).(jpe?g|png|gif)`)
	Img         = regexp.MustCompile(`(?i).(jpe?g)`)
	Vid         = regexp.MustCompile(`(?i).(mov|mp4|m4v)`)
	SkipFile    = errors.New("skip this file")
	NoPath      = errors.New("no path")
	UnknownFile = errors.New("unknown file")
	flagHelp    bool
	flagVersion bool
)

type FlagType int

const (
	_ FlagType = iota
	FlagUnknown
	FlagError
	FlagSkipped
	FlagImage
	FlagVideo
)

var flagTypeValues = []string{
	FlagUnknown: "?",
	FlagError:   "X",
	FlagSkipped: ".",
	FlagImage:   "i",
	FlagVideo:   "v",
}

func (t FlagType) String() string {
	if t <= 0 || int(t) >= len(flagTypeValues) {
		return ""
	}
	return flagTypeValues[t]
}

func (t FlagType) IsLoggable() bool {
	return t == FlagUnknown || t == FlagError || t == FlagSkipped
}

func (t FlagType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.String() + `"`), nil
}

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

type FileMeta struct {
	Width  int
	Height int
}

func (meta FileMeta) IsZero() bool {
	return meta.Width == 0 || meta.Height == 0
}

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

func ConstructPath(tm time.Time) string {
	return path.Join(strconv.Itoa(tm.Year()), tm.Format("01-Jan"), tm.Format("2006-01-02"))
}

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
	//log.Println(x)

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

// Walks over a given path and processes the found files
func WalkPath(inDir string, outDir string) (Files, error) {
	files := make(Files, 0, 100)

	visit := func(fpath string, f os.FileInfo, _err error) error {
		if f.IsDir() {
			// TODO: use filepath.SkipDir if a dir is marked for skipping
			return nil
		}

		var newPath string
		var meta FileMeta
		var err error

		var file = File{
			Origin: fpath,
			Link:   "",
			Flag:   FlagUnknown,
			Size:   f.Size(),
			Name:   f.Name(),
			Ext:    strings.ToLower(filepath.Ext(f.Name())[1:]),
			Width:  0,
			Height: 0,
		}
		defer func() { files = append(files, file) }()
		defer func() { fmt.Printf("%s", file.Flag) }()

		if Img.MatchString(f.Name()) {
			newPath, meta, err = ReadImage(fpath)
			file.Flag = FlagImage
			file.Width = meta.Width
			file.Height = meta.Height
		} else if Vid.MatchString(f.Name()) {
			newPath, meta, err = ReadVideo(fpath)
			file.Flag = FlagVideo
			file.Width = meta.Width
			file.Height = meta.Height
		} else {
			return nil
		}

		if err == SkipFile {
			file.Flag = FlagSkipped
			return nil
		} else if err != nil {
			file.Flag = FlagError
			return err
		}

		if newPath == "" {
			file.Flag = FlagError
			return NoPath
		}

		linkPath := path.Join(outDir, newPath)

		if err = os.MkdirAll(linkPath, os.ModePerm); err != nil {
			file.Flag = FlagError
			return err
		}

		if err = os.Link(fpath, path.Join(linkPath, f.Name())); err != nil && !os.IsExist(err) {
			file.Flag = FlagError
			return err
		}

		file.Link = path.Join(newPath, f.Name())
		return nil
	}

	err := filepath.Walk(inDir, visit)

	fmt.Printf("\n")
	return files, err
}

var Usage = func() {
	fmt.Fprintf(os.Stderr, "usage: %s [options] <path>\n", path.Base(os.Args[0]))
	flag.PrintDefaults()
}

func init() {
	flag.Usage = Usage
	flag.BoolVarP(&flagHelp, "help", "h", false, "print help and exit")
	flag.BoolVarP(&flagVersion, "version", "v", false, "print version and exit")
}

func main() {
	flag.Parse()

	if flagHelp {
		Usage()
		os.Exit(0)
	}

	if flagVersion {
		fmt.Printf("Version: %s\nCommit: %s", Version, Build)
		os.Exit(0)
	}

	root := flag.Arg(0)
	//root = "/Users/avioli/Pictures/Photos Library.photoslibrary/Masters/2016/10/"
	if root == "" {
		fmt.Fprintln(os.Stderr, "No path specified")
		Usage()
		os.Exit(2)
	}

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	// create outDir
	outDir := path.Join(cwd, "output")
	if err = os.MkdirAll(outDir, os.ModePerm); err != nil {
		log.Fatal(err)
	}

	// walk the directory
	files, walkErr := WalkPath(root, outDir)
	if walkErr != nil {
		//log.Fatal(err)
		fmt.Fprintln(os.Stderr, walkErr)
	}

	// log loggables
	for _, el := range files {
		if el.Flag.IsLoggable() {
			fmt.Fprintf(os.Stderr, "%s\t%s\n", el.Flag, el.Origin)
		}
	}

	// output json
	jsonBytes, err := json.Marshal(files)
	if err != nil {
		log.Fatal(err)
	}
	//os.Stdout.Write(jsonBytes)

	jsonOut := path.Join(outDir, "files.json")
	if err = ioutil.WriteFile(jsonOut, jsonBytes, 0644); err != nil {
		log.Fatal(err)
	}

	// output templates
	templatePath := path.Join(cwd, "error-template.html")
	templateOut := path.Join(outDir, "errors.html")
	data := map[string]interface{}{
		"OutDir": outDir,
		"Files":  files,
	}
	rendered := mustache.RenderFile(templatePath, data)
	if err = ioutil.WriteFile(templateOut, []byte(rendered), 0644); err != nil {
		log.Fatal(err)
	}
}
