package main

import (
	"fmt"
	"os"
	"path/filepath"
)

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
