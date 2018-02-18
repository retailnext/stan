// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package stan

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/gcexportdata"
)

var (
	imports = make(map[string]*types.Package)
	fset    = token.NewFileSet()
	imp     = gcexportdata.NewImporter(fset, imports)
)
