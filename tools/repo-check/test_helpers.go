package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Check named test helpers, including files excluded by platform/build tags.
// Anonymous test callbacks are entrypoints, not reusable helpers.
func testHelpers(root string) error {
	return walkSources(root, func(rel string, entry fs.DirEntry) error {
		if entry.IsDir() || !strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		positions := token.NewFileSet()
		file, err := parser.ParseFile(positions, filepath.Join(root, rel), nil, 0)
		if err != nil {
			return err
		}
		testingImport := ""
		for _, imported := range file.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			if path == "testing" {
				testingImport = "testing"
				if imported.Name != nil {
					testingImport = imported.Name.Name
				}
			}
		}
		if testingImport == "" {
			return nil
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || len(function.Type.Params.List) == 0 {
				continue
			}
			parameter := function.Type.Params.List[0]
			kind := testingHandleKind(parameter.Type, testingImport)
			if kind == "" || testEntrypoint(function, kind) {
				continue
			}
			if len(parameter.Names) != 1 || parameter.Names[0].Name == "_" {
				return fmt.Errorf("%s: test helper %s requires one named testing handle", positions.Position(parameter.Pos()), function.Name.Name)
			}
			name := parameter.Names[0].Name
			if len(function.Body.List) == 0 || !helperCall(function.Body.List[0], name) {
				return fmt.Errorf("%s: test helper %s must begin with %s.Helper()", positions.Position(function.Pos()), function.Name.Name, name)
			}
		}
		return nil
	})
}

func testingHandleKind(expression ast.Expr, testingImport string) string {
	pointer, isPointer := expression.(*ast.StarExpr)
	if isPointer {
		expression = pointer.X
	}
	qualified, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	packageName, ok := qualified.X.(*ast.Ident)
	if !ok || packageName.Name != testingImport {
		return ""
	}
	kind := qualified.Sel.Name
	if (isPointer && (kind == "T" || kind == "B" || kind == "F")) || (!isPointer && kind == "TB") {
		return kind
	}
	return ""
}

func testEntrypoint(function *ast.FuncDecl, kind string) bool {
	if function.Recv != nil || function.Type.Params.NumFields() != 1 || function.Type.Results.NumFields() != 0 {
		return false
	}
	prefix := map[string]string{"T": "Test", "B": "Benchmark", "F": "Fuzz"}[kind]
	if prefix == "" || !strings.HasPrefix(function.Name.Name, prefix) {
		return false
	}
	// Match Go testing's name convention, not every name starting with Test.
	suffix := strings.TrimPrefix(function.Name.Name, prefix)
	first, _ := utf8.DecodeRuneInString(suffix)
	return suffix == "" || !unicode.IsLower(first)
}

func helperCall(statement ast.Stmt, handle string) bool {
	expression, ok := statement.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expression.X.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || method.Sel.Name != "Helper" {
		return false
	}
	receiver, ok := method.X.(*ast.Ident)
	return ok && receiver.Name == handle
}
