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

			parsed, err := parseDir(path, fset)
			if err != nil {
				panic(fmt.Sprintf("error parsing %s: %s", path, err))
			}

			if parsed.code != nil {
				parsed.code.path = name
				for i, m := range match {
					if m(name) {
						ret[i] = append(ret[i], parsed.code)
					}
				}
			}
			if parsed.xtest != nil {
				parsed.xtest.path = name + ":xtest"
				for i, m := range match {
					if m(name) {
						ret[i] = append(ret[i], parsed.xtest)
					}
				}
			}
			for _, nobuild := range parsed.nobuild {
				nobuild.path = name + ":nobuild(" + nobuild.pkg.Name + ")"
				for i, m := range match {
					if m(name) {
						ret[i] = append(ret[i], nobuild)
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

		parsed, err := parseDir(path, fset)
		if err != nil {
			panic(fmt.Sprintf("error parsing %s: %s", path, err))
		}

		if parsed.code != nil {
			parsed.code.path = name
			pkgs = append(pkgs, parsed.code)
		}
		if parsed.xtest != nil {
			parsed.xtest.path = name + ":xtest"
			pkgs = append(pkgs, parsed.xtest)
		}
		for _, nobuild := range parsed.nobuild {
			nobuild.path = name + ":nobuild(" + nobuild.pkg.Name + ")"
			pkgs = append(pkgs, nobuild)
		}

		return nil
	})
	return pkgs
}

func findAndParseSingle(importPath string) *parsedPackage {
	wantXtest := strings.HasSuffix(importPath, ":xtest")
	importPath = strings.TrimSuffix(importPath, ":xtest")

	path := filepath.FromSlash(importPath)

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

	parsed, err := parseDir(dir, fset)
	if err != nil {
		panic(fmt.Sprintf("error parsing %s: %s", dir, err))
	}

	if wantXtest {
		if parsed.xtest != nil {
			parsed.xtest.path = importPath + ":xtest"
			return parsed.xtest
		} else {
			panic(fmt.Sprintf("could not find package %s", importPath))
		}
	} else {
		if parsed.code != nil {
			parsed.code.path = importPath
			return parsed.code
		} else {
			panic(fmt.Sprintf("could not find package %s", importPath))
		}
	}
}

type parsedDir struct {
	code    *parsedPackage
	xtest   *parsedPackage
	nobuild []*parsedPackage
}

func parseDir(dir string, fset *token.FileSet) (*parsedDir, error) {
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
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
			return nil, err
		}
	}

	astFiles, err = cgoIfRequired(nil, fset, astFiles)
	if err != nil {
		return nil, err
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

		fileName := fset.Position(f.Pos()).Filename
		baseName := filepath.Base(fileName)

		ctx.OpenFile = func(p string) (io.ReadCloser, error) {
			if p != goFileNames[i] {
				panic(fmt.Sprintf("context asked for unexpected file: %s != %s", p, fileName))
			}
			return ioutil.NopCloser(bytes.NewReader(goFileContents[i])), nil
		}

		var match bool
		if baseName == "C" {
			match = true
		} else {
			match, err = ctx.MatchFile(dir, baseName)
			if err != nil {
				return nil, err
			}
		}

		if match {
			pkg.buildFiles = append(pkg.buildFiles, f)
		} else {
			pkg.nonBuildFiles = append(pkg.nonBuildFiles, f)
		}

		pkg.pkg.Files[fileName] = f
	}

	var ret parsedDir
	for _, pkg := range pkgs {
		if len(pkg.buildFiles) == 0 {
			ret.nobuild = append(ret.nobuild, pkg)
		} else if strings.HasSuffix(pkg.pkg.Name, "_test") {
			if ret.xtest != nil {
				return nil, fmt.Errorf("more than one _test package in %s", dir)
			}
			ret.xtest = pkg
		} else {
			if ret.code != nil {
				return nil, fmt.Errorf("more than one package declared in %s (%s and %s)", dir, ret.code.pkg.Name, pkg.pkg.Name)
			}
			ret.code = pkg
		}
	}

	return &ret, nil
}

func cgoIfRequired(bp *build.Package, fset *token.FileSet, astFiles []*ast.File) ([]*ast.File, error) {
	var hasCgo bool
Files:
	for _, f := range astFiles {
		for _, imp := range f.Imports {
			if strings.Trim(imp.Path.Value, "`\"") == "C" {
				hasCgo = true
				break Files
			}
		}
	}

	if !hasCgo {
		return astFiles, nil
	}

	if bp == nil {
		var err error
		bp, err = build.ImportDir(filepath.Dir(fset.Position(astFiles[0].Pos()).Filename), 0)
		if err != nil {
			return nil, err
		}
	}

	cgoFiles, err := processCgoFiles(bp, fset, nil, 0)
	if err != nil {
		return nil, err
	}

CgoFiles:
	for _, cf := range cgoFiles {
		for ai, af := range astFiles {
			if fset.Position(af.Pos()).Filename == fset.Position(cf.Pos()).Filename {
				astFiles[ai] = cf
				continue CgoFiles
			}
		}

		// no match, add as additional file
		astFiles = append(astFiles, cf)
	}

	return astFiles, nil
}
