package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

func writeJSON(files FilesList, jsonFile string) error {
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

func readJSON(jsonFile string) (files FilesList, err error) {
	inFile, err := os.Open(jsonFile)
	if err != nil {
		return
	}

	jsonBytes := json.NewDecoder(inFile)
	err = jsonBytes.Decode(&files)

	return
}
