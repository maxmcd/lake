package lake

import (
	"testing"

	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/stretchr/testify/assert"
)

func TestProjectLakefile(t *testing.T) {
	_, files, diags := ParseDirectory("../")
	if diags.HasErrors() {
		if err := PrintDiagnostics(files, diags); err != nil {
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
