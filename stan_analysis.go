// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package stan

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strings"
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
	for _, obj := range p.TypesInfo.Implicits {
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

type importNode struct {
	pkg        *parsedPackage
	imports    map[string]*importNode
	importedBy map[string]*importNode
}

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

	var (
		parsedPkgs = findAndParse(needLookup)
		nodes      = make(map[string]*importNode)
	)
	for _, pkgs := range parsedPkgs {
		for _, pkg := range pkgs {
			if _, found := packagesCache[pkg.path]; found {
				continue
			}
			nodes[pkg.path] = &importNode{
				pkg:        pkg,
				imports:    make(map[string]*importNode),
				importedBy: make(map[string]*importNode),
			}
		}
	}

	// create directed acyclic graph of package imports
	for _, n := range nodes {
		for _, f := range n.pkg.pkg.Files {
			for _, imp := range f.Imports {
				// not right for vendor case, but does not cause incorrect
				// behavior (does cause potential additional importer call)
				path := strings.Trim(imp.Path.Value, `"`)
				if other := nodes[path]; other != nil {
					other.importedBy[n.pkg.path] = n
					n.imports[path] = other
				}
			}
		}
	}

	// walk graph from leaf nodes so we know we have not encountered any importers
	// of the current node yet
	for len(nodes) > 0 {
		startSize := len(nodes)
		for id, n := range nodes {
			if len(n.imports) == 0 {
				checked := typeCheck(n.pkg)
				packagesCache[n.pkg.path] = []*Package{checked}
				// stick our type checked *types.Package into the importer map to
				// avoid extra work importing this package from other packages
				if imports[n.pkg.path] == nil {
					imports[n.pkg.path] = checked.TypesPkg
				}
				for _, importsMe := range n.importedBy {
					delete(importsMe.imports, n.pkg.path)
				}
				delete(nodes, id)
			}
		}

		if len(nodes) == startSize {
			// We probably have an import loop, but it is possible it isn't a loop due
			// to vendoring. Type check remaining packages in un-optimized order.
			for _, n := range nodes {
				checked := typeCheck(n.pkg)
				packagesCache[n.pkg.path] = []*Package{checked}
			}

			break
		}
	}

	for i, pkgs := range parsedPkgs {
		var loaded []*Package
		for _, pkg := range pkgs {
			loaded = append(loaded, packagesCache[pkg.path]...)
		}
		// sort packages so ordering is deterministic
		sort.Slice(loaded, func(i, j int) bool {
			return loaded[i].Path() < loaded[j].Path()
		})
		packagesCache[needLookup[i]] = loaded
		ret = append(ret, loaded...)
	}

	return ret
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

// Walk the AST starting rooted at n yielding the slice of ancestors for
// each node visited. The ancestors slice is mutated during traversal so
// be sure to copy() the ancestors if you want to save them off.
func WalkAST(n ast.Node, fn func(node ast.Node, ancs Ancestors)) {
	if n == nil {
		panic(fmt.Sprintf("nil ast.Node passed to WalkAST"))
	}

	walker := &astWalker{
		fn: fn,
	}
	ast.Walk(walker, n)
}

// Look up ancestor nodes of given node. AncestorsOf panics if the target node
// is not found in p's AST. If you need the ancestors of many nodes you may be
// better off walking the AST once yourself.
func (p *Package) AncestorsOf(target ast.Node) Ancestors {
	var ret Ancestors
	WalkAST(p.Node, func(node ast.Node, ancs Ancestors) {
		if node == target {
			ret = make(Ancestors, len(ancs))
			copy(ret, ancs)
		}
	})

	if ret == nil {
		panic("node not found")
	}

	return ret
}
