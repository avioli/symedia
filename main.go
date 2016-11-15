package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/docopt/docopt-go"
	"github.com/hoisie/mustache"
	"github.com/kardianos/osext"
	"github.com/rwcarlsen/goexif/exif"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/ioutil"
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
	AppName     string
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

func writeJSON(files Files, jsonFile string) error {
	jsonBytes, err := json.MarshalIndent(files, "", "\t")
	if err != nil {
		return fmt.Errorf("Cannot convert files data to json.\n%s", err.Error())
	}
	//os.Stdout.Write(jsonBytes)

	if err = ioutil.WriteFile(jsonFile, jsonBytes, 0644); err != nil {
		return fmt.Errorf("Cannot write json: %s\n%s", jsonFile, err.Error())
	}
	return nil
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
			return nil
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

func getRoot(args map[string]interface{}, fallbackToCwd bool) (root string, err error) {
	root, ok := args["PATH"].(string)
	if !ok || root == "" {
		if fallbackToCwd {
			root, err = os.Getwd()
			return
		}
		err = fmt.Errorf("No PATH specified")
		return
	}

	if _, err = os.Stat(root); os.IsNotExist(err) {
		return
	}

	root, err = filepath.Abs(root)
	return
}

func cmdProcess(argv []string) (err error) {
	usage := fmt.Sprint(`Usage: `, AppName, ` process PATH [OUTPUT_DIR]
       `, AppName, ` process [--template_path FILE] [--template_out FILE] [--json FILE] PATH [OUTPUT_DIR]
       `, AppName, ` process -h | --help

Process PATH for images and videos and hard-link them to OUTPUT_DIR.
At the end, it writes a JSON file with the gathered metadata and parses a Mustache template file.

!!! ATTENTION !!!: Ensure OUTPUT_DIR is not within the PATH structure.

Arguments:
  PATH        the path to process
  OUTPUT_DIR  an optional output path [Defaults to "output"]

Options:
  -h, --help            print this help, then exit
  --version             print version and build, then exit
  --template_path FILE  define a custom template path [Defaults to "error-template.html" in cwd or with the executable]
  --template_out FILE   define a path for the template output [Defaults to "OUTPUT_DIR/errors.html"]
  --json FILE           define a path for the JSON output [Defaults to "OUTPUT_DIR/files.json"]

Mustache Template Data:
  { "OutDir" string,
    "Files": [{ "Origin" string
                "Link"   string
                "Flag"   (?|X|.|i|v)
                "Size"   int64
                "Name"   string
                "Ext"    string
                "Width"  int
                "Height" int
              }, ...]
  }
`)
	args, _ := docopt.Parse(usage, argv, true, "", false)
	// fmt.Println(args)

	root, err := getRoot(args, false)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Cannot get current working directory")
	}

	// create outDir
	var outDir string
	if str, ok := args["OUTPUT_DIR"].(string); ok {
		outDir = str
	} else {
		outDir = path.Join(cwd, "output")
	}
	if err = os.MkdirAll(outDir, os.ModePerm); err != nil {
		return fmt.Errorf("Cannot create output directory: %s", outDir)
	}

	// walk the directory
	files, walkErr := WalkPath(root, outDir)
	if walkErr != nil {
		fmt.Fprintln(os.Stderr, walkErr)
	}

	// log loggables
	for _, el := range files {
		if el.Flag.IsLoggable() {
			fmt.Fprintf(os.Stderr, "%s\t%s\n", el.Flag, el.Origin)
		}
	}

	// output json
	var jsonFile string
	if str, ok := args["--json"].(string); ok {
		jsonFile = str
	} else {
		jsonFile = path.Join(outDir, "files.json")
	}
	if err = writeJSON(files, jsonFile); err != nil {
		return err
	}

	// output templates
	var templatePath string
	if str, ok := args["--template_path"].(string); ok {
		templatePath = str
	} else {
		templatePath = path.Join(cwd, "error-template.html")
		if _, err := os.Stat(templatePath); os.IsNotExist(err) {
			execDir, err := osext.ExecutableFolder()
			if err != nil {
				return fmt.Errorf("Cannot get current executable path.\n%s", err.Error())
			}
			templatePath = path.Join(execDir, "error-template.html")
		}
	}

	var templateOut string
	if str, ok := args["--template_out"].(string); ok {
		templateOut = str
	} else {
		templateOut = path.Join(outDir, "errors.html")
	}
	data := map[string]interface{}{
		"OutDir": outDir,
		"Files":  files,
	}
	rendered := mustache.RenderFile(templatePath, data)
	if err = ioutil.WriteFile(templateOut, []byte(rendered), 0644); err != nil {
		return fmt.Errorf("Cannot render or write errored files: %s.\n%s", templatePath, err.Error())
	}
	return
}

func cmdHelp(argv []string) (err error) {
	usage := fmt.Sprint(`Usage: `, AppName, ` help [<command>] [<args>...]

Give help for given command
`)
	args, _ := docopt.Parse(usage, argv, true, "", false)

	if cmd, ok := args["<command>"].(string); ok {
		return runCommand(cmd, []string{"--help"})
	}

	return mainApp([]string{"--help"})
}

func runCommand(cmd string, args []string) (err error) {
	argv := make([]string, 1)
	argv[0] = cmd
	argv = append(argv, args...)

	switch cmd {
	case "help":
		return cmdHelp(argv)
	case "process":
		return cmdProcess(argv)
	}

	return fmt.Errorf("%s is not a valid command. See '%s help'", cmd, AppName)
}

func mainApp(argv []string) (err error) {
	usage := fmt.Sprint(`Usage: `, AppName, ` <command> [<args>...]
       `, AppName, ` -h | --help | --version

Options:
  -h, --help  print this help, then exit
  --version   print version and build, then exit

Commands:
  help <command>  Print help for specific command
  process         Process a directory for images and videos, while hard-linking them to an output directory.

See '`, AppName, ` help <command>' for more information on a specific command.
`)
	args, _ := docopt.Parse(usage, argv, true, "", true)

	if args["--version"].(bool) {
		fmt.Printf("Version: %s\nCommit: %s", Version, Build)
		os.Exit(0)
	}

	// fmt.Println("global arguments:")
	// fmt.Println(args)

	// fmt.Println("command arguments:")
	cmd := args["<command>"].(string)
	cmdArgs := args["<args>"].([]string)

	return runCommand(cmd, cmdArgs)
}

func init() {
	AppName = path.Base(os.Args[0])
}

func main() {
	err := mainApp(nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
