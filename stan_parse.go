// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package stan

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

type parsedPackage struct {
	pkg           *ast.Package
	buildFiles    []*ast.File
	nonBuildFiles []*ast.File

	path string
	fset *token.FileSet
}

func findAndParse(paths []string) [][]*parsedPackage {
	var (
		wildcard []string
		ret      [][]*parsedPackage
	)

	for _, p := range paths {
		if strings.Contains(p, "...") {
			if build.IsLocalImport(p) {
				ret = append(ret, findAndParseWildcardLocal(p))
			} else {
				wildcard = append(wildcard, p)
			}
		} else {
			ret = append(ret, []*parsedPackage{findAndParseSingle(p)})
		}
	}

	return append(ret, findAndParseWildcard(wildcard)...)
}

type matcher = func(string) bool

func any(matchers []matcher, input string) bool {
	for _, m := range matchers {
		if m(input) {
			return true
		}
	}
	return false
}

// based on cmd/go/internal/load.MatchPackages
func findAndParseWildcard(paths []string) [][]*parsedPackage {
	if len(paths) == 0 {
		return nil
	}

	match := make([]matcher, len(paths))
	treeCanMatch := make([]matcher, len(paths))
	for i, pattern := range paths {
		match[i] = matchPattern(pattern)
		treeCanMatch[i] = treeCanMatchPattern(pattern)
	}

	ret := make([][]*parsedPackage, len(paths))

	have := make(map[string]bool)

	for _, src := range build.Default.SrcDirs() {
		src = filepath.Clean(src) + string(filepath.Separator)

		filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
			if err != nil || path == src {
				return nil
			}

			want := true

			_, elem := filepath.Split(path)
			if strings.HasPrefix(elem, ".") || strings.HasPrefix(elem, "_") || elem == "testdata" {
				want = false
			}

			name := filepath.ToSlash(path[len(src):])

			if !any(treeCanMatch, name) {
				want = false
			}

			if !fi.IsDir() {
				return nil
			}

			if !want {
				return filepath.SkipDir
			}

			if have[name] {
				return nil
			}
			have[name] = true

			if !any(match, name) {
				return nil
			}

			code, xtest, err := parseDir(path)
			if err != nil {
				panic(fmt.Sprintf("error parsing %s: %s", path, err))
			}

			if code != nil {
				code.path = name
				for i, m := range match {
					if m(name) {
						ret[i] = append(ret[i], code)
					}
				}
			}
			if xtest != nil {
				xtest.path = name + ":xtest"
				for i, m := range match {
					if m(name) {
						ret[i] = append(ret[i], xtest)
					}
				}
			}

			return nil
		})
	}

	return ret
}

// based on cmd/go/internal/load.MatchPackagesInFS
func findAndParseWildcardLocal(pattern string) []*parsedPackage {

	i := strings.Index(pattern, "...")
	dir, _ := path.Split(pattern[:i])

	prefix := ""
	if strings.HasPrefix(pattern, "./") {
		prefix = "./"
	}
	match := matchPattern(pattern)

	var pkgs []*parsedPackage
	filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || !fi.IsDir() {
			return nil
		}
		if path == dir {
			path = filepath.Clean(path)
		}

		_, elem := filepath.Split(path)
		dot := strings.HasPrefix(elem, ".") && elem != "." && elem != ".."
		if dot || strings.HasPrefix(elem, "_") || elem == "testdata" {
			return filepath.SkipDir
		}

		name := prefix + filepath.ToSlash(path)
		if !match(name) {
			return nil
		}

		code, xtest, err := parseDir(path)
		if err != nil {
			panic(fmt.Sprintf("error parsing %s: %s", path, err))
		}

		if code != nil {
			code.path = name
			pkgs = append(pkgs, code)
		}
		if xtest != nil {
			xtest.path = name + ":xtest"
			pkgs = append(pkgs, xtest)
		}

		return nil
	})
	return pkgs
}

func findAndParseSingle(importPath string) *parsedPackage {
	path := filepath.FromSlash(strings.TrimSuffix(importPath, ":xtest"))

	var dir string

	if build.IsLocalImport(path) {
		dir = path
	} else {
		for _, src := range build.Default.SrcDirs() {
			maybeDir := filepath.Join(src, path)
			if _, err := os.Stat(maybeDir); err != nil {
				continue
			}
			dir = maybeDir
			break
		}
	}

	if dir == "" {
		panic(fmt.Sprintf("could not find package %s", importPath))
	}

	codePkg, testxPkg, err := parseDir(dir)
	if err != nil {
		panic(fmt.Sprintf("error parsing %s: %s", dir, err))
	}

	if strings.HasSuffix(importPath, ":xtest") {
		// they asked for _test package
		if testxPkg == nil {
			panic(fmt.Sprintf("could not find package %s", importPath))
		}
		testxPkg.path = importPath
		return testxPkg
	} else {
		if codePkg == nil {
			panic(fmt.Sprintf("could not find package %s", importPath))
		}
		codePkg.path = importPath
		return codePkg
	}
}

func parseDir(dir string) (*parsedPackage, *parsedPackage, error) {
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}

	var goFileNames []string

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}

		goFileNames = append(goFileNames, filepath.Join(dir, entry.Name()))
	}

	astFiles := make([]*ast.File, len(goFileNames))
	errors := make([]error, len(goFileNames))
	goFileContents := make([][]byte, len(goFileNames))

	fset := token.NewFileSet()

	var wg sync.WaitGroup
	wg.Add(len(goFileNames))

	for idx := range goFileNames {
		go func(idx int) {
			defer wg.Done()

			// slurp contents once so we don't have to open twice when parsing
			// and checking build match
			contents, err := ioutil.ReadFile(goFileNames[idx])
			if err != nil {
				errors[idx] = err
				return
			}

			goFileContents[idx] = contents

			astFiles[idx], errors[idx] = parser.ParseFile(fset, goFileNames[idx], contents, 0)
		}(idx)
	}

	wg.Wait()

	for _, err := range errors {
		if err != nil {
			return nil, nil, err
		}
	}

	ctx := build.Default

	pkgs := make(map[string]*parsedPackage)

	for i, f := range astFiles {
		pkg := pkgs[f.Name.Name]
		if pkg == nil {
			pkg = &parsedPackage{
				pkg: &ast.Package{
					Name:  f.Name.Name,
					Files: make(map[string]*ast.File),
				},
				fset: fset,
			}
			pkgs[f.Name.Name] = pkg
		}

		ctx.OpenFile = func(p string) (io.ReadCloser, error) {
			if p != goFileNames[i] {
				panic(fmt.Sprintf("context asked for unexpected file: %s != %s", p, goFileNames[i]))
			}
			return ioutil.NopCloser(bytes.NewReader(goFileContents[i])), nil
		}

		match, err := ctx.MatchFile(dir, filepath.Base(goFileNames[i]))
		if err != nil {
			return nil, nil, err
		}
		if match {
			pkg.buildFiles = append(pkg.buildFiles, f)
		} else {
			pkg.nonBuildFiles = append(pkg.nonBuildFiles, f)
		}

		pkg.pkg.Files[goFileNames[i]] = f
	}

	var codePkg, xtestPkg *parsedPackage
	for _, pkg := range pkgs {
		if strings.HasSuffix(pkg.pkg.Name, "_test") {
			if xtestPkg != nil {
				return nil, nil, fmt.Errorf("more than one _test package in %s", dir)
			}
			xtestPkg = pkg
		} else {
			if codePkg != nil {
				return nil, nil, fmt.Errorf("more than one package declared in %s (%s and %s)", dir, codePkg.pkg.Name, pkg.pkg.Name)
			}
			codePkg = pkg
		}
	}

	return codePkg, xtestPkg, nil
}
