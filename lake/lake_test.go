package lake

import (
	"fmt"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	type parseTest struct {
		name        string
		files       map[string]string
		errContains string
	}
	quickTest := func(name, errContains string, files ...string) parseTest {
		pt := parseTest{
			name:        name,
			errContains: errContains,
			files:       map[string]string{},
		}
		for i := 0; i < len(files); i += 2 {
			pt.files[files[i]] = files[i+1]
		}
		return pt
	}

	for _, test := range []parseTest{
		quickTest("identifier store name conflict", "Duplicate name",
			"Lakefile",
			`empty_store = "foo"
			store "empty_store" {
				inputs = []
				script = ""
			}`),
		quickTest("store and target name conflict", "Duplicate name",
			"Lakefile",
			`target "empty_store" {
				inputs = []
				script = ""
			}
			store "empty_store" {
				inputs = []
				script = ""
			}`),
		quickTest("weird block type", "unexpected block type",
			"Lakefile",
			`unexpected "empty_store" {}`),
	} {
		t.Run(test.name, func(t *testing.T) {
			err := func() error {
				var contents []*hcl.BodyContent
				var attrBodies []hcl.Body
				for file, src := range test.files {
					content, attrBody, err := parseHCL([]byte(src), file)
					if err != nil {
						return err
					}
					contents = append(contents, content)
					attrBodies = append(attrBodies, attrBody)
				}
				_, err := parseBody(contents, attrBodies)
				return err
			}()
			if err != nil {
				fmt.Println(err)
			}
			if test.errContains != "" {
				if err == nil {
					t.Fatalf("Expected error to contain %q but there was no err", test.errContains)
				}
				require.Contains(t, err.Error(), test.errContains)
			}
			if test.errContains == "" && err != nil {
				t.Fatal(err)
			}
		})
	}
}
