package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The Dashboard's "Recent activity" says each audit action in the interface
// language (set16 observation O3: raw ids such as server.setup.child:nginx on
// both installed servers). The web reads auditActionNames and requires an
// English and a Turkish sentence for each name. This test keeps that list
// complete without trusting anyone to remember it: it parses this package's
// source, finds every INSERT INTO audit_logs, follows the action argument back
// through parameters, local variables, closures, string concatenation,
// fmt.Sprintf and string-returning helpers to the literals it is built from,
// and requires every name it can produce to be listed (and every listed name to
// be producible). A part that is decided at run time (a request's action, an
// outcome) is a wildcard; the list names its values.
//
// Panodaki "Son etkinlik" her denetim eylemini arayüz dilinde söyler. Web,
// auditActionNames listesini okur ve her ad için İngilizce ve Türkçe cümle
// ister. Bu test listeyi kaynaktan türeterek eksiksiz tutar: her
// INSERT INTO audit_logs bulunur, eylem bağımsız değişkeni parametreler, yerel
// değişkenler, kapanışlar, birleştirme ve fmt.Sprintf üzerinden kurulduğu
// sabitlere dek izlenir. Çalışma anında belirlenen parça jokerdir; liste
// değerlerini adlandırır.

const auditWildcard = "\x00"

type auditAnalysis struct {
	t        *testing.T
	consts   map[string]ast.Expr
	funcs    map[string][]*ast.FuncDecl
	calls    map[string][]*ast.CallExpr
	assigned map[*ast.Object][]ast.Expr
	params   map[*ast.Object]auditParam
	viaType  map[string][]*ast.CallExpr
	encl     map[*ast.CallExpr]*ast.FuncDecl
	memo     map[string][]string
	active   map[string]bool
}

type auditParam struct {
	owner string      // a FuncDecl's name, or "" for a closure
	local *ast.Object // the variable a closure is assigned to
	index int
}

func loadAuditAnalysis(t *testing.T) *auditAnalysis {
	t.Helper()
	a := &auditAnalysis{
		t: t, consts: map[string]ast.Expr{}, funcs: map[string][]*ast.FuncDecl{},
		calls: map[string][]*ast.CallExpr{}, assigned: map[*ast.Object][]ast.Expr{},
		params: map[*ast.Object]auditParam{}, viaType: map[string][]*ast.CallExpr{},
		encl: map[*ast.CallExpr]*ast.FuncDecl{}, memo: map[string][]string{}, active: map[string]bool{},
	}
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, path, source, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.GenDecl:
				if decl.Tok == token.CONST {
					for _, spec := range decl.Specs {
						spec := spec.(*ast.ValueSpec)
						for i, name := range spec.Names {
							if i < len(spec.Values) {
								a.consts[name.Name] = spec.Values[i]
							}
						}
					}
				}
			case *ast.FuncDecl:
				a.funcs[decl.Name.Name] = append(a.funcs[decl.Name.Name], decl)
				a.recordParams(decl.Type, auditParam{owner: decl.Name.Name})
				if decl.Body != nil {
					enclosing := decl
					ast.Inspect(decl.Body, func(node ast.Node) bool {
						if call, ok := node.(*ast.CallExpr); ok {
							a.encl[call] = enclosing
						}
						return true
					})
				}
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.CallExpr:
				switch fun := node.Fun.(type) {
				case *ast.Ident:
					a.calls[fun.Name] = append(a.calls[fun.Name], node)
					if fun.Obj != nil {
						if field, ok := fun.Obj.Decl.(*ast.Field); ok {
							if typ, ok := field.Type.(*ast.Ident); ok {
								a.viaType[typ.Name] = append(a.viaType[typ.Name], node)
							}
						}
					}
				case *ast.SelectorExpr:
					a.calls[fun.Sel.Name] = append(a.calls[fun.Sel.Name], node)
				}
			case *ast.AssignStmt:
				if len(node.Lhs) == len(node.Rhs) {
					for i, lhs := range node.Lhs {
						if ident, ok := lhs.(*ast.Ident); ok && ident.Obj != nil {
							a.assigned[ident.Obj] = append(a.assigned[ident.Obj], node.Rhs[i])
							if lit, ok := node.Rhs[i].(*ast.FuncLit); ok {
								a.recordParams(lit.Type, auditParam{local: ident.Obj})
							}
						}
					}
				}
			case *ast.ValueSpec:
				for i, name := range node.Names {
					if i < len(node.Values) && name.Obj != nil {
						a.assigned[name.Obj] = append(a.assigned[name.Obj], node.Values[i])
					}
				}
			}
			return true
		})
	}
	return a
}

func (a *auditAnalysis) recordParams(typ *ast.FuncType, owner auditParam) {
	index := 0
	for _, field := range typ.Params.List {
		for _, name := range field.Names {
			if name.Obj != nil {
				param := owner
				param.index = index
				a.params[name.Obj] = param
			}
			index++
		}
		if len(field.Names) == 0 {
			index++
		}
	}
}

// shapes evaluates a string expression to every value it can take, with
// auditWildcard for a part decided at run time. env binds the parameters of a
// helper being evaluated for one call.
func (a *auditAnalysis) shapes(expr ast.Expr, env map[*ast.Object][]string, depth int) []string {
	if depth > 12 {
		return []string{auditWildcard}
	}
	switch expr := expr.(type) {
	case *ast.BasicLit:
		if expr.Kind == token.STRING {
			value, err := strconv.Unquote(expr.Value)
			if err == nil {
				return []string{value}
			}
		}
		return []string{auditWildcard}
	case *ast.ParenExpr:
		return a.shapes(expr.X, env, depth+1)
	case *ast.BinaryExpr:
		if expr.Op == token.ADD {
			return a.concat(a.shapes(expr.X, env, depth+1), a.shapes(expr.Y, env, depth+1))
		}
	case *ast.Ident:
		if expr.Obj != nil {
			if bound, ok := env[expr.Obj]; ok {
				return bound
			}
			if expr.Obj.Kind == ast.Con {
				if spec, ok := expr.Obj.Decl.(*ast.ValueSpec); ok {
					for i, name := range spec.Names {
						if name.Name == expr.Name && i < len(spec.Values) {
							return a.shapes(spec.Values[i], nil, depth+1)
						}
					}
				}
			}
			if param, ok := a.params[expr.Obj]; ok {
				return a.paramShapes(param, depth+1)
			}
			if values := a.assigned[expr.Obj]; len(values) > 0 {
				var out []string
				for _, value := range values {
					out = append(out, a.shapes(value, env, depth+1)...)
				}
				return dedupeAuditShapes(out)
			}
			return []string{auditWildcard}
		}
		if value, ok := a.consts[expr.Name]; ok {
			return a.shapes(value, nil, depth+1)
		}
	case *ast.CallExpr:
		return a.callShapes(expr, env, depth)
	}
	return []string{auditWildcard}
}

func (a *auditAnalysis) callShapes(call *ast.CallExpr, env map[*ast.Object][]string, depth int) []string {
	name := ""
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		name = fun.Name
		if name == "string" && len(call.Args) == 1 {
			return a.shapes(call.Args[0], env, depth+1)
		}
	case *ast.SelectorExpr:
		if pkg, ok := fun.X.(*ast.Ident); ok && pkg.Name == "fmt" && fun.Sel.Name == "Sprintf" && len(call.Args) > 0 {
			return a.sprintf(call, env, depth)
		}
		if pkg, ok := fun.X.(*ast.Ident); ok && pkg.Obj == nil && pkg.Name != "p" {
			return []string{auditWildcard} // another package's function
		}
		name = fun.Sel.Name
	}
	decls := a.funcs[name]
	if len(decls) != 1 || decls[0].Type.Results == nil || len(decls[0].Type.Results.List) != 1 {
		return []string{auditWildcard}
	}
	decl := decls[0]
	if result, ok := decl.Type.Results.List[0].Type.(*ast.Ident); !ok || result.Name != "string" || decl.Body == nil {
		return []string{auditWildcard}
	}
	bound := map[*ast.Object][]string{}
	index := 0
	for _, field := range decl.Type.Params.List {
		for _, param := range field.Names {
			if index < len(call.Args) && param.Obj != nil {
				bound[param.Obj] = a.shapes(call.Args[index], env, depth+1)
			}
			index++
		}
	}
	var out []string
	ast.Inspect(decl.Body, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		if ret, ok := node.(*ast.ReturnStmt); ok && len(ret.Results) == 1 {
			out = append(out, a.shapes(ret.Results[0], bound, depth+1)...)
		}
		return true
	})
	if len(out) == 0 {
		return []string{auditWildcard}
	}
	return dedupeAuditShapes(out)
}

var auditFormatVerb = regexp.MustCompile(`%%|%[-+# 0-9.]*[a-zA-Z]`)

func (a *auditAnalysis) sprintf(call *ast.CallExpr, env map[*ast.Object][]string, depth int) []string {
	formats := a.shapes(call.Args[0], env, depth+1)
	var out []string
	for _, format := range formats {
		results := []string{""}
		arg := 1
		last := 0
		for _, match := range auditFormatVerb.FindAllStringIndex(format, -1) {
			results = a.concat(results, []string{format[last:match[0]]})
			verb := format[match[0]:match[1]]
			switch {
			case verb == "%%":
				results = a.concat(results, []string{"%"})
			case arg < len(call.Args) && strings.HasSuffix(verb, "s"):
				results = a.concat(results, a.shapes(call.Args[arg], env, depth+1))
				arg++
			default:
				results = a.concat(results, []string{auditWildcard})
				arg++
			}
			last = match[1]
		}
		out = append(out, a.concat(results, []string{format[last:]})...)
	}
	return dedupeAuditShapes(out)
}

// callsOf is every call of a function by name, including the calls of the
// service-operation writer through its function type.
func (a *auditAnalysis) callsOf(name string) []*ast.CallExpr {
	calls := a.calls[name]
	if name == "auditServiceOperation" {
		calls = append(calls[:len(calls):len(calls)], a.viaType["serviceOperationAuditWriter"]...)
	}
	return calls
}

// referencedParams is the parameters of decl that expr uses, directly or
// through a local variable.
func (a *auditAnalysis) referencedParams(expr ast.Expr, decl *ast.FuncDecl, seen map[*ast.Object]bool) map[*ast.Object]int {
	out := map[*ast.Object]int{}
	ast.Inspect(expr, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if !ok || ident.Obj == nil || seen[ident.Obj] {
			return true
		}
		seen[ident.Obj] = true
		if param, ok := a.params[ident.Obj]; ok && param.owner == decl.Name.Name {
			out[ident.Obj] = param.index
			return true
		}
		for _, value := range a.assigned[ident.Obj] {
			for obj, index := range a.referencedParams(value, decl, seen) {
				out[obj] = index
			}
		}
		return true
	})
	return out
}

// shapesIn evaluates expr inside decl once per call of decl, with the
// parameters it uses bound to that call's arguments together, so values that
// arrive together (an operation and its result) stay together.
func (a *auditAnalysis) shapesIn(expr ast.Expr, decl *ast.FuncDecl, depth int) []string {
	if decl == nil || depth > 10 {
		return a.shapes(expr, nil, depth+1)
	}
	used := a.referencedParams(expr, decl, map[*ast.Object]bool{})
	if len(used) == 0 {
		return a.shapes(expr, nil, depth+1)
	}
	key := decl.Name.Name
	if a.active["in:"+key] {
		return a.shapes(expr, nil, depth+1)
	}
	a.active["in:"+key] = true
	defer delete(a.active, "in:"+key)
	var out []string
	for _, call := range a.callsOf(decl.Name.Name) {
		env := map[*ast.Object][]string{}
		for obj, index := range used {
			if index < len(call.Args) {
				env[obj] = a.shapesIn(call.Args[index], a.encl[call], depth+1)
			}
		}
		out = append(out, a.shapes(expr, env, depth+1)...)
	}
	if len(out) == 0 {
		return a.shapes(expr, nil, depth+1)
	}
	return dedupeAuditShapes(out)
}

// paramShapes is every value a parameter receives at the calls of its function.
func (a *auditAnalysis) paramShapes(param auditParam, depth int) []string {
	var calls []*ast.CallExpr
	key := ""
	if param.owner != "" {
		key = param.owner + "#" + strconv.Itoa(param.index)
		calls = a.callsOf(param.owner)
	} else {
		key = param.local.Name + "@" + strconv.Itoa(param.index) + "@" + strconv.Itoa(int(param.local.Pos()))
		for _, call := range a.calls[param.local.Name] {
			if ident, ok := call.Fun.(*ast.Ident); ok && ident.Obj == param.local {
				calls = append(calls, call)
			}
		}
	}
	if cached, ok := a.memo[key]; ok {
		return cached
	}
	if a.active[key] {
		return []string{auditWildcard}
	}
	a.active[key] = true
	defer delete(a.active, key)
	var out []string
	for _, call := range calls {
		if param.index < len(call.Args) {
			out = append(out, a.shapesIn(call.Args[param.index], a.encl[call], depth+1)...)
		}
	}
	if len(out) == 0 {
		out = []string{auditWildcard}
	}
	out = dedupeAuditShapes(out)
	a.memo[key] = out
	return out
}

func (a *auditAnalysis) concat(left, right []string) []string {
	out := make([]string, 0, len(left)*len(right))
	for _, l := range left {
		for _, r := range right {
			value := l + r
			for strings.Contains(value, auditWildcard+auditWildcard) {
				value = strings.ReplaceAll(value, auditWildcard+auditWildcard, auditWildcard)
			}
			out = append(out, value)
		}
	}
	if len(out) > 4096 {
		a.t.Fatalf("an audit action expands to more than 4096 shapes")
	}
	return dedupeAuditShapes(out)
}

func dedupeAuditShapes(values []string) []string {
	seen := map[string]bool{}
	out := values[:0:0]
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

var auditInsertColumns = regexp.MustCompile(`(?s)INSERT INTO audit_logs\s*\(([^)]*)\)\s*(?:VALUES\s*\((.*?)\)|SELECT\s+(.*?)\s+(?:WHERE|FROM)\b)`)

// emittedAuditNames is every audit action name this package can write, with
// the file and line of each writer.
func emittedAuditNames(t *testing.T) (map[string][]string, int) {
	t.Helper()
	a := loadAuditAnalysis(t)
	names := map[string][]string{}
	writers := 0
	for _, decls := range a.funcs {
		for _, decl := range decls {
			if decl.Body == nil {
				continue
			}
			ast.Inspect(decl.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || len(call.Args) < 2 {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || (selector.Sel.Name != "ExecContext" && selector.Sel.Name != "Exec") {
					return true
				}
				sqlShapes := a.shapes(call.Args[1], nil, 0)
				if len(sqlShapes) != 1 || !strings.Contains(sqlShapes[0], "INSERT INTO audit_logs") {
					return true
				}
				match := auditInsertColumns.FindStringSubmatch(sqlShapes[0])
				if match == nil {
					t.Fatalf("%s: cannot read the audit_logs insert", decl.Name.Name)
				}
				columns := strings.Split(match[1], ",")
				values := match[2]
				if values == "" {
					values = match[3]
				}
				items := strings.Split(values, ",")
				if len(items) != len(columns) {
					t.Fatalf("%s: %d columns, %d values", decl.Name.Name, len(columns), len(items))
				}
				placeholder := 0
				argument := -1
				for i, column := range columns {
					isPlaceholder := strings.TrimSpace(items[i]) == "?"
					if strings.TrimSpace(column) == "action" {
						if !isPlaceholder {
							t.Fatalf("%s: the action is not a parameter", decl.Name.Name)
						}
						argument = 2 + placeholder
					}
					if isPlaceholder {
						placeholder++
					}
				}
				if argument < 0 || argument >= len(call.Args) {
					t.Fatalf("%s: no action argument", decl.Name.Name)
				}
				writers++
				for _, shape := range a.shapesIn(call.Args[argument], decl, 0) {
					// AUDIT_SHAPES=1 go test -run TestAuditActionNames -v prints
					// every derived action shape (for adding a new name).
					if os.Getenv("AUDIT_SHAPES") != "" {
						t.Logf("SHAPE %q", strings.ReplaceAll(shape, auditWildcard, "{}"))
					}
					name := auditActionName(shape)
					names[name] = append(names[name], decl.Name.Name)
				}
				return true
			})
		}
	}
	return names, writers
}

// auditActionName is the part of an action before its first ':' or space; the
// web reads the same part (lib/auditAction.ts).
func auditActionName(action string) string {
	if cut := strings.IndexAny(action, ": "); cut >= 0 {
		action = action[:cut]
	}
	return strings.ReplaceAll(action, auditWildcard, "{}")
}

func auditNamePattern(name string) *regexp.Regexp {
	parts := strings.Split(name, "{}")
	for i, part := range parts {
		parts[i] = regexp.QuoteMeta(part)
	}
	return regexp.MustCompile(`^` + strings.Join(parts, `[a-z0-9_-]+(?:\.[a-z0-9_-]+)*`) + `$`)
}

// auditUnreachableActionNames are names the analysis derives that no request
// can write, each with the reason; the analysis does not follow branches.
var auditUnreachableActionNames = map[string]string{
	// handleDomainSiteConfig sends action "" to the read handler, which writes
	// no audit row; only keep, take, recreate and apply reach the audit.
	"domain.site_config.": "site_config.go handleDomainSiteConfig: the read path",
}

func TestAuditActionNamesListEveryActionThePanelWrites(t *testing.T) {
	emitted, writers := emittedAuditNames(t)
	if writers < 6 {
		t.Fatalf("found %d audit_logs inserts; expected at least 6", writers)
	}
	listed := map[string]bool{}
	for _, name := range auditActionNames {
		if listed[name] {
			t.Errorf("%s is listed twice", name)
		}
		listed[name] = true
		if !regexp.MustCompile(`^[a-z0-9_]+(?:[.-][a-z0-9_]+)*$`).MatchString(name) {
			t.Errorf("%q is not an action name", name)
		}
	}
	shapes := make([]string, 0, len(emitted))
	for name := range emitted {
		shapes = append(shapes, name)
	}
	sort.Strings(shapes)
	used := map[string]bool{}
	for _, shape := range shapes {
		if _, ok := auditUnreachableActionNames[shape]; ok {
			continue
		}
		if shape == "{}" || strings.HasPrefix(shape, "{}") {
			t.Errorf("an audit action written by %v has no fixed name", emitted[shape])
			continue
		}
		pattern := auditNamePattern(shape)
		matched := false
		for _, name := range auditActionNames {
			if pattern.MatchString(name) {
				matched = true
				used[name] = true
			}
		}
		if !matched {
			t.Errorf("the Panel can write %q (via %v); add it to auditActionNames and give it EN/TR words in web/src/i18n", shape, emitted[shape])
		}
	}
	for _, name := range auditActionNames {
		if !used[name] {
			t.Errorf("auditActionNames lists %q, which no writer in this package produces", name)
		}
	}
	if testing.Verbose() {
		t.Logf("%d writers, %d shapes: %s", writers, len(shapes), strings.Join(shapes, " "))
	}
}

// A negative control: the analysis must see a name built from a parameter of
// a helper, a closure and a local variable, or the test above proves nothing.
func TestAuditActionAnalysisFollowsParametersClosuresAndLocals(t *testing.T) {
	emitted, _ := emittedAuditNames(t)
	for _, name := range []string{
		"backup.restore.failed",                       // auditBackupFailure("restore", …)
		"server.setup.child",                          // launchServiceOperation(… "server.setup.child:"+…)
		"service.install.recovered.failed",            // a local assigned in a var block
		"server.setup.mail_enrollment.retry_reviewed", // a tuple assignment
		"ssl.reissue",                                 // a local reassigned in a branch
		"dns.engine.switch.post_commit.pending",       // a Sprintf in a helper, two calls deep
		"system_sqlite.snapshot_download.completed",
		"store.catalog.update", // an inline Sprintf in the insert
	} {
		if _, ok := emitted[name]; !ok {
			t.Errorf("the analysis did not find %s", name)
		}
	}
}
