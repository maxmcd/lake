package lake

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/stretchr/testify/assert"
	"github.com/zclconf/go-cty/cty"
)

func TestProjectLakefile(t *testing.T) {
	_, pkg, diags := ParseDirectory("../", func(name string) (values map[string]Value, diags hcl.Diagnostics) {
		return map[string]Value{
			"fish": ValueFromCTY(cty.StringVal("carp")),
		}, nil
	})
	if diags.HasErrors() {
		if err := PrintDiagnostics(pkg.FileMap(), diags); err != nil {
			t.Fatal(err)
		}
		t.Fatal(diags)
	}
}

// TestConfirmSchemaMatch confirms that our struct schema mirrors our spec
func TestConfirmSchemaMatch(t *testing.T) {
	// TODO: figure out how to use a single source of truth so that maintaining
	// these mappings isn't necessary
	{
		schema, _ := gohcl.ImpliedBodySchema(Recipe{})
		assert.Equal(t, schema, hcldec.ImpliedSchema(recipeSpec))
	}
	{
		schema, _ := gohcl.ImpliedBodySchema(Config{})
		assert.Equal(t, schema, hcldec.ImpliedSchema(configSpec))
	}
}
