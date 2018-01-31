// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the GO_LICENSE file.

package stan

import (
	"regexp"
	"strings"
)

// below functions are copied verbatim from cmd/go/internal/load/{search,path}.go

func hasPathPrefix(s, prefix string) bool {
	switch {
	default:
		return false
	case len(s) == len(prefix):
		return s == prefix
	case len(s) > len(prefix):
		if prefix != "" && prefix[len(prefix)-1] == '/' {
			return strings.HasPrefix(s, prefix)
		}
		return s[len(prefix)] == '/' && s[:len(prefix)] == prefix
	}
}

func treeCanMatchPattern(pattern string) func(name string) bool {
	wildCard := false
	if i := strings.Index(pattern, "..."); i >= 0 {
		wildCard = true
		pattern = pattern[:i]
	}
	return func(name string) bool {
		return len(name) <= len(pattern) && hasPathPrefix(pattern, name) ||
			wildCard && strings.HasPrefix(name, pattern)
	}
}

func matchPattern(pattern string) func(name string) bool {

	const vendorChar = "\x00"

	if strings.Contains(pattern, vendorChar) {
		return func(name string) bool { return false }
	}

	re := regexp.QuoteMeta(pattern)
	re = replaceVendor(re, vendorChar)
	switch {
	case strings.HasSuffix(re, `/`+vendorChar+`/\.\.\.`):
		re = strings.TrimSuffix(re, `/`+vendorChar+`/\.\.\.`) + `(/vendor|/` + vendorChar + `/\.\.\.)`
	case re == vendorChar+`/\.\.\.`:
		re = `(/vendor|/` + vendorChar + `/\.\.\.)`
	case strings.HasSuffix(re, `/\.\.\.`):
		re = strings.TrimSuffix(re, `/\.\.\.`) + `(/\.\.\.)?`
	}
	re = strings.Replace(re, `\.\.\.`, `[^`+vendorChar+`]*`, -1)

	reg := regexp.MustCompile(`^` + re + `$`)

	return func(name string) bool {
		if strings.Contains(name, vendorChar) {
			return false
		}
		return reg.MatchString(replaceVendor(name, vendorChar))
	}
}

func replaceVendor(x, repl string) string {
	if !strings.Contains(x, "vendor") {
		return x
	}
	elem := strings.Split(x, "/")
	for i := 0; i < len(elem)-1; i++ {
		if elem[i] == "vendor" {
			elem[i] = repl
		}
	}
	return strings.Join(elem, "/")
}
