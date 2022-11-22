package main

import (
	"encoding/json"
	"os"

	"github.com/maxmcd/lake/lake"
)

func main() {
	directory, files, diags := lake.ParseDirectory(".")
	if diags.HasErrors() {
		lake.PrintDiagnostics(files, diags)
		return
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(directory)
}
