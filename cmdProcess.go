package main

import (
	"errors"
	"fmt"
	"github.com/docopt/docopt-go"
	"github.com/hoisie/mustache"
	"github.com/kardianos/osext"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// Img = regexp.MustCompile(`(?i).(jpe?g|png|gif)`)
	Img         = regexp.MustCompile(`(?i).(jpe?g)`)
	Vid         = regexp.MustCompile(`(?i).(mov|mp4|m4v)`)
	SkipFile    = errors.New("skip this file")
	NoPath      = errors.New("no path")
	UnknownFile = errors.New("unknown file")
)

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
