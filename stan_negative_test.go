// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package stan

import (
	"fmt"
	"go/types"
	"testing"
)

func TestNegativeTest(t *testing.T) {
	got := EvalTest(myTest, `
package banana

func DontLook() {
	var Banana = "monkey"
}
`)

	if len(got) != 1 {
		t.Errorf("wanted 1 error, got %v", got)
	}

	got = EvalTest(myTest, `
package banana

func DontLook() {
	var Monkey = "banana"
}
`)

	if len(got) != 0 {
		t.Errorf("wanted 0 errors, got %v", got)
	}
}

func myTest(pkg *Package) []error {
	var ret []error

	pkg.SearchObjects(func(o types.Object) bool {
		if o.Name() == "Banana" {
			ret = append(ret, fmt.Errorf("Object using the forbidden name at %s", pkg.Pos(o)))
		}

		return false
	})

	return ret
}