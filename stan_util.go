// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package stan

import (
	"fmt"
	"go/ast"
)

// Returns the "next" statement after a given node. It searches through ancestors
// until it finds the first BlockStmt, then it returns the next statement in the
// block relative to the statement of which n is a descendant.
func NextStmt(n ast.Node, ancs Ancestors) ast.Stmt {
	for ai := len(ancs) - 1; ai >= 0; ai-- {
		a := ancs[ai]

		block, _ := a.(*ast.BlockStmt)
		if block == nil {
			continue
		}

		var currStmt ast.Stmt
		if ai == len(ancs)-1 {
			currStmt = n.(ast.Stmt)
		} else {
			currStmt = ancs[ai+1].(ast.Stmt)
		}

		for bi, bs := range block.List {
			if bs == currStmt {
				if bi < len(block.List)-1 {
					return block.List[bi+1]
				}
				break
			}
		}

		break
	}

	return nil
}

// Walk the AST starting rooted at n yielding the slice of ancestors for
// each node visited. The ancestors slice is mutated during traversal so
// be sure to copy() the ancestors if you want to save them off.
func WalkAST(n ast.Node, fn func(node ast.Node, ancs Ancestors)) {
	if n == nil {
		panic(fmt.Sprintf("nil ast.Node passed to WalkAST"))
	}

	walker := &astWalker{
		fn: fn,
	}
	ast.Walk(walker, n)
}

type astWalker struct {
	ancestors Ancestors
	fn        func(node ast.Node, ancs Ancestors)
}

func (w *astWalker) Visit(node ast.Node) ast.Visitor {
	// finished walking children, remove self from ancestors
	if node == nil {
		w.ancestors = w.ancestors[:len(w.ancestors)-1]
		return nil
	}

	w.fn(node, w.ancestors)

	// add self to ancestors list for walking children
	w.ancestors = append(w.ancestors, node)

	return w
}

// Ancestors is a slice of ast.Nodes representing a node's ancestor
// nodes in the AST. A node's direct parent is the final node in the
// Ancestors.
type Ancestors []ast.Node

// Peek() returns the closest ancestor in a, or nil if a is empty.
func (a Ancestors) Peek() ast.Node {
	if len(a) == 0 {
		return nil
	}
	return a[len(a)-1]
}

// Pop() removes and returns the closest ancestor in a. Pop() returns nil if a
// is empty.
func (a *Ancestors) Pop() ast.Node {
	if len(*a) == 0 {
		return nil
	}
	ret := (*a)[len(*a)-1]
	*a = (*a)[:len(*a)-1]
	return ret
}
