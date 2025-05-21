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
	varPath := make([]ast.Node, 0)

	ast.Inspect(node, func(n ast.Node) bool {
		if n == nil {
			if len(varPath) > 0 {
				varPath = varPath[:len(varPath)-1]
			}
			return true
		}
		varPath = append(varPath, n)

		basicLit, ok := n.(*ast.BasicLit)
		if !ok || basicLit.Kind != token.STRING {
			return true
		}

		val, err := strconv.Unquote(basicLit.Value)
		if err != nil {
			if basicLit.Value[0] == '`' && basicLit.Value[len(basicLit.Value)-1] == '`' && len(basicLit.Value) >= 2 {
				val = basicLit.Value[1 : len(basicLit.Value)-1]
			} else if (basicLit.Value[0] == '"' || basicLit.Value[0] == '\'') && len(basicLit.Value) >= 2 {
				val = basicLit.Value[1 : len(basicLit.Value)-1]
			} else {
				val = basicLit.Value
			}
		}

		startLine := fset.Position(basicLit.Pos()).Line
		linesInContent := utils.CountNewlines(val) + 1
		isMultiLineExplicit := basicLit.Value[0] == '`'

		var varName, invFuncName, invReceiverName string

		// Traverse up the varPath
		for i := len(varPath) - 2; i >= 0; i-- {
			parentNode := varPath[i]

			if assignStmt, isAssign := parentNode.(*ast.AssignStmt); isAssign {
				for idx, rhsExpr := range assignStmt.Rhs {
					if rhsExpr == basicLit || (rhsExpr == n && n == basicLit) { // Check if current node is the RHS
						if len(assignStmt.Lhs) > idx {
							if ident, isIdent := assignStmt.Lhs[idx].(*ast.Ident); isIdent {
								varName = ident.Name
								goto foundContext // Found primary context (assignment)
							}
						}
					}
				}
			} else if valueSpec, isValueSpec := parentNode.(*ast.ValueSpec); isValueSpec {
				for idx, valNode := range valueSpec.Values {
					if valNode == basicLit || (valNode == n && n == basicLit) {
						if len(valueSpec.Names) > idx {
							varName = valueSpec.Names[idx].Name
							goto foundContext // Found primary context (declaration)
						}
					}
				}
			} else if callExpr, isCall := parentNode.(*ast.CallExpr); isCall {
				// Check if basicLit is one of the arguments
				isArg := false
				for _, arg := range callExpr.Args {
					if arg == basicLit || (arg == n && n == basicLit) {
						isArg = true
						break
					}
				}
				if isArg {
					// It's an argument. Try to get function name.
					switch fun := callExpr.Fun.(type) {
					case *ast.Ident: // Direct function call, e.g., Println("...")
						invFuncName = fun.Name
					case *ast.SelectorExpr: // Method call, e.g., logger.Info("...") or fmt.Println("...")
						if xIdent, ok := fun.X.(*ast.Ident); ok {
							invReceiverName = xIdent.Name
						}
						invFuncName = fun.Sel.Name
					}
					// If part of a call, we don't consider it an assignment for varName purposes
					// unless it's also assigned, e.g. x := logger.Info("...") - this is complex to chain.
					// For now, if it's a direct argument to a call, prioritize call context.
					if varName == "" { // Only if not already part of an assignment
						goto foundContext
					}
				}
			}
			// If we found a varName, don't let a higher-level call overwrite it as primary context
			// but do record the call if the var is then used in a call. This needs more complex state.
			// For now, the closest context (assignment or call arg) wins.
		}
	foundContext:

		fp := FoundPrompt{
			Filepath:    filePath,
			Line:        startLine,
			Content:     val,
			IsMultiLine: isMultiLineExplicit || linesInContent > 1,
		}
		context := PromptContext{
			Text:                   val,
			VariableName:           varName,
			IsMultiLineExplicit:    isMultiLineExplicit,
			LinesInContent:         linesInContent,
			FileExtension:          ext,
			InvocationFunctionName: invFuncName,
			InvocationReceiverName: invReceiverName,
		}

		if s.IsPotentialPrompt(context, &fp) {
			prompts = append(prompts, fp)
		}
		return true
	})
	return prompts, nil
}
