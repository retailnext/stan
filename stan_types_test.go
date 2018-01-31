// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package stan

import (
	"go/types"
	"reflect"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	for _, pkg := range Pkgs("github.com/retailnext/stan/internal/...") {
		if strings.HasSuffix(pkg.Path(), ":xtest") {
			continue
		}

		// show that resolved type works for both a package that imports the type
		// and the package that defines it
		barType := pkg.ResolveType("github.com/retailnext/stan/internal/bar.BarType")
		objs := pkg.SearchObjects(func(o types.Object) bool {
			return types.Identical(o.Type(), barType)
		})
		if len(objs) == 0 {
			t.Errorf("%s has no objects of type %s", pkg.Path(), barType)
		}

		// similar, but for object
		barObj := pkg.ResolveObject("github.com/retailnext/stan/internal/bar.BarVar")
		if len(pkg.SpanOf(barObj).Uses) == 0 {
			t.Errorf("%s has no uses of object %s", pkg.Path(), barObj)
		}
	}
}

func TestResolveBuiltinType(t *testing.T) {
	foo := Pkgs("github.com/retailnext/stan/internal/foo")[0]
	obj := foo.ResolveObject("github.com/retailnext/stan/internal/foo.myIntArray")
	if !types.Identical(obj.Type(), foo.ResolveType("[10]int")) {
		t.Error("types didn't match")
	}
}

// test special handling of _test packages
func TestXTestPackage(t *testing.T) {
	foo := Pkgs("github.com/retailnext/stan/internal/foo...")

	if p := foo[0].Path(); p != "github.com/retailnext/stan/internal/foo" {
		t.Errorf("got %s", p)
	}

	if p := foo[1].Path(); p != "github.com/retailnext/stan/internal/foo:xtest" {
		t.Errorf("got %s", p)
	}
	if p := foo[1].Node.Name; p != "foo_test" {
		t.Errorf("got %s", p)
	}

	// they both have FooFunc()
	foo[0].ResolveObject("github.com/retailnext/stan/internal/foo.FooFunc")
	foo[1].ResolveObject("github.com/retailnext/stan/internal/foo:xtest.FooFunc")

	// if you ask for package directly, you get code package
	singleFoo := Pkgs("github.com/retailnext/stan/internal/foo")
	if !reflect.DeepEqual(singleFoo, Packages{foo[0]}) {
		t.Errorf("got %v", singleFoo)
	}
}