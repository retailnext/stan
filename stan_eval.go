// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package stan

import (
	"fmt"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
)

// EvalPkg() parses and type checks code into a *Package. EvalPkg() is useful
// for unit testing static analysis tests. Vendor imports operate as if the
// code was run from os.Getwd(). EvalPkg() panics if there is an error parsing
// or type checking code.
func EvalPkg(code string) *Package {
	tmpDir, err := ioutil.TempDir("", "stan_fake_package")
	if err != nil {
		panic(fmt.Sprintf("error making temp dir: %s", err))
	}

	defer os.RemoveAll(tmpDir)

	err = ioutil.WriteFile(filepath.Join(tmpDir, "fake_package.go"), []byte(code), 0644)
	if err != nil {
		panic(fmt.Sprintf("error writing fake_package.go: %s", err))
	}

	parsed, err := parseDir(tmpDir, token.NewFileSet())
	if err != nil {
		panic(fmt.Sprintf("error parsing fake package: %s", err))
	}

	pkg := parsed.code
	if pkg == nil {
		pkg = parsed.xtest
	}

	var packageName string
	for _, f := range pkg.pkg.Files {
		packageName = f.Name.Name
	}

	pkg.path = "fake/" + packageName

	wd, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("os.Getwd() error: %s", err))
	}

	return typeCheck(pkg, importerWithDirOverride(map[string]string{tmpDir: wd}))
}
