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

func parseHCLFile(path string) (content *hcl.BodyContent, attrBody hcl.Body, err error) {
	src, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error reading %q", path)
	}
	return parseHCL(src, filepath.Base(path))
}

func parseHCL(src []byte, filename string) (content *hcl.BodyContent, attrBody hcl.Body, err error) {
	hclFile, diags := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, nil, diags
	}
	return parseHCLBody(hclFile.Body)
}

func parseHCLBody(body hcl.Body) (content *hcl.BodyContent, attrBody hcl.Body, err error) {
	schema, _ := gohcl.ImpliedBodySchema(Directory{})
	content, attrBody, diags := body.PartialContent(schema)
	for _, block := range attrBody.(*hclsyntax.Body).Blocks {
		if _, found := blockSpecMap[block.Type]; !found {
			return nil, nil, errors.Errorf("%s: Found unexpected block type %q", block.DefRange(), block.Type)
		}
	}
	if diags.HasErrors() {
		return nil, nil, diags
	}

	return content, attrBody, nil
}

func parseBody(content []*hcl.BodyContent, attrBody []hcl.Body) (directory Directory, err error) {
	dirParser := newOrderedParser()
	if err := dirParser.reviewBlocks(content); err != nil {
		return Directory{}, err
	}

	if err := dirParser.reviewAttributes(attrBody); err != nil {
		return Directory{}, err
	}

	return dirParser.walkGraphAndAssembleDirectory()
}

// ParseDirectory takes a directory and searches it for Lakefiles. Those files
// are parsed and the resulting data is returned.
func ParseDirectory(path string) (Directory, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return Directory{}, errors.Wrapf(err, "error attempting to read directory %q", path)
	}
	var files []string
	for _, entry := range entries {
		// Lakefile or *.Lakefile
		if entry.Name() == LakeFilename ||
			filepath.Ext(entry.Name()) == "."+LakeFilename {
			files = append(files, filepath.Join(path, entry.Name()))
		}
	}

	var contents []*hcl.BodyContent
	var attrBodies []hcl.Body
	for _, file := range files {
		content, attrBody, err := parseHCLFile(file)
		if err != nil {
			return Directory{}, errors.Wrapf(err, "error parsing hcl file for %q", file)
		}
		contents = append(contents, content)
		attrBodies = append(attrBodies, attrBody)
	}

	return parseBody(contents, attrBodies)
}
