package main

import (
	"encoding/json"
	"os"
)

func main() {
	directory, err := ParseDirectory(".")
	if err != nil {
		panic(err)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(directory)
}
