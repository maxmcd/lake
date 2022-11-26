package lake

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/maxmcd/dag"
	"github.com/pkg/errors"
)

type toParse struct {
	block   *hcl.Block
	attr    *hcl.Attribute
	configs []*hcl.Block
}

type orderedParser struct {
	graph             *dag.AcyclicGraph
	referencesToParse map[string]toParse
	names             map[string]hcl.Range
	generatedStores   []Recipe
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
		graph:             &dag.AcyclicGraph{},
	}
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
			var name string
			if block.Type == ConfigBlockTypeName {
				// TODO: put illegal chars in name?
				name = ConfigBlockTypeName
				toParse := op.referencesToParse[name]
				toParse.configs = append(toParse.configs, block)
				op.referencesToParse[name] = toParse
			} else {
				// Is "store" or "target"
				name = block.Labels[0]
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
				op.referencesToParse[name] = toParse{block: block}
			}

			op.graph.Add(name)
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

func (op *orderedParser) walkGraphAndAssembleDirectory() (values map[string]Value, diags hcl.Diagnostics) {
	if diags := op.checkGraphForCycles(); diags.HasErrors() {
		return nil, diags
	}

	return newWalkDecoder().walk(op.graph, op.referencesToParse)
}
