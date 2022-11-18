package main

import (
	"fmt"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/maxmcd/dag"
	"github.com/pkg/errors"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

type toParse struct {
	block *hcl.Block
	attr  *hcl.Attribute
}

type orderedParser struct {
	graph             dag.AcyclicGraph
	configsToParse    []toParse
	referencesToParse map[string]toParse
	evalContext       *hcl.EvalContext
	names             map[string]hcl.Range
	generatedStores   []StoreOrTarget
}

func newOrderedParser() *orderedParser {
	op := &orderedParser{
		referencesToParse: map[string]toParse{},
		names:             map[string]hcl.Range{},
		evalContext: &hcl.EvalContext{
			Functions: map[string]function.Function{},
			Variables: map[string]cty.Value{},
		},
	}
	op.evalContext.Functions["download_file"] = function.New(&function.Spec{
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
			sot := StoreOrTarget{Name: "download_file", Env: map[string]string{
				"fetch_url": "true",
				"url":       args[0].AsString(),
			}}
			op.generatedStores = append(op.generatedStores, sot)
			return sot.ctyString(), nil
		},
	})
	return op
}

func (op *orderedParser) reviewBlocks(content *hcl.BodyContent) error {
	for _, block := range content.Blocks {
		spec, found := blockSpecMap[block.Type]
		if !found {
			return errors.Errorf("Unexpected block type %q found", block.Type)
		}
		if block.Type == ConfigBlockTypeName {
			op.configsToParse = append(op.configsToParse, toParse{
				block: block,
			})
			continue
		}
		// Is "store" or "target"
		name := block.Labels[0]
		conflictRange, found := op.names[block.Labels[0]]
		if found {
			return fmt.Errorf(
				"%s: Duplicate name; The name %q has already been used at %s. Target and store names must be unique.",
				block.DefRange, name, conflictRange)
		}
		op.names[block.Labels[0]] = block.DefRange
		op.graph.Add(name)

		op.referencesToParse[name] = toParse{block: block}

		// TODO: validate correct attributes are present here, or catch later?
		// Can this catch someone up if there are variables present in an
		// unparsed attribute that we don't pick up here?
		for _, variable := range hcldec.Variables(block.Body, spec) {
			op.graph.Add(variable.RootName())
			op.graph.Connect(dag.BasicEdge(name, variable.RootName()))
		}
	}
	return nil
}

func (op *orderedParser) reviewAttributes(attrBody hcl.Body) error {
	// TODO: Confused about why this is required, docs say identified blocks are
	// removed by Body.PartialContent
	attrBody.(*hclsyntax.Body).Blocks = nil

	attributes, diags := attrBody.JustAttributes()
	if diags.HasErrors() {
		return diags
	}
	for _, attr := range attributes {
		name := attr.Name
		conflictRange, found := op.names[name]
		if found {
			return fmt.Errorf("%s: Duplicate name; The name %q has already been used at %s. Target and store names cannot conflict with attribute names", attr.Range, name, conflictRange)
		}

		op.graph.Add(name)
		variables := attr.Expr.Variables()
		op.referencesToParse[name] = toParse{attr: attr}
		for _, variable := range variables {
			op.graph.Add(variable.RootName())
			op.graph.Connect(dag.BasicEdge(name, variable.RootName()))
		}
	}
	return nil
}

func (op *orderedParser) walkGraphAndParse(directory *Directory) error {
	var lock sync.Mutex
	errs := op.graph.Walk(func(v dag.Vertex) error {
		// Force serial for now
		lock.Lock()
		defer lock.Unlock()

		name := v.(string)
		parse := op.referencesToParse[name]
		if parse.block != nil {
			var storeOrTarget StoreOrTarget

			if err := gohcl.DecodeBody(parse.block.Body, op.evalContext, &storeOrTarget); err != nil {
				return err
			}

			storeOrTarget.Name = parse.block.Labels[0]
			if parse.block.Type == StoreBlockTypeName {
				directory.Stores = append(directory.Stores, storeOrTarget)
			} else if parse.block.Type == TargetBlockTypeName {
				directory.Targets = append(directory.Targets, storeOrTarget)
			}

			op.evalContext.Variables[name] = storeOrTarget.ctyString()
		} else if parse.attr != nil {
			var diags hcl.Diagnostics
			op.evalContext.Variables[name], diags = parse.attr.Expr.Value(op.evalContext)
			if diags.HasErrors() {
				return diags
			}
		}
		return nil
	})
	if errs != nil {
		return errs[0]
	}

	for _, store := range op.generatedStores {
		directory.Stores = append(directory.Stores, store)
	}

	for _, parse := range op.configsToParse {
		var config Config
		// TODO: support other things; also this pattern is meh
		if err := gohcl.DecodeBody(parse.block.Body, op.evalContext, &config); err != nil {
			return err
		}
		directory.Configs = append(directory.Configs, config)
	}

	return nil
}
