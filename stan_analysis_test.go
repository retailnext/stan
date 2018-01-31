// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package stan

import (
	"go/types"
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
	bar.SearchObjects(func(o types.Object) bool {
		got[o.Name()] = true
		return true
	})

	for _, e := range expected {
		if !got[e] {
			t.Errorf("didn't get %s", e)
		}
	}
}
