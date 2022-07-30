package main

import (
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/load"
	"fmt"
	"log"
)

// 无需纠结这段代码  .就是用来打印 而已
func printVal(val cue.Value) {
	syn := val.Syntax(
		cue.Final(),         // close structs and lists
		cue.Concrete(false), // allow incomplete values
		cue.Definitions(false),
		cue.Hidden(true),
		cue.Optional(true),
		cue.Attributes(true),
		cue.Docs(true),
	)

	// Pretty print the AST, returns ([]byte, error)
	bs, _ := format.Node(
		syn,
		format.TabIndent(false),
		format.UseSpaces(2),
	)
	fmt.Println(string(bs))
}

func main() {

	binst := load.Instances([]string{"./pkg/cues/fast/nginx.cue"}, nil)
	insts := cue.Build(binst)
	doc := insts[0]
	input := doc.Value().LookupPath(cue.ParsePath("#input"))
	//nsList := doc.Value().LookupPath(cue.ParsePath("#namespace"))
	newDoc := input.FillPath(cue.ParsePath("namespace"), strNsList(doc.Value().Context()))
	if err := newDoc.Err(); err != nil {
		log.Fatalln(err)
	}
	printVal(newDoc)
}

func strNsList(cc *cue.Context) cue.Value {
	ns := `
      #namespaces: "default" | "bcd" | "12345"
`
	v := cc.CompileString(ns)
	return v.LookupPath(cue.ParsePath("#namespaces"))

}
