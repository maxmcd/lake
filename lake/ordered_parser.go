package lake

import (
	"fmt"
	"strings"
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

func errDuplicateName(name string, conflictRange hcl.Range, subject, context *hcl.Range) *hcl.Diagnostic {
	return &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Duplicate name",
		Detail: fmt.Sprintf(
			"The name %q has already been used at %s. Target, store, and attribute names must be unique.",
			name, conflictRange),
		Subject: subject,
		Context: context,
	}
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

func (op *orderedParser) reviewBlocks(contents []*hcl.BodyContent) (diags hcl.Diagnostics) {
	for _, content := range contents {
		for _, block := range content.Blocks {
			spec, found := blockSpecMap[block.Type]
			if !found {
				// Blocks should be validated before reaching this function
				panic(errors.Errorf("Unexpected block type %q found", block.Type))
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
				diags = append(diags, errDuplicateName(
					name,
					conflictRange,
					rangePointer(block.DefRange),
					rangePointer(block.Body.(*hclsyntax.Body).SrcRange)),
				)
				continue
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
	}
	return diags
}

func (op *orderedParser) reviewAttributes(attrBodies []hcl.Body) (diags hcl.Diagnostics) {
	for _, attrBody := range attrBodies {
		// TODO: Confused about why this is required, docs say identified blocks are
		// removed by Body.PartialContent
		attrBody.(*hclsyntax.Body).Blocks = nil

		attributes, theseDiags := attrBody.JustAttributes()
		if theseDiags.HasErrors() {
			diags = append(diags, theseDiags...)
			continue
		}
		for _, attr := range attributes {
			name := attr.Name
			conflictRange, found := op.names[name]
			if found {
				diags = append(diags, errDuplicateName(
					name,
					conflictRange,
					rangePointer(attr.Range),
					nil,
				))

				continue
			}
			op.names[name] = attr.Range
			op.graph.Add(name)
			variables := attr.Expr.Variables()
			op.referencesToParse[name] = toParse{attr: attr}
			for _, variable := range variables {
				op.graph.Add(variable.RootName())
				op.graph.Connect(dag.BasicEdge(name, variable.RootName()))
			}
		}
	}
	return diags
}

func (op *orderedParser) checkGraphForCycles() (diags hcl.Diagnostics) {
	// Report errors for cycles
	for _, cycles := range op.graph.Cycles() {
		stringCycles := []string{}
		// TODO: The order this is reported in is random, can we pick a
		// deterministic starting point
		for _, id := range cycles {
			stringCycles = append(stringCycles, id.(string))
		}
		first_identifier := cycles[0]
		var subject *hcl.Range
		var context *hcl.Range

		parse := op.referencesToParse[first_identifier.(string)]
		if parse.block != nil {
			context = &parse.block.Body.(*hclsyntax.Body).SrcRange
			subject = &parse.block.DefRange
		} else if parse.attr != nil {
			subject = &parse.attr.Range
		}
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Circular reference",
			Detail:   fmt.Sprintf("Identifiers %s create a circular reference.", strings.Join(stringCycles, " -> ")),
			Subject:  subject,
			Context:  context,
		})
	}
	return diags
}

func (op *orderedParser) walkGraphAndAssembleDirectory() (directory Directory, diags hcl.Diagnostics) {
	if diags := op.checkGraphForCycles(); diags.HasErrors() {
		return Directory{}, diags
	}

	var lock sync.Mutex
	errs := op.graph.Walk(func(v dag.Vertex) error {
		// Force serial for now
		lock.Lock()
		defer lock.Unlock()

		name := v.(string)
		parse := op.referencesToParse[name]

		if parse.block != nil {
			var storeOrTarget StoreOrTarget

			if diags := gohcl.DecodeBody(parse.block.Body, op.evalContext, &storeOrTarget); diags.HasErrors() {
				for _, diag := range diags {
					// Add more context to error
					diag.Context = &parse.block.Body.(*hclsyntax.Body).SrcRange
				}
				return diags
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
			if op.evalContext.Variables[name], diags = parse.attr.Expr.Value(op.evalContext); diags.HasErrors() {
				return diags
			}
			return nil
		}
		return nil
	})
	for _, err := range errs {
		diags = diags.Extend(err.(hcl.Diagnostics))
	}

	if diags.HasErrors() {
		return Directory{}, diags
	}

	for _, store := range op.generatedStores {
		directory.Stores = append(directory.Stores, store)
	}

	for _, parse := range op.configsToParse {
		var config Config
		if err := gohcl.DecodeBody(parse.block.Body, op.evalContext, &config); err != nil {
			return Directory{}, diags.Extend(err)
		}
		// TODO: merge configs
		directory.Configs = append(directory.Configs, config)
	}

	return directory, diags
}
