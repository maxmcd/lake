package main

import (
	"fmt"
	"io/ioutil"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/hcl/v2/hclsimple"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/maxmcd/dag"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

type File struct {
	Stores  []StoreOrTarget `hcl:"store,block"`
	Configs []Config        `hcl:"config,block"`
	Targets []StoreOrTarget `hcl:"targets,block"`
	Remain  hcl.Body        `hcl:",remain"`
}

type Config struct {
	Shell     []string `hcl:"shell,optional"`
	Temporary string   `hcl:"temporary,optional"`
}

var configSpec = &hcldec.TupleSpec{
	&hcldec.AttrSpec{"shell", cty.List(cty.String), false},
	&hcldec.AttrSpec{"temporary", cty.String, false},
}

type StoreOrTarget struct {
	Name   string   `hcl:"name,label"`
	Inputs []string `hcl:"inputs"`
	Script string   `hcl:"script"`
	Shell  []string `hcl:"shell,optional"`
}

var storeOrTargetSpec = &hcldec.TupleSpec{
	&hcldec.AttrSpec{"inputs", cty.List(cty.String), false},
	&hcldec.AttrSpec{"script", cty.String, true},
	&hcldec.AttrSpec{"shell", cty.List(cty.String), false},
}

var blockSpecMap = map[string]hcldec.Spec{
	"target": storeOrTargetSpec,
	"store":  storeOrTargetSpec,
	"config": configSpec,
}

type toParse struct {
	block *hcl.Block
	attr  *hcl.Attribute
}

func main() {

	// Parse file
	// Ensure store and target blocks don't have identical names
	// Find each attr or block and note their variable references
	// Create graph with blocks and attr's pointing to their variable dependencies
	// descend graph and parse as variables become available

	src, err := ioutil.ReadFile("Lakefile")
	if err != nil {
		panic(err)
	}
	hclFile, diags := hclsyntax.ParseConfig(src, "Lakefile", hcl.Pos{1, 1, 0})
	if diags.HasErrors() {
		panic(diags)
	}
	var file File

	schema, _ := gohcl.ImpliedBodySchema(File{})
	content, attrBody, diags := hclFile.Body.PartialContent(schema)
	if diags.HasErrors() {
		panic(diags)
	}

	evalContext := &hcl.EvalContext{
		Functions: map[string]function.Function{
			"download_file": function.New(&function.Spec{
				Description: `Downloads a file`,
				Params: []function.Parameter{
					{
						Name:             "url",
						Type:             cty.String,
						AllowDynamicType: false,
					},
				},
				Type: function.StaticReturnType(cty.String),
				Impl: func(args []cty.Value, retType cty.Type) (ret cty.Value, err error) {
					return args[0], nil
				},
			}),
		},
		Variables: map[string]cty.Value{},
	}

	graph := dag.AcyclicGraph{}
	graph.Add(1)

	unreferencableToParse := []toParse{}
	referencesToParse := map[string]toParse{}

	names := map[string]*hcl.Block{}
	for _, block := range content.Blocks {
		spec, found := blockSpecMap[block.Type]
		if !found {
			panic(fmt.Errorf("Unexpected block type %q found", block.Type))
		}
		if block.Type == "config" {
			unreferencableToParse = append(unreferencableToParse, toParse{
				block: block,
			})
			continue
		}
		// Is "store" or "target"
		name := block.Labels[0]
		conflictBlock, found := names[block.Labels[0]]
		if found {
			panic(fmt.Errorf("%s: Duplicate name; The name %q has already been used at %s. Target and store names must be unique.", block.DefRange, name, conflictBlock.DefRange))
		}
		names[block.Labels[0]] = block

		referencesToParse[name] = toParse{block: block}
		// TODO: validate correct attributes are present here, or catch later?
		// Can this catch someone up?
		resp := hcldec.Variables(block.Body, spec)
		fmt.Println(resp)

	}

	// TODO: Confused about why this is required, docs say identified blocks are
	// removed by Body.PartialContent
	attrBody.(*hclsyntax.Body).Blocks = nil

	attributes, diags := attrBody.JustAttributes()
	if diags.HasErrors() {
		fmt.Println(diags.Errs())
		panic(diags)
	}
	for _, attr := range attributes {
		variables := attr.Expr.Variables()
		fmt.Println(variables)
		for _, v := range variables {
			fmt.Println(v, v.RootName())
		}
		// fmt.Println(variables[0].RootName())
	}

	if diags := gohcl.DecodeBody(hclFile.Body, evalContext, &file); diags.HasErrors() {
		fmt.Println(diags.Errs())
		panic(diags)
	}
	fmt.Println(hclFile)
	_ = file
	_ = hclsimple.Decode
}
