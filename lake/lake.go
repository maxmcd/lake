package lake

import "github.com/hashicorp/hcl/v2"

var (
	LakeFilename = "Lakefile"
)

type ImportFunction func(name string) (values map[string]Value, diags hcl.Diagnostics)
