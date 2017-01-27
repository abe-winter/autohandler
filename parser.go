package main

import (
	"fmt"
	"bytes"
	"go/ast"
	"strings"
	"go/token"
	"go/types"
	"go/format"

	"golang.org/x/tools/go/gcexportdata"
)

type Visitor struct {
	Fset      *token.FileSet
	Package   string            // this is because types.Check resolves names as package.Name
	Mimetypes map[string]string // maps typename to mimetype parsed from comment
	Functions map[string]*ast.FuncDecl
	Methods   map[string]map[string]*ast.FuncDecl
}

// this is horrible
// for strings thou art and to strings thou shall return
func FormatReceiver(pkg string, decl *ast.FuncDecl, with_pkg bool) string {
	if len(decl.Recv.List) != 1 {
		panic("expected len=1 for receiver list")
	}
	addPkg := func(raw string) string {
		if with_pkg {
			return pkg + "." + raw
		} else {
			return raw
		}
	}
	switch rt := decl.Recv.List[0].Type.(type) {
	case *ast.StarExpr:
		return "*" + addPkg(rt.X.(*ast.Ident).Name)
	case *ast.Ident:
		return addPkg(rt.Name)
	default:
		panic("unk type in receiver")
	}
}

func (self *Visitor) Visit(node ast.Node) ast.Visitor {
	switch tn := node.(type) {
	case *ast.FuncDecl:
		if tn.Recv != nil {
			if self.Methods == nil {
				self.Methods = make(map[string]map[string]*ast.FuncDecl)
			}
			recv_type := FormatReceiver(self.Package, tn, true)
			if self.Methods[recv_type] == nil {
				self.Methods[recv_type] = make(map[string]*ast.FuncDecl)
			}
			self.Methods[recv_type][tn.Name.String()] = tn
		} else {
			if self.Functions == nil {
				self.Functions = make(map[string]*ast.FuncDecl)
			}
			self.Functions[tn.Name.String()] = tn
		}
	case *ast.TypeSpec:
		if tn.Comment != nil {
			comment := tn.Comment.List[0].Text
			if strings.HasPrefix(comment, "//mimetype ") {
				mimetype := strings.Split(comment, " ")[1]
				if self.Mimetypes == nil {
					self.Mimetypes = make(map[string]string)
				}
				self.Mimetypes[self.Package+"."+tn.Name.String()] = mimetype
			}
		}
	}
	return self
}

// scrape methods & mimetypes from the file
// this needs redesign -- it should parse a whole package
// that means no more gogen files -- instead, create a pkgname_handlers.go
func Scrape(fset *token.FileSet, p *ast.Package) *Visitor {
	v := &Visitor{Package: p.Name, Fset: fset}
	ast.Walk(ast.Visitor(v), p)
	return v
}

type FoundFunction struct {
	decl     *ast.FuncDecl
	tp       *types.Func
	mimetype string
}

// return list of FuncDecls that need to be wrapped
func Candidates(v *Visitor, pkg *ast.Package) []FoundFunction {
	info := types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}
	fmt.Printf("methods %v\n", v.Methods)
	fmt.Printf("functions %v\n", v.Functions)
	fmt.Printf("mimetypes %v\n", v.Mimetypes)
	conf := types.Config{
		Importer: gcexportdata.NewImporter(v.Fset, make(map[string]*types.Package)),
		IgnoreFuncBodies: true,
	}
	files := make([]*ast.File, 0, len(pkg.Files))
	for _, v := range pkg.Files {
		files = append(files, v)
	}
	_, err := conf.Check(pkg.Name, v.Fset, files, &info)
	if err != nil {
		panic(err)
	}
	found_functions := make([]FoundFunction, 0, 5)
	for _, obj := range info.Defs {
		switch decl := obj.(type) {
		case *types.Func:
			sig := decl.Type().(*types.Signature)
			if sig.Results().Len() != 2 {
				continue
			}
			if fmt.Sprintf("%v", sig.Results().At(1).Type().String()) != "int" {
				continue
			}
			ret := sig.Results().At(0)
			if mimetype, ok := v.Mimetypes[ret.Type().String()]; ok {
				if sig.Recv() != nil {
					if methods, ok := v.Methods[sig.Recv().Type().String()]; ok {
						if fn, ok := methods[decl.Name()]; ok {
							found_functions = append(found_functions, FoundFunction{fn, decl, mimetype})
						}
					}
				} else {
					panic("todo: look up non-method functions")
				}
			}
		}
	}
	return found_functions
}

const JSONSUB = `func (self %s) %s(w http.ResponseWriter, req *http.Request){
    w.Header().Set("Content-Type", "%s")
    raw := make([]byte, 0, 0)
    if _, err := req.Body.Read(raw); err != nil {panic(err)}
    var parsed map[string]interface{}
    if err := json.Unmarshal(raw, &parsed); err != nil {panic(err)}
    body, retcode := self.%s(%s)
    w.WriteHeader(retcode)
    io.WriteString(w, string(body))
}
`

const RAWSUB = `func (self %s) %s(w http.ResponseWriter, req *http.Request){
	w.Header().Set("Content-Type", "%s")
    body, retcode := self.%s(%s)
    w.WriteHeader(retcode)
    io.WriteString(w, string(body))
}
`

func (self *Visitor) FormatNode(node interface{}) string {
	buf := bytes.Buffer{}
	format.Node(&buf, self.Fset, node)
	return buf.String()
}

func makeWrappers(v *Visitor, funcs []FoundFunction) []string {
	wrappers := make([]string, 0, len(funcs))
	for _, ff := range funcs {
		decl := ff.decl
		argstrings := make([]string, 0, len(decl.Type.Params.List))
		any_json_args := false
		for _, field := range decl.Type.Params.List {
			typename := v.FormatNode(field.Type)
			if typename == "*http.Request" {
				argstrings = append(argstrings, "req")
			} else {
				for _, name := range field.Names {
					any_json_args = true
					argstrings = append(argstrings, fmt.Sprintf(`parsed["%s"].(%s)`, name.Name, typename))
				}
			}
		}
		var template_string string
		if any_json_args {template_string = JSONSUB} else {template_string = RAWSUB}
		wrappers = append(
			wrappers,
			fmt.Sprintf(
				template_string,
				FormatReceiver("", decl, false),
				"Handle"+decl.Name.Name,
				ff.mimetype,
				decl.Name.Name,
				strings.Join(argstrings, ", "),
			),
		)
	}
	return wrappers
}
