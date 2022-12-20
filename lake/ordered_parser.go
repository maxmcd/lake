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
)

type toParse struct {
	block   *hcl.Block
	attr    *hcl.Attribute
	configs []*hcl.Block
}

type orderedParser struct {
	graph             *dag.AcyclicGraph
	referencesToParse map[string]toParse
	generatedStores   []Recipe
	pkg               Package
	nameStore         nameStore
}

func errDuplicateName(name string, conflictRange hcl.Range, subject, context *hcl.Range) *hcl.Diagnostic {
	return &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Duplicate name",
		Detail: fmt.Sprintf(
			"The name %q has already been used at %s. Target, store, and argument names must be unique.",
			name, conflictRange),
		Subject: subject,
		Context: context,
	}
}
func newOrderedParser(pkg Package) *orderedParser {
	op := &orderedParser{
		referencesToParse: map[string]toParse{},
		graph:             &dag.AcyclicGraph{},
		nameStore:         newNameStore(pkg),
		pkg:               pkg,
	}
	return op
}

type nameStore map[string]map[string]hcl.Range

func newNameStore(pkg Package) nameStore {
	ns := make(nameStore)

	for _, file := range pkg.files {
		ns[file.filename] = make(map[string]hcl.Range)
	}
	return ns
}

func (ns nameStore) addImport(filename, name string, block *hcl.Block) (diags hcl.Diagnostics) {
	conflictRange, found := ns[filename][name]
	if found {
		diags = append(diags, errDuplicateName(
			name,
			conflictRange,
			rangePointer(block.DefRange),
			rangePointer(block.Body.(*hclsyntax.Body).SrcRange)),
		)
		return diags
	}
	ns[filename][name] = block.DefRange
	return nil
}
func (ns nameStore) addBlock(name string, block *hcl.Block) (diags hcl.Diagnostics) {
	for filename, fileVars := range ns {
		conflictRange, found := fileVars[name]
		if found {
			diags = append(diags, errDuplicateName(
				name,
				conflictRange,
				rangePointer(block.DefRange),
				rangePointer(block.Body.(*hclsyntax.Body).SrcRange)),
			)
			continue
		}
		ns[filename][name] = block.DefRange
	}
	return diags
}

func (ns nameStore) addAttr(name string, attr *hcl.Attribute) (diags hcl.Diagnostics) {
	for filename, fileVars := range ns {
		conflictRange, found := fileVars[name]
		if found {
			diags = append(diags, errDuplicateName(
				name,
				conflictRange,
				rangePointer(attr.Range),
				nil,
			))

			continue
		}
		ns[filename][name] = attr.Range
	}
	return diags
}

func (op *orderedParser) reviewBlocks() (diags hcl.Diagnostics) {
	for _, file := range op.pkg.files {
		for _, block := range file.content.Blocks {
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
			} else if block.Type == ImportBlockTypeName {
				// TODO: this won't work
				diags = append(diags, op.nameStore.addImport(file.filename, block.Labels[0], block)...)
				op.referencesToParse[block.Labels[0]] = toParse{block: block}
			} else {
				// Is "store" or "target"
				name = block.Labels[0]
				diags = append(diags, op.nameStore.addBlock(name, block)...)
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

func (op *orderedParser) reviewAttributes() (diags hcl.Diagnostics) {
	for _, file := range op.pkg.files {
		// TODO: Confused about why this is required, docs say identified blocks are
		// removed by Body.PartialContent
		file.attrBody.(*hclsyntax.Body).Blocks = nil

		attributes, theseDiags := file.attrBody.JustAttributes()
		if theseDiags.HasErrors() {
			diags = append(diags, theseDiags...)
			continue
		}
		for _, attr := range attributes {
			name := attr.Name
			diags = append(diags, op.nameStore.addAttr(name, attr)...)
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

type walkDecoder struct {
	values      map[string]Value
	evalContext *hcl.EvalContext
	config      Config
}

func newWalkDecoder() *walkDecoder {
	return &walkDecoder{
		evalContext: &hcl.EvalContext{
			Functions: nil,
			Variables: map[string]cty.Value{},
		},
		values: map[string]Value{},
	}
}

// insertConfigDescendants patches our graph so that things that depend on config
// values depend on the config in the graph. Config values are intended to be
// defaults values for recipes that don't have those values defined. First we
// must find the recipes that are used as inputs to the config and then we must
// mark all other recipes as recipients of the config values. Keep in mind that
// individual config attributes might have different dependency relationships
// and might need to be handled separately in the future.
func insertConfigDescendants(graph *dag.AcyclicGraph, referencesToParse map[string]toParse) {
	// Skip if we have no configs
	if _, found := referencesToParse[ConfigBlockTypeName]; !found {
		return
	}
	ancestorNames := map[string]struct{}{ConfigBlockTypeName: {}}
	set, _ := graph.Ancestors(ConfigBlockTypeName) // This seems like it cannot error?
	for nameInterface := range set {
		ancestorNames[nameInterface.(string)] = struct{}{}
	}
	for _, vtx := range graph.Vertices() {
		name := vtx.(string)
		if _, found := ancestorNames[name]; found {
			continue
		}
		graph.Connect(dag.BasicEdge(name, ConfigBlockTypeName))
	}
}

func mermaidGraph(graph *dag.AcyclicGraph) string {
	sb := strings.Builder{}
	sb.WriteString("graph TD;\n")
	for _, edge := range graph.Edges() {
		fmt.Fprintf(&sb, "    %s --> %s\n", dag.VertexName(edge.Source()), dag.VertexName(edge.Target()))
	}
	return sb.String()
}

func (wd *walkDecoder) walk(graph *dag.AcyclicGraph, referencesToParse map[string]toParse) (
	values map[string]Value, diags hcl.Diagnostics) {
	insertConfigDescendants(graph, referencesToParse)
	var lock sync.Mutex
	errs := graph.Walk(func(v dag.Vertex) error {
		// Force serial for now
		lock.Lock()
		defer lock.Unlock()

		name := v.(string)
		parse := referencesToParse[name]

		switch {
		case len(parse.configs) > 0:
			for _, block := range parse.configs {
				if diags := wd.decodeConfig(block); diags.HasErrors() {
					return diags
				}
			}
		case parse.block != nil:
			if diags := wd.decodeRecipe(name, parse.block); diags.HasErrors() {
				return diags
			}
		case parse.attr != nil:
			if diags := wd.decodeAttribute(name, parse.attr); diags.HasErrors() {
				return diags
			}
		}
		return nil
	})
	for _, err := range errs {
		diags = diags.Extend(err.(hcl.Diagnostics))
	}

	return wd.values, diags
}

func (wd *walkDecoder) decodeConfig(block *hcl.Block) (diags hcl.Diagnostics) {
	var config Config
	if diags := gohcl.DecodeBody(block.Body, wd.evalContext, &config); diags.HasErrors() {
		return diags
	}
	if len(wd.config.Shell) > 0 && len(config.Shell) > 0 {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Conflicting config value",
			// TODO: cite previous occurrence
			Detail: fmt.Sprintf("Config values are global and can only be defined once per directory."),
			// TODO: specific attribute range
			Subject: &block.DefRange,
			Context: &block.Body.(*hclsyntax.Body).SrcRange,
		})
	} else {
		wd.config.Shell = config.Shell
	}

	return diags
}

func (wd *walkDecoder) decodeRecipe(name string, block *hcl.Block) (diags hcl.Diagnostics) {
	var recipe Recipe
	if diags := gohcl.DecodeBody(block.Body, wd.evalContext, &recipe); diags.HasErrors() {
		for _, diag := range diags {
			// Add more context to error
			diag.Context = &block.Body.(*hclsyntax.Body).SrcRange
		}
		return diags
	}

	recipe.Name = name
	if block.Type == StoreBlockTypeName {
		recipe.IsStore = true
	}
	if len(recipe.Shell) == 0 {
		recipe.Shell = wd.config.Shell
	}
	wd.values[name] = Value{recipe: &recipe}
	wd.evalContext.Variables[name] = recipe.ctyString()

	return nil
}

func (wd *walkDecoder) decodeAttribute(name string, attr *hcl.Attribute) (diags hcl.Diagnostics) {
	if wd.evalContext.Variables[name], diags = attr.Expr.Value(wd.evalContext); diags.HasErrors() {
		return diags
	}
	ctyVal := wd.evalContext.Variables[name]
	wd.values[name] = Value{cty: &ctyVal}
	return nil
}
