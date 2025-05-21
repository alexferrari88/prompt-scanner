// scanner/go_parser.go
package scanner

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"

	"github.com/alexferrari88/prompt-scanner/utils" // Adjust import path
)

// ParseGoFile uses go/ast to find prompts in Go files.
func (s *Scanner) ParseGoFile(filePath string, contentBytes []byte) ([]FoundPrompt, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, contentBytes, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var prompts []FoundPrompt
	ext := filepath.Ext(filePath)
	varPath := make([]ast.Node, 0) // Keep track of path to current node

	ast.Inspect(node, func(n ast.Node) bool {
		if n == nil {
			if len(varPath) > 0 {
				varPath = varPath[:len(varPath)-1] // Pop from path
			}
			return true
		}
		varPath = append(varPath, n) // Push to path

		basicLit, ok := n.(*ast.BasicLit)
		if !ok || basicLit.Kind != token.STRING {
			return true
		}

		val, err := strconv.Unquote(basicLit.Value)
		if err != nil {
			// Attempt to handle raw strings that strconv.Unquote might not like if unterminated etc.
			if basicLit.Value[0] == '`' && basicLit.Value[len(basicLit.Value)-1] == '`' && len(basicLit.Value) >= 2 {
				val = basicLit.Value[1 : len(basicLit.Value)-1]
			} else if (basicLit.Value[0] == '"' || basicLit.Value[0] == '\'') && len(basicLit.Value) >= 2 {
				// Simple trim for malformed regular strings, won't unescape
				val = basicLit.Value[1 : len(basicLit.Value)-1]
			} else {
				val = basicLit.Value // Fallback to raw value if unquoting fails badly
			}
		}

		startLine := fset.Position(basicLit.Pos()).Line
		linesInContent := utils.CountNewlines(val) + 1
		isMultiLineExplicit := basicLit.Value[0] == '`' // Raw strings `...`

		var varName string
		// Traverse up the varPath to find an assignment or declaration
		for i := len(varPath) - 2; i >= 0; i-- { // Start from parent of BasicLit
			parentNode := varPath[i]
			if assignStmt, isAssign := parentNode.(*ast.AssignStmt); isAssign {
				// Check if basicLit is one of the RHS expressions
				for idx, rhsExpr := range assignStmt.Rhs {
					if rhsExpr == basicLit {
						if len(assignStmt.Lhs) > idx {
							if ident, isIdent := assignStmt.Lhs[idx].(*ast.Ident); isIdent {
								varName = ident.Name
								goto foundVarName // Exit loop once varName is found
							}
						}
					}
				}
			} else if valueSpec, isValueSpec := parentNode.(*ast.ValueSpec); isValueSpec { // Catches var x = "val" or const x = "val"
				for idx, valNode := range valueSpec.Values {
					if valNode == basicLit {
						if len(valueSpec.Names) > idx {
							varName = valueSpec.Names[idx].Name
							goto foundVarName // Exit loop
						}
					}
				}
			} else if _, isReturn := parentNode.(*ast.ReturnStmt); isReturn {
				// String literal is being returned, no direct variable name here
				// Could mark as "return_value" or similar if desired
				break // Stop searching up for var name in this case
			} else if _, isCall := parentNode.(*ast.CallExpr); isCall {
				// String literal is an argument to a function call
				break
			}
		}
	foundVarName:

		fp := FoundPrompt{
			Filepath:    filePath,
			Line:        startLine,
			Content:     val,
			IsMultiLine: isMultiLineExplicit || linesInContent > 1, // Fallback to content check
		}
		context := PromptContext{
			Text:                val,
			VariableName:        varName,
			IsMultiLineExplicit: isMultiLineExplicit,
			LinesInContent:      linesInContent,
			FileExtension:       ext,
		}

		if s.IsPotentialPrompt(context, &fp) {
			prompts = append(prompts, fp)
		}
		return true
	})

	return prompts, nil
}
