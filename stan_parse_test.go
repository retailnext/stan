// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package stan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// make sure our package traversal finds the same stuff as go list
func TestListPackages(t *testing.T) {
	var expectedPaths []string
	for _, pkg := range listPackages([]string{"github.com/retailnext/stan/..."}) {
		expectedPaths = append(expectedPaths, pkg.ImportPath)
	}

	var gotPaths []string
	for _, pkg := range findAndParse([]string{"github.com/retailnext/stan/..."})[0] {
		// we return separate packages with pseudo import paths for the
		// _test packages
		if strings.HasSuffix(pkg.path, ":xtest") || strings.Contains(pkg.path, ":nobuild") {
			continue
		}
		gotPaths = append(gotPaths, pkg.path)
	}

	sort.Strings(expectedPaths)
	sort.Strings(gotPaths)

	if !reflect.DeepEqual(gotPaths, expectedPaths) {
		t.Errorf("got %v, expected %v", gotPaths, expectedPaths)
	}
}

func listPackages(paths []string) []goListPackage {
	cmd := exec.Command("go", append([]string{"list", "-json"}, paths...)...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	p, err := cmd.StdoutPipe()
	if err != nil {
		panic(fmt.Sprintf("error getting go list pipe: %s", err))
	}

	if err := cmd.Start(); err != nil {
		panic(fmt.Sprintf("error running go list: %s", err))
	}

	var pkgs []goListPackage

	dec := json.NewDecoder(p)
	for {
		var pkg goListPackage
		err := dec.Decode(&pkg)
		if err == io.EOF {
			break
		} else if err != nil {
			panic(fmt.Sprintf("error decoding go list output: %s", err))
		}

		pkgs = append(pkgs, pkg)
	}

	if err := cmd.Wait(); err != nil {
		panic(fmt.Sprintf("error running go list: %s (%s)", stderr.String(), err))
	}

	return pkgs
}

type goListPackage struct {
	ImportPath string // import path of package in dir
}
