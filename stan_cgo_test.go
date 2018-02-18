// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package stan

import (
	"go/types"
	"testing"
)

func TestStanCgo(t *testing.T) {
	baz := Pkgs("github.com/retailnext/stan/internal/foo")[0]

	var cgoInt types.Object
	baz.IterateObjects(func(o types.Object) {
		if o.Name() == "cgoIntArg" {
			cgoInt = o
		}
	})

	if cgoInt == nil {
		t.Fatal("no cgo int object")
	}

	if ts := cgoInt.Type().String(); ts != "github.com/retailnext/stan/internal/foo._Ctype_int" {
		t.Errorf("got %s", ts)
	}
}
