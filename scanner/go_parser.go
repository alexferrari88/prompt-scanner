// scanner/go_parser.go
package scanner

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"

	"github.com/alexferrari88/prompt-scanner/utils"
)

// ParseGoFile uses go/ast to find prompts in Go files.
func (s *Scanner) ParseGoFile(filePath string, contentBytes []byte) ([]FoundPrompt, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, contentBytes, parser.ParseComments|parser.SkipObjectResolution)
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

		for i := len(varPath) - 2; i >= 0; i-- {
			parentNode := varPath[i]

			if assignStmt, isAssign := parentNode.(*ast.AssignStmt); isAssign {
				for idx, rhsExpr := range assignStmt.Rhs {
					if rhsExpr == n {
						if len(assignStmt.Lhs) > idx {
							if ident, isIdent := assignStmt.Lhs[idx].(*ast.Ident); isIdent {
								varName = ident.Name
								goto foundPrimaryContext
							}
						}
					}
				}
			} else if valueSpec, isValueSpec := parentNode.(*ast.ValueSpec); isValueSpec {
				for idx, valNode := range valueSpec.Values {
					if valNode == n {
						if len(valueSpec.Names) > idx {
							varName = valueSpec.Names[idx].Name
							goto foundPrimaryContext
						}
					}
				}
			} else if callExpr, isCall := parentNode.(*ast.CallExpr); isCall {
				isArg := false
				for _, arg := range callExpr.Args {
					if arg == n {
						isArg = true
						break
					}
				}
				if isArg {
					switch fun := callExpr.Fun.(type) {
					case *ast.Ident: // Direct function call like Println("..."), or panic("...")
						invFuncName = fun.Name
						// No special receiver for direct calls like panic() or global Error()
					case *ast.SelectorExpr: // Method call like logger.Info("..."), errors.New("...")
						if xIdent, ok := fun.X.(*ast.Ident); ok {
							invReceiverName = xIdent.Name // "errors", "fmt", "logger"
						}
						invFuncName = fun.Sel.Name // "New", "Errorf", "Info"
					}
					if varName == "" {
						goto foundPrimaryContext
					}
				}
			}
			// Removed incorrect ast.PanicStmt and token.NEW checks.
			// `panic` calls are handled by the *ast.CallExpr case where Fun is an *ast.Ident named "panic".
			// `throw` is not a Go keyword.
		}
	foundPrimaryContext:

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
