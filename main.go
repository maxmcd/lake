package main

import (
	"encoding/json"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/maxmcd/lake/lake"
	"github.com/zclconf/go-cty/cty"
)

func main() {
	directory, pkg, diags := lake.ParseDirectory(".", func(name string) (values map[string]lake.Value, diags hcl.Diagnostics) {
		return map[string]lake.Value{
			"fish": lake.ValueFromCTY(cty.StringVal("carp")),
		}, nil
	})
	if diags.HasErrors() {
		lake.PrintDiagnostics(pkg.FileMap(), diags)
		return
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(directory)
}
