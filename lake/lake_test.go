package lake

import (
	"io/ioutil"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testFile struct {
	Tests []test `hcl:"test,block"`
}
type test struct {
	Name        string         `hcl:"name,label"`
	ErrContains string         `hcl:"err_contains,optional"`
	Files       []testCaseFile `hcl:"file,block"`
}
type testCaseFile struct {
	Name string   `hcl:"name,label"`
	Body hcl.Body `hcl:",remain"`
}

func TestTestHCL(t *testing.T) {
	src, err := ioutil.ReadFile("./test.hcl")
	if err != nil {
		t.Fatal(err)
	}
	hclFile, diags := hclsyntax.ParseConfig(src, "test.hcl", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	var tf testFile
	if err := gohcl.DecodeBody(hclFile.Body, nil, &tf); err != nil {
		t.Fatal(err)
	}

	for _, test := range tf.Tests {
		t.Run(test.Name, func(t *testing.T) {
			err := func() error {
				var contents []*hcl.BodyContent
				var attrBodies []hcl.Body
				for _, body := range test.Files {
					// TODO: fix this so filename is accurate relative to the
					// test case in err messages
					content, attrBody, err := parseHCLBody(body.Body)
					if err != nil {
						return err
					}
					contents = append(contents, content)
					attrBodies = append(attrBodies, attrBody)
				}
				_, err := parseBody(contents, attrBodies)
				return err
			}()
			if test.ErrContains != "" {
				if err == nil {
					t.Fatalf("Expected error to contain %q but there was no err", test.ErrContains)
				}
				require.Contains(t, err.Error(), test.ErrContains)
			}
			if test.ErrContains == "" && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProjectLakefile(t *testing.T) {
	_, err := ParseDirectory("../")
	if err != nil {
		t.Fatal(err)
	}
}

// TestConfirmSchemaMatch confirms that our struct schema mirrors our spec
func TestConfirmSchemaMatch(t *testing.T) {
	// TODO: figure out how to use a single source of truth so that maintaining
	// these mappings isn't necessary
	{
		schema, _ := gohcl.ImpliedBodySchema(StoreOrTarget{})
		assert.Equal(t, schema, hcldec.ImpliedSchema(storeOrTargetSpec))
	}
	{
		schema, _ := gohcl.ImpliedBodySchema(Config{})
		assert.Equal(t, schema, hcldec.ImpliedSchema(configSpec))
	}
}
