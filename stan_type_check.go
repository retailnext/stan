// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package stan

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/types"
)

func typeCheck(pkg *parsedPackage, importer types.ImporterFrom) *Package {
	if importer == nil {
		importer = imp
	}

	var hardError error

	config := &types.Config{
		Importer: importer,
		Error: func(err error) {
			te := err.(types.Error)

			if te.Soft {
				// soft errors are ignorable
				return
			}

			fileName := pkg.fset.Position(te.Pos).Filename
			for _, nb := range pkg.nonBuildFiles {
				if fileName == pkg.fset.Position(nb.Pos()).Filename {
					// we know non-buildable files can have OS specific stuff we can't handle
					return
				}
			}

			hardError = err
		},
		Sizes: types.SizesFor("gc", build.Default.GOARCH),
	}
	info := types.Info{
		Types:     make(map[ast.Expr]types.TypeAndValue),
		Defs:      make(map[*ast.Ident]types.Object),
		Uses:      make(map[*ast.Ident]types.Object),
		Implicits: make(map[ast.Node]types.Object),
		Scopes:    make(map[ast.Node]*types.Scope),
	}

	dedupeObjects(pkg.buildFiles, pkg.nonBuildFiles)

	// check buildable files first so in the case of duplicate
	// objects, the buildable file keeps the original name
	allFiles := append(pkg.buildFiles, pkg.nonBuildFiles...)

	tPkg, _ := config.Check(pkg.path, pkg.fset, allFiles, &info)

	if hardError != nil {
		panic(fmt.Sprintf("type checker error: %s", hardError))
	}

	lifetimes := make(map[types.Object]ObjectLifetime)

	updateLifetimes := func(objs map[*ast.Ident]types.Object, isUse bool) {
		for ident, obj := range objs {
			if obj == nil {
				continue
			}
			sp, ok := lifetimes[obj]
			if !ok || ident.Pos() < sp.First {
				sp.First = ident.Pos()
			}
			if !ok || ident.End() > sp.Last {
				sp.Last = ident.End()
			}
			if isUse {
				sp.Uses = append(sp.Uses, ident)
			} else {
				sp.Def = ident
			}
			lifetimes[obj] = sp
		}
	}

	updateLifetimes(info.Defs, false)
	updateLifetimes(info.Uses, true)

	return &Package{
		Node:         pkg.pkg,
		Fset:         pkg.fset,
		TypesInfo:    &info,
		TypesPkg:     tPkg,
		lifetimes:    lifetimes,
		typesCache:   make(map[string]types.Type),
		objectsCache: make(map[string]types.Object),
	}
}

type nameWithRecv struct {
	recv string
	name string
}

// find any duplicate objects and rename them in the non buildable files
// so they are at least present after type checking
func dedupeObjects(buildable, nonBuildable []*ast.File) {

	// we need to check all file level declarations (variables, constants, functions
	// and methods)
	iterNames := func(f *ast.File, cb func(nameId *ast.Ident, name nameWithRecv)) {
		for _, d := range f.Decls {
			switch v := d.(type) {
			case *ast.FuncDecl:
				var recv string
				if v.Recv != nil {
					switch rv := v.Recv.List[0].Type.(type) {
					case *ast.Ident:
						recv = rv.Name
					case *ast.StarExpr:
						id, _ := rv.X.(*ast.Ident)
						if id != nil {
							recv = id.Name
						}
					}
					if recv == "" {
						// weren't able to figure out receiver name
						continue
					}
				}
				cb(v.Name, nameWithRecv{name: v.Name.Name, recv: recv})
			case *ast.GenDecl:
				for _, spec := range v.Specs {
					switch v := spec.(type) {
					case *ast.ValueSpec:
						for _, id := range v.Names {
							cb(id, nameWithRecv{name: id.Name})
						}
					case *ast.TypeSpec:
						cb(v.Name, nameWithRecv{name: v.Name.Name})
					}
				}
			}
		}
	}

	usedNames := make(map[nameWithRecv]int)

	// seed top level names from buildable files
	for _, f := range buildable {
		iterNames(f, func(nameId *ast.Ident, name nameWithRecv) {
			usedNames[name]++
		})
	}

	for _, f := range nonBuildable {
		iterNames(f, func(nameId *ast.Ident, name nameWithRecv) {
			if c := usedNames[name]; c > 0 {
				// if someone has already used this name then make
				// a new one
				newName := fmt.Sprintf("%s_nobuild%d", name.name, c)

				// in case new name happens to already be in use
				for usedNames[nameWithRecv{name: newName, recv: name.recv}] > 0 {
					usedNames[name]++
					newName = fmt.Sprintf("%s_nobuild%d", name.name, usedNames[name])
				}

				usedNames[name]++

				nameId.Name = newName
			}
		})
	}
}
