package lake

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
)

var (
	LakeFilename = "Lakefile"
)

var (
	importAttributeName = "import"
)

type ImportFunction func(name string) (values map[string]Value, diags hcl.Diagnostics)

func TmpLoadLakeImport(name string) (values map[string]Value, diags hcl.Diagnostics) {
	var projectRoot string
	do := func() error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
				if os.IsNotExist(err) {
					dir = filepath.Join(dir, "..")
					continue
				}
				return err
			}
			break
		}
		projectRoot = dir
		return nil
	}
	if err := do(); err != nil {
		return nil, hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  err.Error(),
		}}
	}
	vals, _, diags := ParseDirectory(
		filepath.Join(
			projectRoot,
			strings.TrimPrefix(name, "lake/"),
		), TmpLoadLakeImport)
	return vals, diags
}
