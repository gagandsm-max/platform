package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// Production must not enqueue a fabricated GOOD sensor heartbeat under an
// invented tenant or poll-job grant. Live health remains in health.NewHealthServer;
// telemetry producers require authoritative assignments and tenant identity.
func TestProductionSensorDoesNotWireLegacySyntheticCollector(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse production entrypoint: %v", err)
	}
	for _, imp := range parsed.Imports {
		name, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatalf("parse import path: %v", err)
		}
		if name == "platform/sensor/internal/collector" {
			t.Error("production entrypoint imports legacy synthetic collector")
		}
	}
	ast.Inspect(parsed, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "NewCollectorEngine" {
			return true
		}
		if owner, ok := sel.X.(*ast.Ident); ok && owner.Name == "collector" {
			t.Error("production startup constructs fake GOOD/default-tenant collector")
		}
		return true
	})
}
