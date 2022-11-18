package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/maxmcd/dag"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

type File struct {
	Stores  []StoreOrTarget `hcl:"store,block"`
	Configs []Config        `hcl:"config,block"`
	Targets []StoreOrTarget `hcl:"target,block"`
	Remain  hcl.Body        `hcl:",remain"`
}

type Config struct {
	Shell     []string `hcl:"shell,optional"`
	Temporary string   `hcl:"temporary,optional"`
}

var configSpec = &hcldec.TupleSpec{
	&hcldec.AttrSpec{Name: "shell", Type: cty.List(cty.String), Required: false},
	&hcldec.AttrSpec{Name: "temporary", Type: cty.String, Required: false},
}

type StoreOrTarget struct {
	Name   string   `hcl:"name,label"`
	Inputs []string `hcl:"inputs"`
	Script string   `hcl:"script"`
	Shell  []string `hcl:"shell,optional"`
}

var storeOrTargetSpec = &hcldec.TupleSpec{
	&hcldec.AttrSpec{Name: "inputs", Type: cty.List(cty.String), Required: false},
	&hcldec.AttrSpec{Name: "script", Type: cty.String, Required: true},
	&hcldec.AttrSpec{Name: "shell", Type: cty.List(cty.String), Required: false},
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

	configsToParse := []toParse{}
	referencesToParse := map[string]toParse{}

	names := map[string]hcl.Range{}
	for _, block := range content.Blocks {
		spec, found := blockSpecMap[block.Type]
		if !found {
			panic(fmt.Errorf("Unexpected block type %q found", block.Type))
		}
		if block.Type == "config" {
			configsToParse = append(configsToParse, toParse{
				block: block,
			})
			continue
		}
		// Is "store" or "target"
		name := block.Labels[0]
		conflictRange, found := names[block.Labels[0]]
		if found {
			panic(fmt.Errorf(
				"%s: Duplicate name; The name %q has already been used at %s. Target and store names must be unique.",
				block.DefRange, name, conflictRange))
		}
		names[block.Labels[0]] = block.DefRange
		graph.Add(name)

		referencesToParse[name] = toParse{block: block}

		// TODO: validate correct attributes are present here, or catch later?
		// Can this catch someone up if there are variables present in an
		// unparsed attribute that we don't pick up here?
		for _, variable := range hcldec.Variables(block.Body, spec) {
			graph.Add(variable.RootName())
			graph.Connect(dag.BasicEdge(name, variable.RootName()))
		}
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
		name := attr.Name
		conflictRange, found := names[name]
		if found {
			panic(fmt.Errorf("%s: Duplicate name; The name %q has already been used at %s. Target and store names cannot conflict with attribute names", attr.Range, name, conflictRange))
		}

		graph.Add(name)
		variables := attr.Expr.Variables()
		referencesToParse[name] = toParse{attr: attr}
		for _, variable := range variables {
			graph.Add(variable.RootName())
			graph.Connect(dag.BasicEdge(name, variable.RootName()))
		}
	}

	var file File

	var lock sync.Mutex
	errs := graph.Walk(func(v dag.Vertex) error {
		// Force serial for now
		lock.Lock()
		defer lock.Unlock()
		fmt.Println(v)
		name := v.(string)
		parse := referencesToParse[name]
		if parse.block != nil {
			var storeOrTarget StoreOrTarget
			if err := gohcl.DecodeBody(parse.block.Body, evalContext, &storeOrTarget); err != nil {
				panic(err)
			}
			storeOrTarget.Name = parse.block.Labels[0]
			if parse.block.Type == "store" {
				file.Stores = append(file.Stores, storeOrTarget)
			} else if parse.block.Type == "target" {
				file.Targets = append(file.Targets, storeOrTarget)
			}
			evalContext.Variables[name] = cty.StringVal(fmt.Sprintf("{{ %s }}", name))
		} else if parse.attr != nil {
			evalContext.Variables[name], diags = parse.attr.Expr.Value(evalContext)
			if diags.HasErrors() {
				panic(diags)
			}
		}
		return nil
	})
	if errs != nil {
		panic(errs)
	}

	for _, parse := range configsToParse {
		var config Config
		// TODO: support other things; also this pattern is meh
		if err := gohcl.DecodeBody(parse.block.Body, evalContext, &config); err != nil {
			panic(err)
		}
		file.Configs = append(file.Configs, config)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(file)
}
