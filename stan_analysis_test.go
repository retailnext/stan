// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package stan

import (
	"fmt"
	"go/types"
	"reflect"
	"testing"
)

func TestNotInBuild(t *testing.T) {
	bar := Pkgs("github.com/retailnext/stan/internal/bar")[0]

	expected := []string{
		"shared",
		"shared_nobuild1",
		"shared_nobuild2",
		"sharedVar",
		"sharedVar_nobuild1",
		"sharedVar_nobuild2",
		"sharedConst",
		"sharedConst_nobuild1",
		"sharedConst_nobuild2",
		"windowsDarwin",
		"windowsDarwin_nobuild1",
		"darwinSpecific",
		"linuxSpecific",
		"windowsSpecific",
	}

	got := make(map[string]bool)
	bar.IterateObjects(func(o types.Object) {
		got[o.Name()] = true
	})

	for _, e := range expected {
		if !got[e] {
			t.Errorf("didn't get %s", e)
		}
	}
}

// Make sure IterateObjects finds implicitly defined
// objects as well
func TestImplicitObjects(t *testing.T) {
	foo := Pkgs("github.com/retailnext/stan/internal/foo")[0]

	typ := foo.LookupType("github.com/retailnext/stan/internal/foo.ImplicitOnlyType")

	var objs []types.Object
	foo.IterateObjects(func(o types.Object) {
		if _, ok := o.(*types.TypeName); ok {
			return
		}
		if types.Identical(o.Type(), typ) {
			objs = append(objs, o)
		}
	})

	// one obj for implicit case def, one for use within case
	if l := len(objs); l != 2 {
		t.Errorf("found %d objs", l)
	}
}

func TestAncestorsOf(t *testing.T) {
	foo := Pkgs("github.com/retailnext/stan/internal/foo")[0]

	barFunc := foo.LookupObject("github.com/retailnext/stan/internal/bar.BarFunc")

	uses := foo.LifetimeOf(barFunc).Uses

	if len(uses) != 1 {
		t.Fatalf("got %d uses", len(uses))
	}

	ancs := foo.AncestorsOf(uses[0])

	var got []string
	for _, a := range ancs {
		got = append(got, fmt.Sprintf("%T", a))
	}

	expected := []string{
		"*ast.Package",
		"*ast.File",
		"*ast.FuncDecl",
		"*ast.BlockStmt",
		"*ast.AssignStmt",
		"*ast.CallExpr",
		"*ast.SelectorExpr",
	}

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("got %v", got)
	}
}

func TestInvocationsOf(t *testing.T) {
	foo := Pkgs("github.com/retailnext/stan/internal/foo")[0]

	invs := foo.InvocationsOf(foo.LookupObject("github.com/retailnext/stan/internal/bar.BarFunc"))
	if len(invs) != 1 {
		t.Fatalf("got %d", len(invs))
	}
	if invs[0].Invocant != foo.LookupObject("github.com/retailnext/stan/internal/foo.bar") {
		t.Errorf("got %s", invs[0].Invocant)
	}

	invs = foo.InvocationsOf(foo.LookupObject("github.com/retailnext/stan/internal/foo.structB.structFunc"))
	if len(invs) != 1 {
		t.Fatalf("got %d", len(invs))
	}
	if invs[0].Invocant.Name() != "b" {
		t.Errorf("got %s", invs[0].Invocant.Name())
	}
}
