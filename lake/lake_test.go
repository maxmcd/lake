package lake

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
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

func hclRangeBytes(rng hcl.Range, file []byte) (out []byte) {
	scanner := bufio.NewScanner(bytes.NewBuffer(file))
	lineNo := 1

	append := func(lineNo int, line []byte) {
		switch {
		case lineNo == rng.Start.Line:
			out = append(out, line[rng.Start.Column-1:]...)
		case lineNo > rng.Start.Line && lineNo < rng.End.Line:
			out = append(out, line...)
		case lineNo == rng.End.Line:
			out = append(out, line[:rng.End.Column-1]...)
		default:
			return
		}
		out = append(out, '\n')
	}

	for scanner.Scan() {
		line := scanner.Bytes()
		append(lineNo, line)
		lineNo++
	}
	return out
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
	if diags := gohcl.DecodeBody(hclFile.Body, nil, &tf); diags.HasErrors() {
		t.Fatal(diags)
	}

	for _, test := range tf.Tests {
		t.Run(test.Name, func(t *testing.T) {
			files := map[string]*hcl.File{}
			diags := func() hcl.Diagnostics {
				var diags hcl.Diagnostics
				var pkg Package
				for _, body := range test.Files {
					syntaxBody := body.Body.(*hclsyntax.Body)

					// Move to within the braces around the closure bytes
					rangeWithinBody := syntaxBody.SrcRange
					rangeWithinBody.Start.Line++
					rangeWithinBody.Start.Column = 1
					rangeWithinBody.End.Column = 1

					file, theseDiags := parseHCL(hclRangeBytes(rangeWithinBody, src), body.Name)
					files[body.Name] = file.file
					if theseDiags.HasErrors() {
						diags = diags.Extend(theseDiags)
					}
					pkg.files = append(pkg.files, file)
				}
				if diags.HasErrors() {
					return diags
				}
				_, diags = parseBody(pkg)
				return diags
			}()
			if test.ErrContains != "" {
				if !diags.HasErrors() {
					t.Fatalf("Expected error to contain %q but there was no err", test.ErrContains)
				}
				if pass := assert.Contains(t, fmt.Sprint(diags.Errs()), test.ErrContains); !pass {
					if err := PrintDiagnostics(files, diags); err != nil {
						t.Fatal(err)
					}
				}
			}
			if test.ErrContains == "" && diags.HasErrors() {
				if err := PrintDiagnostics(files, diags); err != nil {
					t.Fatal(err)
				}
				t.Fatal(diags)
			}
		})
	}
}
