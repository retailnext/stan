// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package stan

import (
	"fmt"
	"go/token"
	"go/types"
	"strings"
)

// Look up a types.Type based on the name of a type, or an unnamed type expression.
//
//   LookupType("encoding/json.Marshaler") // named types are <import path>.<name>
//   LookupType("*encoding/json.Encoder")  // prepend "*" to get pointer type
//   LookupType("[5]int")                  // for builtin types, use arbitary expression
//
// If an error occurs or the type cannot be found, LookupType() panics.
func (p *Package) LookupType(typeSpec string) types.Type {
	if cached := p.typesCache[typeSpec]; cached != nil {
		return cached
	}

	t, err := p.lookupType(typeSpec)
	if err != nil {
		panic(fmt.Sprintf("error looking up type %s: %s", typeSpec, err))
	}
	if t == nil {
		panic(fmt.Sprintf("no such type %s", typeSpec))
	}

	p.typesCache[typeSpec] = t

	return t
}

func (p *Package) lookupType(typeSpec string) (types.Type, error) {
	finalSlash := strings.LastIndexByte(typeSpec, '/')
	firstDotAfterLastSlash := strings.IndexByte(typeSpec[finalSlash+1:], '.')

	if firstDotAfterLastSlash == -1 {
		// assume unnamed type expression
		tv, err := types.Eval(token.NewFileSet(), nil, 0, typeSpec)
		if err != nil {
			return nil, fmt.Errorf("error evaluating type expression %q: %s", typeSpec, err)
		}
		return tv.Type, nil
	}

	dotIdx := finalSlash + 1 + firstDotAfterLastSlash

	importPath := strings.TrimLeft(typeSpec[:dotIdx], "*")
	typ := strings.TrimLeft(typeSpec[dotIdx+1:], "*")

	var tPkg *types.Package
	if importPath == p.Path() {
		tPkg = p.TypesPkg
	} else {
		var err error
		tPkg, err = imp.Import(importPath)
		if err != nil {
			return nil, fmt.Errorf("error importing %s: %s", importPath, err)
		}
	}

	obj := tPkg.Scope().Lookup(typ)
	if obj == nil {
		return nil, nil
	}

	typeName, ok := obj.(*types.TypeName)
	if !ok {
		return nil, fmt.Errorf("%s.%s is not a type name (%T)", tPkg.Path(), typ, obj)
	}

	t := typeName.Type()
	for numPtrs := len(typeSpec[:dotIdx]) - len(importPath) + len(typeSpec[dotIdx+1:]) - len(typ); numPtrs > 0; numPtrs-- {
		t = types.NewPointer(t)
	}
	return t, nil
}

// Look up a types.Object based on name.
//
//   LookupObject("io.EOF")         // yields *types.Var
//   LookupObject("io.Copy")        // yields *types.Func
//   LookupObject("io.Reader")      // yields *types.TypeName
//   LookupObject("io.Reader.Read") // yields *types.Func
//   LookupObject("io.pipe.data")   // yields *types.Var
//
// If an error occurs or the object cannot be found, LookupObject() panics.
func (p *Package) LookupObject(objSpec string) types.Object {
	if cached := p.objectsCache[objSpec]; cached != nil {
		return cached
	}

	o, err := p.lookupObject(objSpec)
	if err != nil {
		panic(fmt.Sprintf("error looking up object %s: %s", objSpec, err))
	}
	if o == nil {
		panic(fmt.Sprintf("no such object %s", objSpec))
	}

	p.objectsCache[objSpec] = o

	return o
}

func (p *Package) lookupObject(objSpec string) (types.Object, error) {
	finalSlash := strings.LastIndexByte(objSpec, '/')
	firstDotAfterLastSlash := strings.IndexByte(objSpec[finalSlash+1:], '.')

	if firstDotAfterLastSlash == -1 {
		return nil, fmt.Errorf("invalid object specifier: %s", objSpec)
	}

	dotIdx := finalSlash + 1 + firstDotAfterLastSlash

	importPath := objSpec[:dotIdx]
	parts := strings.Split(objSpec[dotIdx+1:], ".")

	var tPkg *types.Package
	if importPath == p.Path() {
		tPkg = p.TypesPkg
	} else {
		var err error
		tPkg, err = imp.Import(importPath)
		if err != nil {
			return nil, fmt.Errorf("error importing %s: %s", importPath, err)
		}
	}

	obj := tPkg.Scope().Lookup(parts[0])
	if obj == nil {
		for _, imp := range p.TypesInfo.Implicits {
			pi, _ := imp.(*types.PkgName)
			if pi == nil {
				continue
			}

			if pi.Name() == parts[0] {
				obj = imp
				break
			}
		}
	}

	if obj == nil {
		return nil, nil
	}

	for i := 1; i < len(parts); i++ {
		nextObj, _, _ := types.LookupFieldOrMethod(obj.Type(), true, tPkg, parts[i])

		if nextObj == nil {
			return nil, fmt.Errorf("could not find %q on %s", parts[i], obj)
		}

		obj = nextObj
	}

	return obj, nil
}
