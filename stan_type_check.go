// Copyright (c) 2018, RetailNext, Inc.
// This material contains trade secrets and confidential information of
// RetailNext, Inc.  Any use, reproduction, disclosure or dissemination
// is strictly prohibited without the explicit written permission
// of RetailNext, Inc.
// All rights reserved.

package stan

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/types"
)

func typeCheck(pkg *parsedPackage) *Package {
	config := &types.Config{
		Importer: imp,
		Error:    func(err error) {},
		Sizes:    types.SizesFor("gc", build.Default.GOARCH),
	}
	info := types.Info{
		Types:     make(map[ast.Expr]types.TypeAndValue),
		Defs:      make(map[*ast.Ident]types.Object),
		Uses:      make(map[*ast.Ident]types.Object),
		Implicits: make(map[ast.Node]types.Object),
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
