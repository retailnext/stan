// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package stan

import (
	"go/types"
	"reflect"
	"strings"
	"testing"
)

func TestLookup(t *testing.T) {
	for _, pkg := range Pkgs("github.com/retailnext/stan/internal/...") {
		if strings.HasSuffix(pkg.Path(), ":xtest") {
			continue
		}

		// show that lookupd type works for both a package that imports the type
		// and the package that defines it
		barType := pkg.LookupType("github.com/retailnext/stan/internal/bar.BarType")

		var objs []types.Object
		pkg.IterateObjects(func(o types.Object) {
			if types.Identical(o.Type(), barType) {
				objs = append(objs, o)
			}
		})
		if len(objs) == 0 {
			t.Errorf("%s has no objects of type %s", pkg.Path(), barType)
		}

		// similar, but for object
		barObj := pkg.LookupObject("github.com/retailnext/stan/internal/bar.BarVar")
		if len(pkg.SpanOf(barObj).Uses) == 0 {
			t.Errorf("%s has no uses of object %s", pkg.Path(), barObj)
		}
	}
}

func TestLookupBuiltinType(t *testing.T) {
	foo := Pkgs("github.com/retailnext/stan/internal/foo")[0]
	obj := foo.LookupObject("github.com/retailnext/stan/internal/foo.myIntArray")
	if !types.Identical(obj.Type(), foo.LookupType("[10]int")) {
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
	foo[0].LookupObject("github.com/retailnext/stan/internal/foo.FooFunc")
	foo[1].LookupObject("github.com/retailnext/stan/internal/foo:xtest.FooFunc")

	// if you ask for package directly, you get code package
	singleFoo := Pkgs("github.com/retailnext/stan/internal/foo")
	if !reflect.DeepEqual(singleFoo, []*Package{foo[0]}) {
		t.Errorf("got %v", singleFoo)
	}
}
