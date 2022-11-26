package lake

import (
	"fmt"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/maxmcd/dag"
	"github.com/zclconf/go-cty/cty"
)

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

// TODO: explain what this function is doing
func insertConfigDecendants(graph *dag.AcyclicGraph, referencesToParse map[string]toParse) {
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

func (wd *walkDecoder) walk(graph *dag.AcyclicGraph, referencesToParse map[string]toParse) (values map[string]Value, diags hcl.Diagnostics) {
	insertConfigDecendants(graph, referencesToParse)
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
			// TODO: cite previous occurence
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
