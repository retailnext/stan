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

type dirOverrideImporter struct {
	types.ImporterFrom
	dirs map[string]string
}

func importerWithDirOverride(dirs map[string]string) types.ImporterFrom {
	return dirOverrideImporter{ImporterFrom: imp, dirs: dirs}
}

func (i dirOverrideImporter) ImportFrom(path, srcDir string, mode types.ImportMode) (*types.Package, error) {
	if override := i.dirs[srcDir]; override != "" {
		srcDir = override
	}
	return i.ImporterFrom.ImportFrom(path, srcDir, mode)
}
