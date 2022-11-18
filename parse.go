package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pkg/errors"
	"github.com/zclconf/go-cty/cty"
)

type Directory struct {
	Stores  []StoreOrTarget `hcl:"store,block"`
	Configs []Config        `hcl:"config,block"`
	Targets []StoreOrTarget `hcl:"target,block"`
}

type Config struct {
	Shell     []string `hcl:"shell,optional"`
	Temporary string   `hcl:"temporary,optional"`
}

type StoreOrTarget struct {
	Name   string   `hcl:"name,label"`
	Inputs []string `hcl:"inputs"`
	Script string   `hcl:"script"`
	Shell  []string `hcl:"shell,optional"`
}

func (sot StoreOrTarget) hash() string {
	h := sha256.New()
	if err := json.NewEncoder(h).Encode(sot); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
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
	&hcldec.AttrSpec{Name: "inputs", Type: cty.List(cty.String), Required: false},
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

	schema, _ := gohcl.ImpliedBodySchema(Directory{})
	content, attrBody, diags = hclFile.Body.PartialContent(schema)
	if diags.HasErrors() {
		return nil, nil, diags
	}

	return content, attrBody, nil
}

func parseBody(content *hcl.BodyContent, attrBody hcl.Body) (directory Directory, err error) {
	dirParser := newOrderedParser()

	if err := dirParser.reviewBlocks(content); err != nil {
		return Directory{}, nil
	}
	if err := dirParser.reviewAttributes(attrBody); err != nil {
		return Directory{}, nil
	}

	if err := dirParser.walkGraphAndParse(&directory); err != nil {
		return Directory{}, err
	}
	return directory, nil

}

// ParseDirectory takes a directory and searches it for Lakefiles. Those files
// are parsed and the resulting data is returned.
func ParseDirectory(path string) (Directory, error) {
	lakepath := filepath.Join(path, "Lakefile")
	// TODO: support multiple files

	content, attrBody, err := parseHCLFile(lakepath)
	if err != nil {
		return Directory{}, err
	}

	return parseBody(content, attrBody)
}
