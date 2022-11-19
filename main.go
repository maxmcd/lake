package main

import (
	"encoding/json"
	"os"

	"github.com/maxmcd/lake/lake"
)

func main() {
	directory, err := lake.ParseDirectory(".")
	if err != nil {
		panic(err)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(directory)
}
