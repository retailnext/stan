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
//   ResolveType("encoding/json.Marshaler") // named types are <import path>.<name>
//   ResolveType("*encoding/json.Encoder")  // prepend "*" to get pointer type
//   ResolveType("[5]int")                  // for builtin types, use arbitary expression
//
// If an error occurs or the type cannot be found, ResolveType() panics.
func (p *Package) ResolveType(typeSpec string) types.Type {
	if cached := p.typesCache[typeSpec]; cached != nil {
		return cached
	}

	t, err := p.resolveType(typeSpec)
	if err != nil {
		panic(fmt.Sprintf("error looking up type %s: %s", typeSpec, err))
	}
	if t == nil {
		panic(fmt.Sprintf("no such type %s", typeSpec))
	}

	p.typesCache[typeSpec] = t

	return t
}

func (p *Package) resolveType(typeSpec string) (types.Type, error) {
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
//   ResolveObject("io.EOF")         // yields *types.Var
//   ResolveObject("io.Copy")        // yields *types.Func
//   ResolveObject("io.Reader")      // yields *types.TypeName
//   ResolveObject("io.Reader.Read") // yields *types.Func
//   ResolveObject("io.pipe.data")   // yields *types.Var
//
// If an error occurs or the object cannot be found, ResolveObject() panics.
func (p *Package) ResolveObject(objSpec string) types.Object {
	if cached := p.objectsCache[objSpec]; cached != nil {
		return cached
	}

	o, err := p.resolveObject(objSpec)
	if err != nil {
		panic(fmt.Sprintf("error looking up object %s: %s", objSpec, err))
	}
	if o == nil {
		panic(fmt.Sprintf("no such object %s", objSpec))
	}

	p.objectsCache[objSpec] = o

	return o
}

func (p *Package) resolveObject(objSpec string) (types.Object, error) {
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
