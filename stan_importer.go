// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package stan

import (
	"go/importer"
	"go/types"
	"sync"
)

var imp types.Importer

func init() {
	imp = newParallelImporter(importer.Default())
}

type parallelImporter struct {
	sync.Mutex
	imp types.Importer
}

func (i *parallelImporter) Import(path string) (*types.Package, error) {
	i.Lock()
	defer i.Unlock()
	return i.imp.Import(path)
}

type parallelImportFromer struct {
	parallelImporter
	imp types.ImporterFrom
}

func (i *parallelImportFromer) ImportFrom(path, dir string, mode types.ImportMode) (*types.Package, error) {
	i.Lock()
	defer i.Unlock()
	return i.imp.ImportFrom(path, dir, mode)
}

func newParallelImporter(i types.Importer) types.Importer {
	if impFrom, ok := i.(types.ImporterFrom); ok {
		return &parallelImportFromer{
			parallelImporter: parallelImporter{imp: i},
			imp:              impFrom,
		}
	} else {
		return &parallelImporter{imp: i}
	}
}
