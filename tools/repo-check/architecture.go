package main

import (
	"fmt"
	"go/build"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func walkSources(root string, visit func(string, fs.DirEntry) error) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() && (rel == ".git" || rel == ".cache" || rel == "bin" || rel == "dist") {
			return filepath.SkipDir
		}
		if rel == "." {
			return nil
		}
		return visit(rel, entry)
	})
}

func moduleName(root string) (string, error) {
	moduleFile, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(moduleFile), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("missing module directive")
}

type source struct {
	path, dir, layer string
	imports          []string
	externalTest     bool
}

func layerOf(dir string) string {
	parts := strings.Split(dir, "/")
	if len(parts) >= 2 && parts[0] == "internal" {
		switch parts[1] {
		case "domain", "ports", "application", "cli", "composition":
			return parts[1]
		case "adapters":
			if len(parts) >= 3 {
				return "adapter:" + parts[2]
			}
		}
	}
	if len(parts) >= 2 && parts[0] == "cmd" {
		return "cmd"
	}
	if parts[0] == "tools" {
		return "tools"
	}
	if parts[0] == "tests" {
		return "tests"
	}
	return ""
}

func permits(from, to string) bool {
	if to == "" {
		return false
	}
	if from == "tests" {
		return to != "tools" && to != "tests" && to != "cmd"
	}
	if from == to {
		return true
	}
	switch from {
	case "ports":
		return to == "domain"
	case "application":
		return to == "domain" || to == "ports"
	case "cli":
		return to == "domain" || to == "ports" || to == "application"
	case "composition":
		return to != "tools" && to != "tests" && to != "cmd"
	case "cmd":
		return to == "composition"
	}
	return strings.HasPrefix(from, "adapter:") && (to == "ports" || to == "domain")
}

func coreStandard(layer, imp string) bool {
	pure := " bytes cmp errors math math/bits regexp slices sort strconv strings unicode unicode/utf8 "
	if strings.Contains(pure, " "+imp+" ") {
		return true
	}
	return (layer == "ports" || layer == "application") && (imp == "context" || imp == "io")
}

func architecture(root string) error {
	module, err := moduleName(root)
	if err != nil {
		return err
	}
	var files []source
	dirs := map[string]bool{}
	err = walkSources(root, func(rel string, entry fs.DirEntry) error {
		if entry.IsDir() {
			if entry.Name() == "vendor" {
				return fmt.Errorf("vendor directory is not allowed: %s", rel)
			}
			return nil
		}
		if (entry.Name() == "go.mod" && rel != "go.mod") || entry.Name() == "go.work" {
			return fmt.Errorf("nested module or workspace is not allowed: %s", rel)
		}
		if !strings.HasSuffix(rel, ".go") {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink Go source is not allowed: %s", rel)
		}
		dir := filepath.ToSlash(filepath.Dir(rel))
		layer := layerOf(dir)
		if layer == "" {
			return fmt.Errorf("unclassified Go source: %s", rel)
		}
		if layer == "tests" && !strings.HasSuffix(rel, "_test.go") {
			return fmt.Errorf("integration directory contains non-test source: %s", rel)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, rel), nil, parser.ParseComments)
		if err != nil {
			return err
		}
		for _, group := range parsed.Comments {
			for _, comment := range group.List {
				if strings.HasPrefix(comment.Text, "//go:linkname") || strings.HasPrefix(comment.Text, "//go:generate") {
					return fmt.Errorf("prohibited directive in %s", rel)
				}
			}
		}
		file := source{path: rel, dir: dir, layer: layer, externalTest: strings.HasSuffix(parsed.Name.Name, "_test")}
		for _, declaration := range parsed.Imports {
			importPath, err := strconv.Unquote(declaration.Path.Value)
			if err != nil {
				return err
			}
			file.imports = append(file.imports, importPath)
		}
		files = append(files, file)
		if !file.externalTest {
			dirs[dir] = true
		}
		return nil
	})
	if err != nil {
		return err
	}
	graph := map[string][]string{}
	for _, file := range files {
		for _, importPath := range file.imports {
			if importPath == "C" || importPath == "unsafe" || importPath == "plugin" {
				return fmt.Errorf("prohibited import %s in %s", importPath, file.path)
			}
			if strings.HasPrefix(importPath, module+"/") || importPath == module {
				target := strings.TrimPrefix(importPath, module+"/")
				if importPath == module {
					target = "."
				}
				if !dirs[target] {
					return fmt.Errorf("unresolved local import %s in %s", importPath, file.path)
				}
				if !permits(file.layer, layerOf(target)) {
					return fmt.Errorf("forbidden %s dependency on %s in %s", file.layer, layerOf(target), file.path)
				}
				if !file.externalTest {
					graph[file.dir] = append(graph[file.dir], target)
				}
				continue
			}
			importedPackage, err := build.Default.Import(importPath, "", build.FindOnly)
			if err != nil || !importedPackage.Goroot {
				return fmt.Errorf("nonstandard or unresolved import %s in %s", importPath, file.path)
			}
			core := file.layer == "domain" || file.layer == "ports" || file.layer == "application"
			if core && !coreStandard(file.layer, importPath) && !(strings.HasSuffix(file.path, "_test.go") && importPath == "testing") {
				return fmt.Errorf("forbidden core builtin %s in %s", importPath, file.path)
			}
		}
	}
	const (
		visiting = iota + 1
		visited
	)
	state := map[string]int{}
	var visit func(string) error
	visit = func(directory string) error {
		if state[directory] == visiting {
			return fmt.Errorf("package import cycle at %s", directory)
		}
		if state[directory] == visited {
			return nil
		}
		state[directory] = visiting
		for _, dependency := range graph[directory] {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[directory] = visited
		return nil
	}
	directories := make([]string, 0, len(graph))
	for directory := range graph {
		directories = append(directories, directory)
	}
	sort.Strings(directories)
	for _, directory := range directories {
		if err := visit(directory); err != nil {
			return err
		}
	}
	return nil
}
