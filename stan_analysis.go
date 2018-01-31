// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package stan

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/token"
	"go/types"
	"sort"
	"sync"
)

type Package struct {
	Node      *ast.Package
	Fset      *token.FileSet
	TypesInfo *types.Info
	TypesPkg  *types.Package

	spans        map[types.Object]Span
	typesCache   map[string]types.Type
	objectsCache map[string]types.Object
}

type Poser interface {
	Pos() token.Pos
}

func (p *Package) Pos(n Poser) token.Position {
	if n == nil {
		panic("nil passed to Pos()")
	}
	return p.Fset.Position(n.Pos())
}

func (p *Package) Path() string {
	return p.TypesPkg.Path()
}

func (p *Package) Files() map[string]*ast.File {
	return p.Node.Files
}

func (p *Package) ObjectOf(id *ast.Ident) types.Object {
	return p.TypesInfo.ObjectOf(id)
}

func (p *Package) TypeOf(e ast.Expr) types.Type {
	return p.TypesInfo.TypeOf(e)
}

func (p *Package) SearchObjects(f func(types.Object) bool) []types.Object {
	var ret []types.Object
	for obj := range p.spans {
		if f == nil || f(obj) {
			ret = append(ret, obj)
		}
	}
	return ret
}

func (p *Package) String() string {
	return p.Path()
}

type Packages []*Package

func (ps Packages) Walk(fn func(*Package, ast.Node, Ancestors)) {
	for _, p := range ps {
		WalkAST(p.Node, func(n ast.Node, ancs Ancestors) {
			fn(p, n, ancs)
		})
	}
}

func (ps Packages) IterateFiles(fn func(fileName string, file *ast.File)) {
	for _, p := range ps {
		for name, f := range p.Node.Files {
			fn(name, f)
		}
	}
}

var (
	packagesCache  = make(map[string][]*Package)
	loadPackagesMu sync.Mutex
)

// Parse all go packages in pkgPaths (not recursive).
func Pkgs(pkgPaths ...string) Packages {
	// keep it simple
	loadPackagesMu.Lock()
	defer loadPackagesMu.Unlock()

	var (
		ret        []*Package
		needLookup []string
	)

	for _, path := range pkgPaths {
		if cached, found := packagesCache[path]; found {
			ret = append(ret, cached...)
		} else {
			needLookup = append(needLookup, path)
		}
	}

	if len(needLookup) == 0 {
		return ret
	}

	for i, pkgs := range findAndParse(needLookup) {
		var (
			wg             sync.WaitGroup
			mu             sync.Mutex
			loadedPackages []*Package
		)

		for _, pkg := range pkgs {
			pkg := pkg

			if cached, found := packagesCache[pkg.path]; found {
				mu.Lock()
				loadedPackages = append(loadedPackages, cached...)
				mu.Unlock()
				continue
			}

			wg.Add(1)
			go func() {
				defer wg.Done()

				staticPkg := typeCheck(pkg)

				mu.Lock()
				loadedPackages = append(loadedPackages, staticPkg)
				mu.Unlock()
			}()
		}

		wg.Wait()

		if len(loadedPackages) == 0 {
			panic(fmt.Sprintf("no packages found for %s", needLookup[i]))
		}

		// sort packages so things are consistent in tests
		sort.Slice(loadedPackages, func(i, j int) bool {
			return loadedPackages[i].Path() < loadedPackages[j].Path()
		})

		packagesCache[needLookup[i]] = loadedPackages

		// make sure each package is cached individually as well
		for _, pkg := range loadedPackages {
			packagesCache[pkg.Path()] = []*Package{pkg}
		}
		ret = append(ret, loadedPackages...)
	}

	return ret
}

func typeCheck(pkg *parsedPackage) *Package {
	config := &types.Config{
		Importer:    imp,
		Error:       func(error) {},
		Sizes:       types.SizesFor("gc", build.Default.GOARCH),
		FakeImportC: true,
	}
	info := types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}

	dedupeObjects(pkg.buildFiles, pkg.nonBuildFiles)

	// check buildable files first so in the case of duplicate
	// objects, the buildable file keeps the original name
	allFiles := append(pkg.buildFiles, pkg.nonBuildFiles...)

	// type check as much as you can and ignore errors (there will
	// be "expected" errors from cgo, build constraints, etc)
	tPkg, _ := config.Check(pkg.path, pkg.fset, allFiles, &info)

	spans := make(map[types.Object]Span)

	updateSpans := func(objs map[*ast.Ident]types.Object, isUse bool) {
		for ident, obj := range objs {
			if obj == nil {
				continue
			}
			sp, ok := spans[obj]
			if !ok || ident.Pos() < sp.First {
				sp.First = ident.Pos()
			}
			if !ok || ident.End() > sp.Last {
				sp.Last = ident.End()
			}
			if isUse {
				sp.Uses = append(sp.Uses, ident)
			}
			spans[obj] = sp
		}
	}

	updateSpans(info.Defs, false)
	updateSpans(info.Uses, true)

	return &Package{
		Node:         pkg.pkg,
		Fset:         pkg.fset,
		TypesInfo:    &info,
		TypesPkg:     tPkg,
		spans:        spans,
		typesCache:   make(map[string]types.Type),
		objectsCache: make(map[string]types.Object),
	}
}

type Span struct {
	First, Last token.Pos
	Uses        []*ast.Ident
}

func (p *Package) SpanOf(o types.Object) Span {
	return p.spans[o]
}

type Ancestors []ast.Node

type astWalker struct {
	ancestors Ancestors
	fn        func(node ast.Node, ancs Ancestors)
}

func (w *astWalker) Visit(node ast.Node) ast.Visitor {
	// finished walking children, remove self from ancestors
	if node == nil {
		w.ancestors = w.ancestors[:len(w.ancestors)-1]
		return nil
	}

	w.fn(node, w.ancestors)

	// add self to ancestors list for walking children
	w.ancestors = append(w.ancestors, node)

	return w
}

func WalkAST(n ast.Node, fn func(node ast.Node, ancs Ancestors)) {
	if n == nil {
		panic(fmt.Sprintf("nil ast.Node passed to WalkAST"))
	}

	walker := &astWalker{
		fn: fn,
	}
	ast.Walk(walker, n)
}

// find any duplicate objects and rename them in the non buildable files
// so they are at least present after type checking
func dedupeObjects(buildable, nonBuildable []*ast.File) {
	topLevelNames := make(map[string]int)

	// seed top level names from buildale files
	for _, f := range buildable {
		for n, _ := range f.Scope.Objects {
			topLevelNames[n]++
		}
	}

	for _, f := range nonBuildable {
		for bn, o := range f.Scope.Objects {
			if c := topLevelNames[bn]; c > 0 {
				// if someone has already used this name then make
				// a new one
				newName := fmt.Sprintf("%s_nobuild%d", bn, c)

				// in case new name happens to already in use
				for topLevelNames[newName] > 0 {
					topLevelNames[bn]++
					newName = fmt.Sprintf("%s_nobuild%d", bn, topLevelNames[bn])
				}

				o.Name = newName
				delete(f.Scope.Objects, bn)
				f.Scope.Objects[newName] = o

				switch d := o.Decl.(type) {
				case *ast.FuncDecl:
					d.Name.Name = newName
				case *ast.ValueSpec:
					for _, vn := range d.Names {
						if vn.Name == bn {
							vn.Name = newName
						}
					}
				}
			}

			topLevelNames[bn]++
		}
	}
}
