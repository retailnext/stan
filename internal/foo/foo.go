// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package foo

import "github.com/retailnext/stan/internal/bar"

func FooFunc() {
	val := bar.BarFunc()
	val = bar.BarVar
	_ = val
}

var myIntArray [10]int
