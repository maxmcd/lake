package lake

import (
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pkg/errors"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/crypto/ssh/terminal"
)

type Directory struct {
	Configs []Config        `hcl:"config,block"`
	Stores  []StoreOrTarget `hcl:"store,block"`
	Targets []StoreOrTarget `hcl:"target,block"`
}

type Config struct {
	Shell     []string `hcl:"shell,optional"`
	Temporary string   `hcl:"temporary,optional"`
}

type StoreOrTarget struct {
	Env    map[string]string `hcl:"env,optional"`
	Inputs []string          `hcl:"inputs"`
	Name   string            `hcl:"name,label"`
	Script string            `hcl:"script"`
	Shell  []string          `hcl:"shell,optional"`
}

func (sot StoreOrTarget) hash() string {
	h := sha256.New()
	if err := json.NewEncoder(h).Encode(sot); err != nil {
		panic(err)
	}
	return bytesToBase32Hash(h.Sum(nil))
}

// bytesToBase32Hash copies nix here
// https://nixos.org/nixos/nix-pills/nix-store-paths.html
// Finally the comments tell us to compute the base32 representation of the
// first 160 bits (truncation) of a sha256 of the above string:
func bytesToBase32Hash(b []byte) string {
	var buf bytes.Buffer
	_, _ = base32.NewEncoder(base32.StdEncoding, &buf).Write(b[:20])
	return strings.ToLower(buf.String())
}

func (sot StoreOrTarget) ctyString() cty.Value {
	return cty.StringVal(fmt.Sprintf("{{ %s }}", sot.hash()))
}

var (
	ConfigBlockTypeName = "config"
	StoreBlockTypeName  = "store"
	TargetBlockTypeName = "target"
)

var configSpec = &hcldec.TupleSpec{
	&hcldec.AttrSpec{Name: "shell", Type: cty.List(cty.String), Required: false},
	&hcldec.AttrSpec{Name: "temporary", Type: cty.String, Required: false},
}

var storeOrTargetSpec = &hcldec.TupleSpec{
	&hcldec.AttrSpec{Name: "env", Type: cty.Map(cty.String), Required: false},
	&hcldec.AttrSpec{Name: "inputs", Type: cty.List(cty.String), Required: true},
	&hcldec.AttrSpec{Name: "script", Type: cty.String, Required: true},
	&hcldec.AttrSpec{Name: "shell", Type: cty.List(cty.String), Required: false},
}

var blockSpecMap = map[string]hcldec.Spec{
	TargetBlockTypeName: storeOrTargetSpec,
	StoreBlockTypeName:  storeOrTargetSpec,
	ConfigBlockTypeName: configSpec,
}

func parseHCLFile(path string) (file *hcl.File, content *hcl.BodyContent, attrBody hcl.Body, diags hcl.Diagnostics) {
	src, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, nil, nil, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  errors.Wrapf(err, "error reading %q", path).Error(),
		})
	}

	return parseHCL(src, filepath.Base(path))
}

func parseHCL(src []byte, filename string) (file *hcl.File, content *hcl.BodyContent, attrBody hcl.Body, diags hcl.Diagnostics) {
	hclFile, diags := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return hclFile, nil, nil, diags
	}
	content, attrBody, diags = parseHCLBody(hclFile.Body)
	return hclFile, content, attrBody, diags
}

func rangePointer(r hcl.Range) *hcl.Range { return &r }

func parseHCLBody(body hcl.Body) (content *hcl.BodyContent, attrBody hcl.Body, diags hcl.Diagnostics) {
	schema, _ := gohcl.ImpliedBodySchema(Directory{})
	content, attrBody, diags = body.PartialContent(schema)
	for _, block := range attrBody.(*hclsyntax.Body).Blocks {
		if _, found := blockSpecMap[block.Type]; !found {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  fmt.Sprintf("Found unexpected block type %q", block.Type),
				Subject:  rangePointer(block.DefRange()),
				Context:  rangePointer(block.Range()),
			})
		}
	}
	return content, attrBody, diags
}

func parseBody(content []*hcl.BodyContent, attrBody []hcl.Body) (directory Directory, diags hcl.Diagnostics) {
	dirParser := newOrderedParser()

	diags = append(diags, dirParser.reviewBlocks(content)...)
	diags = append(diags, dirParser.reviewAttributes(attrBody)...)

	if diags.HasErrors() {
		return Directory{}, diags
	}

	return dirParser.walkGraphAndAssembleDirectory()
}

// ParseDirectory takes a directory and searches it for Lakefiles. Those files
// are parsed and the resulting data is returned.
func ParseDirectory(path string) (d Directory, files map[string]*hcl.File, diags hcl.Diagnostics) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return Directory{}, nil, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  errors.Wrapf(err, "error attempting to read directory %q", path).Error(),
		})
	}
	var filepaths []string
	for _, entry := range entries {
		// Lakefile or *.Lakefile
		if entry.Name() == LakeFilename ||
			filepath.Ext(entry.Name()) == "."+LakeFilename {
			filepaths = append(filepaths, filepath.Join(path, entry.Name()))
		}
	}

	files = map[string]*hcl.File{}
	var contents []*hcl.BodyContent
	var attrBodies []hcl.Body
	for _, fp := range filepaths {
		hclFile, content, attrBody, theseDiags := parseHCLFile(fp)
		files[filepath.Base(fp)] = hclFile
		if theseDiags.HasErrors() {
			diags = diags.Extend(theseDiags)
			continue
		}
		contents = append(contents, content)
		attrBodies = append(attrBodies, attrBody)
	}
	if diags.HasErrors() {
		return Directory{}, nil, diags
	}

	d, diags = parseBody(contents, attrBodies)
	return d, files, diags
}

// PrintDiagnostics is an opinionated use of hcl.NewDiagnosticTextWriter that
// fetches the terminal width, determines if the output should contain color and
// prints to stderr
func PrintDiagnostics(files map[string]*hcl.File, diags hcl.Diagnostics) error {
	color := os.Getenv("NO_COLOR") == ""
	width, _, err := terminal.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		width = 80
		color = false // assume we don't have a terminal
	}
	writer := hcl.NewDiagnosticTextWriter(os.Stderr, files, uint(width), color)
	if err := writer.WriteDiagnostics(diags); err != nil {
		return err
	}
	return nil
}
