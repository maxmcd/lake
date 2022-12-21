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

// orderedParser takes a collection of hcl blocks and attributes, parses them in
// variable dependency order, and returns a collection of named values
type orderedParser struct {
	// The graph of variable references
	graph             *dag.AcyclicGraph
	referencesToParse map[string]toParse
	generatedStores   []Recipe

	imports        map[string]map[string]Value
	perFileImports map[string]map[string]map[string]Value
	importFunc     ImportFunction

	// the package we're working on
	pkg Package

	// nameStore stores variables names for each file
	nameStore nameStore
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
func newOrderedParser(pkg Package, importFunc ImportFunction) *orderedParser {
	op := &orderedParser{
		referencesToParse: map[string]toParse{},
		graph:             &dag.AcyclicGraph{},
		nameStore:         newNameStore(pkg),
		pkg:               pkg,
		importFunc:        importFunc,
		imports:           map[string]map[string]Value{},
		perFileImports:    map[string]map[string]map[string]Value{},
	}
	return op
}

func (op *orderedParser) loadImport(filename string, iv importVal) (diags hcl.Diagnostics) {
	if _, found := op.perFileImports[filename]; !found {
		op.perFileImports[filename] = map[string]map[string]Value{}
	}
	values, found := op.imports[iv.name]
	if found {
		op.perFileImports[filename][iv.refName()] = values
		return nil
	}
	values, diags = op.importFunc(iv.name)
	op.imports[iv.name] = values
	op.perFileImports[filename][iv.refName()] = values
	return diags
}

func (op *orderedParser) reviewBlocks() (diags hcl.Diagnostics) {
	for _, file := range op.pkg.files {
		for _, block := range file.blocks {
			spec, found := blockSpecMap[block.Type]
			if !found {
				// Blocks should be validated before reaching this function
				panic(errors.Errorf("Unexpected block type %q found", block.Type))
			}
			var name string
			if block.Type == ConfigBlockTypeName {
				// TODO: what if name conflicts with store or target name?
				name = ConfigBlockTypeName
				toParse := op.referencesToParse[name]
				toParse.configs = append(toParse.configs, block)
				op.referencesToParse[name] = toParse
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
				varName := variableName(variable)
				op.graph.Add(varName)
				op.graph.Connect(dag.BasicEdge(name, varName))
			}
		}
	}
	return diags
}

func variableName(v hcl.Traversal) string {
	var sb strings.Builder
	for _, part := range v {
		switch t := part.(type) {
		case hcl.TraverseRoot:
			sb.WriteString(t.Name)
		case hcl.TraverseAttr:
			sb.WriteString("." + t.Name)
		default:
			panic(v)
		}
	}
	return sb.String()
}

func checkForThingsAboveImport(imprt *hcl.Attribute, file File) (diags hcl.Diagnostics) {
	importLocationErr := &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Invalid import location",
		Detail:   fmt.Sprintf("Import statement must be the first attribute defined in a file."),
	}
	importLine := imprt.Range.Start.Line
	for _, attr := range file.attributes {
		if attr.Name == importAttributeName {
			continue
		}
		if attr.Range.Start.Line > importLine {
			continue
		}
		var context *hcl.Range
		// Show more context if it's near
		if attr.Range.Start.Line-importLine < 10 {
			context = &hcl.Range{
				Filename: attr.Range.Filename,
				Start:    attr.Range.Start,
				End:      imprt.Range.End,
			}
		}
		importLocationErr.Subject = &imprt.Range
		importLocationErr.Context = context
		return append(diags, importLocationErr)
	}
	for _, block := range file.blocks {
		if block.DefRange.Start.Line > importLine {
			continue
		}
		var context *hcl.Range
		// Show more context if it's near
		if block.DefRange.Start.Line-importLine < 10 {
			context = &hcl.Range{
				Filename: block.DefRange.Filename,
				Start:    block.DefRange.Start,
				End:      imprt.Range.End,
			}
		}
		importLocationErr.Subject = &imprt.Range
		importLocationErr.Context = context
		return append(diags, importLocationErr)
	}
	return nil
}

type importVal struct {
	name  string
	alias string
}

func (iv importVal) refName() string {
	if iv.alias != "" {
		return iv.alias
	}
	return iv.name
}

func convertInputValue(imprt *hcl.Attribute) (vals []importVal, diags hcl.Diagnostics) {
	value, theseDiags := imprt.Expr.Value(nil)
	if diags = append(diags, theseDiags...); diags.HasErrors() {
		return nil, diags
	}

	iterator := value.ElementIterator()
	for iterator.Next() {
		_, v := iterator.Element()
		if v.Type() == cty.String {
			vals = append(vals, importVal{name: v.AsString()})
			continue
		}
		if v.LengthInt() != 1 {
			panic(v)
		}
		mapIterator := v.ElementIterator()
		for mapIterator.Next() {
			alias, name := mapIterator.Element()
			vals = append(vals, importVal{name: name.AsString(), alias: alias.AsString()})
		}
	}
	return vals, diags
}

func (op *orderedParser) loadImports() (diags hcl.Diagnostics) {

	// Range once for errors.
	for _, file := range op.pkg.files {
		imprt, found := file.attributes[importAttributeName]
		if !found {
			continue
		}
		if len(imprt.Expr.Variables()) > 0 {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Import statement cannot contain variables.",
				Subject:  &imprt.Range,
			})
		}
		diags = append(diags, checkForThingsAboveImport(imprt, file)...)
	}
	if diags.HasErrors() {
		return diags
	}
	// Range again for importing
	for _, file := range op.pkg.files {
		imprt, found := file.attributes[importAttributeName]
		if !found {
			continue
		}
		vals, theseDiags := convertInputValue(imprt)
		if diags = append(diags, theseDiags...); theseDiags.HasErrors() {
			continue
		}
		for _, val := range vals {
			diags = append(diags, op.loadImport(file.filename, val)...)
		}
	}

	return diags
}

func (op *orderedParser) reviewAttributes() (diags hcl.Diagnostics) {
	for _, file := range op.pkg.files {
		for name, attr := range file.attributes {
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

	return newWalkDecoder(op.perFileImports).walk(op.graph, op.referencesToParse)
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

// walkDecoder walks the dependency tree of variable references and parses each
// hcl block and attribute as we the values that they need to be available
type walkDecoder struct {
	values      map[string]Value
	evalContext *hcl.EvalContext
	config      Config

	imports map[string]map[string]map[string]Value
}

func newWalkDecoder(imports map[string]map[string]map[string]Value) *walkDecoder {
	return &walkDecoder{
		evalContext: &hcl.EvalContext{
			Functions: nil,
			Variables: map[string]cty.Value{},
		},
		values:  map[string]Value{},
		imports: imports,
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

	fmt.Println(mermaidGraph(graph))

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

func (wd *walkDecoder) fileEvalContext(filename string) *hcl.EvalContext {
	child := wd.evalContext.NewChild()
	child.Variables = make(map[string]cty.Value)
	for imprt, values := range wd.imports[filename] {
		child.Variables[imprt] = valueMapToCTYObject(values)
	}
	return child
}

func (wd *walkDecoder) decodeConfig(block *hcl.Block) (diags hcl.Diagnostics) {
	var config Config
	if diags := gohcl.DecodeBody(block.Body, wd.fileEvalContext(block.DefRange.Filename), &config); diags.HasErrors() {
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
	if diags := gohcl.DecodeBody(block.Body, wd.fileEvalContext(block.DefRange.Filename), &recipe); diags.HasErrors() {
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
	if wd.evalContext.Variables[name], diags = attr.Expr.Value(wd.fileEvalContext(attr.Range.Filename)); diags.HasErrors() {
		return diags
	}
	ctyVal := wd.evalContext.Variables[name]
	wd.values[name] = Value{cty: &ctyVal}
	return nil
}
