package utils

import (
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/pkg/strings"
	"github.com/pkg/errors"
	"regexp"
)

func toFile(n ast.Node) (*ast.File, error) {
	switch x := n.(type) {
	case nil:
		return nil, nil
	case *ast.StructLit:
		decls := []ast.Decl{}
		for _, elt := range x.Elts {
			if _, ok := elt.(*ast.Ellipsis); ok {
				continue
			}
			decls = append(decls, elt)
		}
		return &ast.File{Decls: decls}, nil
	case ast.Expr:
		ast.SetRelPos(x, token.NoSpace)
		return &ast.File{Decls: []ast.Decl{&ast.EmbedDecl{Expr: x}}}, nil
	case *ast.File:
		return x, nil
	default:
		return nil, errors.Errorf("Unsupported node type %T", x)
	}
}
func ToFile(n ast.Node) (*ast.File, error) {
	return toFile(n)
}
func ListOpen(expr ast.Node) ast.Node {
	listOpen(expr)
	return expr
}

func listOpen(expr ast.Node) {
	switch v := expr.(type) {
	case *ast.File:
		for _, decl := range v.Decls {
			listOpen(decl)
		}
	case *ast.Field:
		listOpen(v.Value)
	case *ast.StructLit:
		for _, elt := range v.Elts {
			listOpen(elt)
		}
	case *ast.BinaryExpr:
		listOpen(v.X)
		listOpen(v.Y)
	case *ast.EmbedDecl:
		listOpen(v.Expr)
	case *ast.Comprehension:
		listOpen(v.Value)
	case *ast.ListLit:
		for _, elt := range v.Elts {
			listOpen(elt)
		}
		if len(v.Elts) > 0 {
			if _, ok := v.Elts[len(v.Elts)-1].(*ast.Ellipsis); !ok {
				v.Elts = append(v.Elts, &ast.Ellipsis{})
			}
		}
	}
}
func IndexMatchLine(ret, target string) (string, bool) {
	if strings.Contains(ret, target) {
		if target == "_|_" {
			r := regexp.MustCompile(`_\|_[\s]//.*`)
			match := r.FindAllString(ret, -1)
			if len(match) > 0 {
				return strings.Join(match, ","), true
			}
		}
	}
	return "", false
}
func CueValueToString(v cue.Value) (string, error) {
	sysopts := []cue.Option{cue.All(), cue.DisallowCycles(true), cue.ResolveReferences(true), cue.Docs(true)}
	f, err := ToFile(v.Syntax(sysopts...))
	if err != nil {
		return "", err
	}
	for _, decl := range f.Decls {
		ListOpen(decl)
	}

	ret, err := format.Node(f)
	if err != nil {
		return "", err
	}

	errInfo, contain := IndexMatchLine(string(ret), "_|_")
	if contain {
		return "", errors.New(errInfo)
	}
	return string(ret), nil
}
