// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package stan

import (
	"fmt"
	"go/types"
	"testing"
)

func TestEvalPkg(t *testing.T) {
	pkg := EvalPkg(`
package banana

func DontLook() {
	var Banana = "monkey"
}
`)

	if errs := myTest(pkg); len(errs) != 1 {
		t.Errorf("wanted 1 error, got %v", errs)
	}

	pkg = EvalPkg(`
package banana

// void banana_func() { }
import "C"

func DontLook() {
	var Monkey = "banana"
	_ = C.banana_func()
}
`)

	if errs := myTest(pkg); len(errs) != 0 {
		t.Errorf("wanted 0 errors, got %v", errs)
	}
}

func myTest(pkg *Package) []error {
	var ret []error

	pkg.IterateObjects(func(o types.Object) {
		if o.Name() == "Banana" {
			ret = append(ret, fmt.Errorf("Object using the forbidden name at %s", pkg.Pos(o)))
		}
	})

	return ret
}
