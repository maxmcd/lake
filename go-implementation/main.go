package main

import (
	"encoding/json"
	"os"

	"github.com/maxmcd/lake/go-implementation/lake"
)

func main() {
	directory, pkg, diags := lake.ParseDirectory(".", lake.TmpLoadLakeImport)
	if diags.HasErrors() {
		lake.PrintDiagnostics(pkg.FileMap(), diags)
		return
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(directory)
}
