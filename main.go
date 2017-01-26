package main

import (
    "os"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
)

const PREAMBLE = `package %s
import (
    "io"
    "net/http"
    "encoding/json"
)
`

func main() {
	pkg := flag.String("pkg", "required", "name of package to parse")
	out := flag.String("out", "required", "output file for wrappers")
	flag.Parse()
	fset := token.NewFileSet()
	// func ParseDir(fset *token.FileSet, path string, filter func(os.FileInfo) bool, mode Mode) (pkgs map[string]*ast.Package, first error)
	packages, err := parser.ParseDir(fset, ".", nil, parser.ParseComments)
	if err != nil {
		panic(err)
	}
	if ast_, ok := packages[*pkg]; !ok {
		panic("package not found " + *pkg)
	} else {
		visitor := Scrape(fset, ast_)
		found := Candidates(visitor, ast_)
		fmt.Printf("found %d to wrap\n", len(found))
		wrappers := makeWrappers(visitor, found)
        if f, err := os.Create(*out); err != nil {
            panic(err)
        } else {
            defer f.Close()
            f.WriteString(fmt.Sprintf(PREAMBLE, *pkg))
            for _, wrapper := range wrappers {
                f.WriteString(wrapper)
    		}
        }
	}
}
