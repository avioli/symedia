package main

import (
	"bytes"
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
	"sort"
	"strings"
	"time"
)

//go:generate go run scripts/embed-template.go

// FlagType stores the type of entry
type FlagType int

const (
	_ FlagType = iota
	// FlagUnknown if for unknown file type
	FlagUnknown
	// FlagError is for an errored file
	FlagError
	// FlagSkipped is for a skipped file
	FlagSkipped
	// FlagExists is for an file that already has a hardlink
	FlagExists
	// FlagImage is for an image file
	FlagImage
	// FlagVideo is for a video file
	FlagVideo
)

var flagTypeValues = []string{
	FlagUnknown: "?",
	FlagError:   "X",
	FlagSkipped: ".",
	FlagExists:  "-",
	FlagImage:   "i",
	FlagVideo:   "v",
}

func (t FlagType) String() string {
	if t <= 0 || int(t) >= len(flagTypeValues) {
		return ""
	}
	return flagTypeValues[t]
}

// IsLoggable returns true if flags are loggable.
// Such flags are FlagUnknown, FlagError and FlagSkipped.
func (t FlagType) IsLoggable() bool {
	return t == FlagUnknown || t == FlagError || t == FlagSkipped
}

// MarshalJSON converts a FlagType(int) to a string
func (t FlagType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.String() + `"`), nil
}

var (
	// Version holds the package version
	Version string
	// Build holds the build git SHA
	Build string
	// AppName holds... the app name
	AppName string
)

const (
	// localDateLayout used by video's CreationTime
	localDateLayout = "2006-01-02T15:04:05.000000Z"
	// tzDateLayout used by video's QtCreationDate
	tzDateLayout = "2006-01-02T15:04:05-0700"
)

// CommandFunc type represents a Command Callback
type CommandFunc func([]string) error

// Command is a registered command struct
type Command struct {
	Name        string
	UsageName   string
	Description string
	Callback    CommandFunc
}

type commands []*Command

var registeredCommands commands

func (cmds commands) String() string {
	var buffer bytes.Buffer
	var names []string
	lines := make(map[string]string)
	maxLen := 0

	for _, command := range cmds {
		n := command.UsageName
		if n == "" {
			n = command.Name
		}
		names = append(names, n)
		lines[n] = command.Description
		if len(n) > maxLen {
			maxLen = len(n)
		}
	}
	sort.Strings(names)

	for _, n := range names {
		buffer.WriteString(fmt.Sprintf("  %s  %s\n", n+strings.Repeat(" ", maxLen-len(n)), lines[n]))
	}

	return buffer.String()
}

func registerCommand(name string, desc string, callback CommandFunc) (*Command, error) {
	cmd := Command{
		Name:        name,
		UsageName:   "",
		Description: desc,
		Callback:    callback,
	}
	registeredCommands = append(registeredCommands, &cmd)
	return &cmd, nil
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

	if cmd == "help" {
		return cmdHelp(argv)
	}

	for _, command := range registeredCommands {
		if command.Name == cmd {
			return command.Callback(argv)
		}
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
`, registeredCommands, `
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

	if cmd, err := registerCommand("help", "Print help for specific command.", cmdHelp); err == nil {
		cmd.UsageName = "help <command>"
	}
}

func main() {
	err := mainApp(nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// === Command `process`

var (
	// Img holds the supported image files regex (PNG and GIF files don't have embedded timestamps)
	Img = regexp.MustCompile(`(?i).(jpe?g)`)
	// Vid holds the supported video files regex
	Vid = regexp.MustCompile(`(?i).(mov|mp4|m4v)`)
)

var (
	// ErrSkipFile represents a marker to skip a file
	ErrSkipFile = errors.New("skip this file")
	// ErrNoPath represents a marker that a processor yielded no path
	ErrNoPath = errors.New("no path")
	// ErrUnknownFile represents a marker that a file did not match Img and Vid
	ErrUnknownFile = errors.New("unknown file")
)

// WalkPath walks over a given path and processes the found files
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

		if Img.MatchString(file.Name) {
			meta, err = ReadImage(fpath)
			newPath = ConstructPath(meta.Time)
			file.Name = ConstructFilename(meta.Time) + "." + file.Ext
			file.Flag = FlagImage
			file.Width = meta.Width
			file.Height = meta.Height
		} else if Vid.MatchString(file.Name) {
			meta, err = ReadVideo(fpath)
			newPath = ConstructPath(meta.Time)
			file.Name = ConstructFilename(meta.Time) + "." + file.Ext
			file.Flag = FlagVideo
			file.Width = meta.Width
			file.Height = meta.Height
		} else {
			// NOTE: Unknown
			return nil
		}

		if err == ErrSkipFile {
			file.Flag = FlagSkipped
			return nil
		} else if err != nil {
			file.Flag = FlagError
			return nil
		}

		if newPath == "" {
			file.Flag = FlagError
			return ErrNoPath
		}

		linkPath := path.Join(outDir, newPath)

		if err = os.MkdirAll(linkPath, os.ModePerm); err != nil {
			file.Flag = FlagError
			return err
		}

		err = os.Link(fpath, path.Join(linkPath, file.Name))
		if err != nil {
			if os.IsExist(err) {
				file.Flag = FlagExists
			} else {
				file.Flag = FlagError
				return err
			}
		}

		file.Link = path.Join(newPath, file.Name)
		return nil
	}

	err := filepath.Walk(inDir, visit)

	fmt.Printf("\n")
	return files, err
}

func cmdProcess(argv []string) (err error) {
	usage := fmt.Sprint(`Usage: `, AppName, ` process PATH [OUTPUT_DIR]
       `, AppName, ` process [--template_path FILE] [--template_out FILE] [--json FILE] PATH [OUTPUT_DIR]
       `, AppName, ` process --print_template
       `, AppName, ` process -h | --help

Process PATH for images and videos and hard-link them to OUTPUT_DIR.
At the end, it writes a JSON file with the gathered metadata and parses a Mustache template file.

!!! ATTENTION !!!: Ensure OUTPUT_DIR is not within the PATH structure.

Arguments:
  PATH        the path to process
  OUTPUT_DIR  an optional output path [Defaults to "./output"]

Options:
  -h, --help            print this help, then exit
  --version             print version and build, then exit
  --template_path FILE  define a custom template path [Defaults to "error-template.html" in cwd, or alongside the executable, or an internal template]
  --template_out FILE   define a path for the template output [Defaults to "OUTPUT_DIR/errors.html"]
  --json FILE           define a path for the JSON output [Defaults to "OUTPUT_DIR/files.json"]
  --print_template      prints the internal template, then exit

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

	if shouldPrintTemplate, ok := args["--print_template"].(bool); ok && shouldPrintTemplate {
		fmt.Fprintln(os.Stdout, errorTemplate)
		os.Exit(0)
		return nil
	}

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

	template, err := mustache.ParseFile(templatePath)
	if err != nil {
		if os.IsNotExist(err) {
			template, err = mustache.ParseString(errorTemplate)
			if err != nil {
				return fmt.Errorf("Cannot parse internal template file.\n%s", err.Error())
			}
		} else {
			return fmt.Errorf("Cannot parse template file: %s.\n%s", templatePath, err.Error())
		}
	}
	rendered := template.Render(data)

	if err = ioutil.WriteFile(templateOut, []byte(rendered), 0644); err != nil {
		return fmt.Errorf("Cannot render or write errored files: %s.\n%s", templatePath, err.Error())
	}
	return
}

func init() {
	registerCommand("process", "Process a directory for images and videos, while hard-linking them to an output directory.", cmdProcess)
}

// ReadImage reads image file's metadata
func ReadImage(fpath string) (meta FileMeta, err error) {
	f, err := os.Open(fpath)
	if err != nil {
		return
	}
	defer f.Close()

	defer func() {
		// alt dimensions
		if meta.Width == 0 || meta.Height == 0 {
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
		err = ErrSkipFile
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
	if _err != nil || tm.IsZero() {
		err = ErrSkipFile
		return
	}

	meta.Time = tm
	return
}

// ReadVideo reads video file's metadata
func ReadVideo(fpath string) (meta FileMeta, err error) {
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
	// 		"tags": { "creation_time": "2016-07-18T02:29:36.000000Z" }
	// 	}, ... ],
	// 	"format": {
	// 		"tags": {
	// 			"creation_time": "2016-07-18T02:29:36.000000Z",
	// 			"com.apple.quicktime.creationdate": "2016-07-18T12:29:35+1000"
	// 		}
	// 	}
	// }

	dec := json.NewDecoder(strings.NewReader(jsonStream))

	var probe Ffprobe
	if err = dec.Decode(&probe); err != nil && err != io.EOF {
		return
	}

	for _, stream := range probe.Streams {
		if stream.Width != 0 && stream.Height != 0 && stream.CodecType == "video" {
			meta.Width = stream.Width
			meta.Height = stream.Height
			break
		}
	}

	// check QtCreationDate first
	tm, _err := time.Parse(tzDateLayout, probe.Format.Tags.QtCreationDate)
	if _err != nil {
		// check Format's CreationTime second
		tm, _err = time.Parse(localDateLayout, probe.Format.Tags.CreationTime)
		if _err != nil {
			// then iterate over the Streams' CreationTimes
			for _, stream := range probe.Streams {
				tm, _err = time.Parse(localDateLayout, stream.Tags.CreationTime)
				if _err == nil {
					break
				}
			}
		}

		if _err == nil {
			tm = tm.In(time.Local)
		}
	}

	if _err != nil || tm.IsZero() {
		err = ErrSkipFile
		return
	}

	meta.Time = tm
	return
}
