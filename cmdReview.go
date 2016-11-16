package main

// NOTE(evo): this command is still WIP

import (
	"fmt"
	"github.com/docopt/docopt-go"
	"net/http"
	"os"
	"path"
)

func BrowseDirectory(w http.ResponseWriter, r *http.Request) {
	upath := r.URL.Path

	// TODO(evo): process directory and output template
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(upath))
}

func initServer(port string) error {
	http.Handle("/", http.HandlerFunc(BrowseDirectory))

	// http.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("static"))))

	fmt.Printf("Go to http://127.0.0.1:%s/", port)

	return http.ListenAndServe(fmt.Sprint(":", port), nil)
}

func cmdReview(argv []string) error {
	usage := fmt.Sprint(`Usage: `, AppName, ` review
		[--json FILE]
		[--port PORT]
		[PATH]
       `, AppName, ` review -h | --help

Start a server to review images and videos at PATH.
Read a JSON file to allow reviewing of unknown, errored or skipped files.

Arguments:
  PATH        the path to review [Defaults to cwd]

Options:
  -h, --help   print this help, then exit
  --json FILE  define the path for the JSON file [Defaults to "PATH/files.json"]
  --port PORT  the port to run the server on [Defaults to 8080]

Notes:
  An environment variable "PORT" will be the checked, before falling back to the default.
`)

	args, _ := docopt.Parse(usage, argv, true, "", false)
	// fmt.Println(args)

	root, err := getRoot(args, true)
	if err != nil {
		return err
	}

	var jsonFile string
	if str, ok := args["--json"].(string); ok {
		jsonFile = str
	} else {
		jsonFile = path.Join(root, "files.json")
	}
	files, err := readJSON(jsonFile)

	// TODO(evo): provide files to server
	fmt.Printf("#files: %v\n", len(files))

	port, ok := args["--port"].(string)
	if !ok || port == "" {
		port = os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
	}

	// @see for go generate and embedding files at compile time: http://stackoverflow.com/a/29500100/1484467

	return initServer(port)
}

func init() {
	registerCommand("review", "Start a server to review processed files.", cmdReview)
}
