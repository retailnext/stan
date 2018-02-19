// Copyright (c) 2018, RetailNext, Inc.
// This material contains trade secrets and confidential information of
// RetailNext, Inc.  Any use, reproduction, disclosure or dissemination
// is strictly prohibited without the explicit written permission
// of RetailNext, Inc.
// All rights reserved.

package stan

import (
	"fmt"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
)

type StaticTest func(*Package) []error

func EvalTest(st StaticTest, code string) []error {
	tmpDir, err := ioutil.TempDir("", "stan_fake_package")
	if err != nil {
		panic(fmt.Sprintf("error making temp dir: %s", err))
	}

	defer os.RemoveAll(tmpDir)

	err = ioutil.WriteFile(filepath.Join(tmpDir, "fake_package.go"), []byte(code), 0644)
	if err != nil {
		panic(fmt.Sprintf("error writing fake_package.go: %s", err))
	}

	codePkg, xtestPkg, err := parseDir(tmpDir, token.NewFileSet())
	if err != nil {
		panic(fmt.Sprintf("error parsing fake package: %s", err))
	}

	pkg := codePkg
	if pkg == nil {
		pkg = xtestPkg
	}

	var packageName string
	for _, f := range pkg.pkg.Files {
		packageName = f.Name.Name
	}

	pkg.path = "fake/" + packageName

	return st(typeCheck(pkg))
}
