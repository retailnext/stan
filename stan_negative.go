// Copyright (c) 2018, RetailNext, Inc.
// This material contains trade secrets and confidential information of
// RetailNext, Inc.  Any use, reproduction, disclosure or dissemination
// is strictly prohibited without the explicit written permission
// of RetailNext, Inc.
// All rights reserved.

package stan

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

type StaticTest func(*Package) []error

func EvalTest(st StaticTest, code string) []error {
	fset := token.NewFileSet()

	fakeFileName := "fake_package.go"

	f, err := parser.ParseFile(fset, fakeFileName, code, 0)
	if err != nil {
		panic(fmt.Sprintf("error parsing code: %s", err))
	}

	pkg := &parsedPackage{
		pkg: &ast.Package{
			Name:  f.Name.Name,
			Files: map[string]*ast.File{fakeFileName: f},
		},
		fset:       fset,
		buildFiles: []*ast.File{f},
		path:       "fake/" + f.Name.Name,
	}

	return st(typeCheck(pkg))
}
