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

// Package contains combines the *ast.Package and *types.Package into a single
// object.
type Package struct {
	Node      *ast.Package
	Fset      *token.FileSet
	TypesInfo *types.Info
	TypesPkg  *types.Package

	lifetimes    map[types.Object]ObjectLifetime
	typesCache   map[string]types.Type
	objectsCache map[string]types.Object
}

type Poser interface {
	Pos() token.Pos
}

// Pos() returns the position of an ast.Node or types.Object in the file set.
// Convenient for error reporting, token.Position.String() yields
// file:line:column.
func (p *Package) Pos(n Poser) token.Position {
	if n == nil {
		panic("nil passed to Pos()")
	}
	return p.Fset.Position(n.Pos())
}

// Path() returns the p's unique import path (including vendor/).
func (p *Package) Path() string {
	return p.TypesPkg.Path()
}

// Files() returns a map of file name to *ast.File for the files that make up p.
func (p *Package) Files() map[string]*ast.File {
	return p.Node.Files
}

// ObjectOf() returns the corresponding types.Object of an ast.Node. n normally
// should be an *ast.Ident, but ObjectOf will also extract the ident from an
// *ast.SelectorExpr. The return value can be nil if there is no corresponding
// object.
func (p *Package) ObjectOf(n ast.Node) types.Object {
	switch v := n.(type) {
	case *ast.Ident:
		return p.TypesInfo.ObjectOf(v)
	case *ast.SelectorExpr:
		return p.TypesInfo.ObjectOf(v.Sel)
	default:
		return nil
	}
}

// TypeOf() returns the types.Type of a given expression. Can return
// nil if expresion not found.
func (p *Package) TypeOf(e ast.Expr) types.Type {
	return p.TypesInfo.TypeOf(e)
}

// IterateObjects() iterates over all types.Objects in p.
func (p *Package) IterateObjects(f func(types.Object)) {
	for obj := range p.lifetimes {
		f(obj)
	}
	for _, obj := range p.TypesInfo.Implicits {
		f(obj)
	}
}

// String() returns the import path of p.
func (p *Package) String() string {
	return p.Path()
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

// Pkgs() finds, parses and type checks the packages specified by pkgPaths.
// Wildcard "..." expressions may be used, similar to various "go" commands.
// Pkgs() panics if there is a parse error, "hard" type check error, or if no
// such package could be found.
//
// In order to maximize test coverage, Pkgs() does a few potentially unexpected
// things to parse/check as much code as possible:
// 	 - includes *_test.go files in packages
// 	 - includes "XTest" _test packages as separate packages
// 	 - attempts to invoke cgo preprocessor on cgo files so type info is
//     available
// 	 - loads all *.go files, even if non-buildable due to build constraints
//     (stan will rename duplicate objects to prevent type checking errors,
//     and ignore "hard" type check error for non-buildable files)
//
func Pkgs(pkgPaths ...string) []*Package {
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
				checked := typeCheck(n.pkg, nil)
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
				checked := typeCheck(n.pkg, nil)
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

// ObjectLifetime represents the "lifetime" of an object.
type ObjectLifetime struct {
	// Lexical first and last use of object
	First, Last token.Pos
	// Definition of object
	Def *ast.Ident
	// Uses of object
	Uses []*ast.Ident
}

// LifetimeOf() returns an object representing the lifetime of
// types.Object obj within p. If obj is not used by p, LifetimeOf
// returns the zero value ObjectLifetime.
func (p *Package) LifetimeOf(obj types.Object) ObjectLifetime {
	return p.lifetimes[obj]
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

// Invocation represents the invocation of a *types.Func.
type Invocation struct {
	// Invocant object, if available.
	Invocant types.Object
	// Args to function invocation
	Args []ast.Expr
	// Invocation's *ast.CallExpr node
	Call *ast.CallExpr
}

// InvocationsOf() returns the invocations of obj within p. InvocationsOf
// panics if obj is not a *types.Func.
func (p *Package) InvocationsOf(obj types.Object) []Invocation {
	fn, _ := obj.(*types.Func)
	if fn == nil {
		panic(fmt.Sprintf("object %[1]s is not *types.Func (%[1]T)", obj))
	}

	var ret []Invocation
	for _, use := range p.LifetimeOf(obj).Uses {
		ancs := p.AncestorsOf(use)

		var invocant types.Object
		if sel, _ := ancs.Peek().(*ast.SelectorExpr); sel != nil {
			switch x := sel.X.(type) {
			case *ast.SelectorExpr:
				invocant = p.ObjectOf(x.Sel)
			case *ast.Ident:
				invocant = p.ObjectOf(x)
			}

			ancs.Pop()
		}

		call, _ := ancs.Peek().(*ast.CallExpr)
		if call == nil {
			continue
		}

		ret = append(ret, Invocation{
			Invocant: invocant,
			Args:     call.Args,
			Call:     call,
		})
	}

	return ret
}
