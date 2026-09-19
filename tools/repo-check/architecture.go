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
	b, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == "module" {
			return f[1], nil
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
	p := strings.Split(dir, "/")
	if len(p) >= 2 && p[0] == "internal" {
		switch p[1] {
		case "domain", "ports", "application", "cli", "composition":
			return p[1]
		case "adapters":
			if len(p) >= 3 {
				return "adapter:" + p[2]
			}
		}
	}
	if len(p) >= 2 && p[0] == "cmd" {
		return "cmd"
	}
	if p[0] == "tools" {
		return "tools"
	}
	if p[0] == "tests" {
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
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, rel), nil, parser.ParseComments)
		if err != nil {
			return err
		}
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if strings.HasPrefix(c.Text, "//go:linkname") || strings.HasPrefix(c.Text, "//go:generate") {
					return fmt.Errorf("prohibited directive in %s", rel)
				}
			}
		}
		s := source{path: rel, dir: dir, layer: layer, externalTest: strings.HasSuffix(f.Name.Name, "_test")}
		for _, i := range f.Imports {
			value, err := strconv.Unquote(i.Path.Value)
			if err != nil {
				return err
			}
			s.imports = append(s.imports, value)
		}
		files = append(files, s)
		if !s.externalTest {
			dirs[dir] = true
		}
		return nil
	})
	if err != nil {
		return err
	}
	graph := map[string][]string{}
	for _, s := range files {
		for _, imp := range s.imports {
			if imp == "C" || imp == "unsafe" || imp == "plugin" {
				return fmt.Errorf("prohibited import %s in %s", imp, s.path)
			}
			if strings.HasPrefix(imp, module+"/") || imp == module {
				target := strings.TrimPrefix(imp, module+"/")
				if imp == module {
					target = "."
				}
				if !dirs[target] {
					return fmt.Errorf("unresolved local import %s in %s", imp, s.path)
				}
				if !permits(s.layer, layerOf(target)) {
					return fmt.Errorf("forbidden %s dependency on %s in %s", s.layer, layerOf(target), s.path)
				}
				if !s.externalTest {
					graph[s.dir] = append(graph[s.dir], target)
				}
				continue
			}
			p, err := build.Default.Import(imp, "", build.FindOnly)
			if err != nil || !p.Goroot {
				return fmt.Errorf("nonstandard or unresolved import %s in %s", imp, s.path)
			}
			core := s.layer == "domain" || s.layer == "ports" || s.layer == "application"
			if core && !coreStandard(s.layer, imp) && !(strings.HasSuffix(s.path, "_test.go") && imp == "testing") {
				return fmt.Errorf("forbidden core builtin %s in %s", imp, s.path)
			}
		}
	}
	state := map[string]int{}
	var visit func(string) error
	visit = func(n string) error {
		if state[n] == 1 {
			return fmt.Errorf("package import cycle at %s", n)
		}
		if state[n] == 2 {
			return nil
		}
		state[n] = 1
		for _, next := range graph[n] {
			if err := visit(next); err != nil {
				return err
			}
		}
		state[n] = 2
		return nil
	}
	keys := make([]string, 0, len(graph))
	for k := range graph {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := visit(k); err != nil {
			return err
		}
	}
	return nil
}
