// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package stan

import "go/ast"

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
