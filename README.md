# stan

Short for STatic ANalysis, stan's goal is to make it easier to write custom static analysis tests for your golang project.

In additional to *ast.Package, *types.Info and *types.Package, stan provides a higher level API to make it easier when your objective relates to particular objects/types.

stan's API and feature set is not stable.

## Standard walk-the-AST approach

```go
// don't compare time.Time values with ==

for _, pkg := range stan.Pkgs("your/namespace/...") {
  timeDotTime := pkg.LookupType("time.Time")

  stan.WalkAST(pkg.Node, func(n ast.Node, ancs stan.Ancestors) {
    binary, _ := n.(*ast.BinaryExpr)
    if binary == nil {
      return
    }

    if binary.Op != token.EQL && binary.Op != token.NEQ {
      return
    }

    if types.Identical(pkg.TypeOf(binary.X), timeDotTime) {
      t.Errorf("Use Equal() to compare time.Time values instead of == at %s", pkg.Pos(binary))
    }
  })
}
```

## Object based approach

```go

// find *csv.Writer users that call Flush() but never check Error()

for _, pkg := range stan.Pkgs("your/namespace/...") {
  naughtyWriters := make(map[types.Object]bool)

  csvWriterFlush := pkg.LookupObject("encoding/csv.Writer.Flush")
  for _, inv := range pkg.InvocationsOf(csvWriterFlush) {
    naughtyWriters[inv.Invocant] = true
  }

  csvWriterError := pkg.LookupObject("encoding/csv.Writer.Error")
  for _, inv := range pkg.InvocationsOf(csvWriterError) {
    delete(naughtyWriters, inv.Invocant)
  }

  for naughty := range naughtyWriters {
    t.Errorf("*csv.Writer calls Flush() but not Error() at %s", pkg.Pos(naughty))
  }
}
```

## Test your static tests

```go

func checkUseTimeEqual(pkg *stan.Package) []error {
  // above test to catch code comparing time.Time with ==
}

func TestUseTimeEqual(t *testing.T) {
  // invoke static check on your code
  for _, pkg := range stan.Pkgs("your/namespace/...") {
    for _, err := range checkUseTimeEquals(pkg) {
      t.Error(err)
    }
  }

  // unit test your static test
  errs := stan.EvalTest(checkUseTimeEqual, `
package fake

import "time"

func foo() {
  now := time.Now()
  if now != now.Round(0) {
    panic("oops!")
  }
}
`)

  if len(errs) != 1 {
    t.Error("expected an error")
  }

  errs = stan.EvalTest(checkUseTimeEqual, `
package fake

import "time"

func foo() {
  now := time.Now()
  if !now.Equal(now.Round(0)) {
    panic("oops!")
  }
}
`)

  if len(errs) != 0 {
    t.Error("expected no errors")
  }
}
```
