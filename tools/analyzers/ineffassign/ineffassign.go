package ineffassign

import (
	"go/ast"
	"go/token"
)

type builder struct {
	roots     []*block
	block     *block
	vars      map[*ast.Object]*variable
	results   []*ast.FieldList
	breaks    branchStack
	continues branchStack
	gotos     branchStack
	labelStmt *ast.LabeledStmt
}

type block struct {
	children []*block
	ops      map[*ast.Object][]operation
}

func (b *block) addChild(c *block) {
	b.children = append(b.children, c)
}

type operation struct {
	id     *ast.Ident
	assign bool
}

type variable struct {
	fundept int
	escapes bool
}

func (bld *builder) walk(n ast.Node) {
	if n != nil {
		ast.Walk(bld, n)
	}
}

// Visit --
func (bld *builder) Visit(n ast.Node) ast.Visitor {
	switch n := n.(type) {
	case *ast.FuncDecl:
		bld.processFuncDel(n)
	case *ast.FuncLit:
		bld.processFuncLit(n)
	case *ast.IfStmt:
		bld.processIfStmt(n)
	case *ast.ForStmt:
		bld.processForStmt(n)
	case *ast.RangeStmt:
		bld.processRangeStmt(n)
	case *ast.SwitchStmt:
		bld.processSwitchStmt(n)
	case *ast.TypeSwitchStmt:
		bld.processTypeSwitchStmt(n)
	case *ast.SelectStmt:
		bld.processSelectStmt(n)
	case *ast.LabeledStmt:
		bld.processLabeledStmt(n)
	case *ast.BranchStmt:
		bld.processBranchStmt(n)
	case *ast.AssignStmt:
		bld.processAssignStmt(n)
	case *ast.GenDecl:
		bld.processGenDecl(n)
	case *ast.IncDecStmt:
		bld.processIncDecStmt(n)
	case *ast.Ident:
		bld.processIdent(n)
	case *ast.ReturnStmt:
		bld.processReturnStmt(n)
	case *ast.SendStmt:
		bld.maybePanic()
		return bld
	case *ast.BinaryExpr:
		bld.processBinaryExpr(n)
		return bld
	case *ast.CallExpr:
		bld.maybePanic()
		return bld
	case *ast.IndexExpr:
		bld.maybePanic()
		return bld
	case *ast.UnaryExpr:
		bld.processUnaryExpr(n)
		return bld
	case *ast.SelectorExpr:
		bld.processSelectorExpr(n)
		return bld
	case *ast.SliceExpr:
		bld.processSliceExpr(n)
		return bld
	case *ast.StarExpr:
		bld.maybePanic()
		return bld
	case *ast.TypeAssertExpr:
		bld.maybePanic()
		return bld

	default:
		return bld
	}
	return nil
}

func (bld *builder) processFuncDel(n *ast.FuncDecl) {
	if n.Body != nil {
		bld.fun(n.Type, n.Body)
	}
}

func (bld *builder) processFuncLit(n *ast.FuncLit) {
	bld.fun(n.Type, n.Body)
}

func (bld *builder) processIfStmt(n *ast.IfStmt) {
	bld.walk(n.Init)
	bld.walk(n.Cond)
	b0 := bld.block
	bld.newBlock(b0)
	bld.walk(n.Body)
	b1 := bld.block
	if n.Else != nil {
		bld.newBlock(b0)
		bld.walk(n.Else)
		b0 = bld.block
	}
	bld.newBlock(b0, b1)
}

func (bld *builder) processForStmt(n *ast.ForStmt) {
	lbl := bld.stmtLabel(n)
	brek := bld.breaks.push(lbl)
	continu := bld.continues.push(lbl)
	bld.walk(n.Init)
	start := bld.newBlock(bld.block)
	bld.walk(n.Cond)
	cond := bld.block
	bld.newBlock(cond)
	bld.walk(n.Body)
	continu.setDestination(bld.newBlock(bld.block))
	bld.walk(n.Post)
	bld.block.addChild(start)
	brek.setDestination(bld.newBlock(cond))
	bld.breaks.pop()
	bld.continues.pop()
}

func (bld *builder) processRangeStmt(n *ast.RangeStmt) {
	lbl := bld.stmtLabel(n)
	brek := bld.breaks.push(lbl)
	continu := bld.continues.push(lbl)
	bld.walk(n.X)
	pre := bld.newBlock(bld.block)
	start := bld.newBlock(pre)
	if n.Key != nil {
		lhs := []ast.Expr{n.Key}
		if n.Value != nil {
			lhs = append(lhs, n.Value)
		}
		bld.walk(&ast.AssignStmt{Lhs: lhs, Tok: n.Tok, TokPos: n.TokPos, Rhs: []ast.Expr{&ast.Ident{NamePos: n.X.End()}}})
	}
	bld.walk(n.Body)
	bld.block.addChild(start)
	continu.setDestination(pre)
	brek.setDestination(bld.newBlock(pre, bld.block))
	bld.breaks.pop()
	bld.continues.pop()
}

func (bld *builder) processSwitchStmt(n *ast.SwitchStmt) {
	bld.walk(n.Init)
	bld.walk(n.Tag)
	bld.swtch(n, n.Body.List)
}

func (bld *builder) processTypeSwitchStmt(n *ast.TypeSwitchStmt) {
	bld.walk(n.Init)
	bld.walk(n.Assign)
	bld.swtch(n, n.Body.List)
}
func (bld *builder) processSelectStmt(n *ast.SelectStmt) {
	brek := bld.breaks.push(bld.stmtLabel(n))
	for _, c := range n.Body.List {
		c := c.(*ast.CommClause).Comm
		if s, ok := c.(*ast.AssignStmt); ok {
			bld.walk(s.Rhs[0])
		} else {
			bld.walk(c)
		}
	}
	b0 := bld.block
	exits := make([]*block, len(n.Body.List))
	dfault := false
	for i, c := range n.Body.List {
		c, ok := c.(*ast.CommClause)
		if !ok {
			continue
		}
		bld.newBlock(b0)
		bld.walk(c)
		exits[i] = bld.block
		dfault = dfault || c.Comm == nil
	}
	if !dfault {
		exits = append(exits, b0)
	}
	brek.setDestination(bld.newBlock(exits...))
	bld.breaks.pop()
}
func (bld *builder) processLabeledStmt(n *ast.LabeledStmt) {
	bld.gotos.get(n.Label).setDestination(bld.newBlock(bld.block))
	bld.labelStmt = n
	bld.walk(n.Stmt)
}
func (bld *builder) processBranchStmt(n *ast.BranchStmt) {
	switch n.Tok {
	case token.BREAK:
		bld.breaks.get(n.Label).addSource(bld.block)
		bld.newBlock()
	case token.CONTINUE:
		bld.continues.get(n.Label).addSource(bld.block)
		bld.newBlock()
	case token.GOTO:
		bld.gotos.get(n.Label).addSource(bld.block)
		bld.newBlock()
	}
}
func (bld *builder) processAssignStmt(n *ast.AssignStmt) {
	if n.Tok == token.QUO_ASSIGN || n.Tok == token.REM_ASSIGN {
		bld.maybePanic()
	}

	for _, x := range n.Rhs {
		bld.walk(x)
	}
	for i, x := range n.Lhs {
		if id, ok := ident(x); ok {
			if n.Tok >= token.ADD_ASSIGN && n.Tok <= token.AND_NOT_ASSIGN {
				bld.use(id)
			}
			// Don't treat explicit initialization to zero as assignment; it is often used as shorthand for a bare declaration.
			if n.Tok == token.DEFINE && i < len(n.Rhs) && isZeroInitializer(n.Rhs[i]) {
				bld.use(id)
			} else {
				bld.assign(id)
			}
		} else {
			bld.walk(x)
		}
	}
}
func (bld *builder) processGenDecl(n *ast.GenDecl) {
	if n.Tok == token.VAR {
		for _, s := range n.Specs {
			s, ok := s.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, x := range s.Values {
				bld.walk(x)
			}
			for _, id := range s.Names {
				if len(s.Values) > 0 {
					bld.assign(id)
				} else {
					bld.use(id)
				}
			}
		}
	}
}
func (bld *builder) processIncDecStmt(n *ast.IncDecStmt) {
	if id, ok := ident(n.X); ok {
		bld.use(id)
		bld.assign(id)
	} else {
		bld.walk(n.X)
	}
}

func (bld *builder) processIdent(n *ast.Ident) {
	bld.use(n)
}

func (bld *builder) processReturnStmt(n *ast.ReturnStmt) {
	for _, x := range n.Results {
		bld.walk(x)
	}
	if res := bld.results[len(bld.results)-1]; res != nil {
		for _, f := range res.List {
			for _, id := range f.Names {
				if n.Results != nil {
					bld.assign(id)
				}
				bld.use(id)
			}
		}
	}
	bld.newBlock()
}

func (bld *builder) processBinaryExpr(n *ast.BinaryExpr) {
	if n.Op == token.EQL || n.Op == token.QUO || n.Op == token.REM {
		bld.maybePanic()
	}
}

func (bld *builder) processUnaryExpr(n *ast.UnaryExpr) {
	id, ok := ident(n.X)
	if ix, isIx := n.X.(*ast.IndexExpr); isIx {
		// We don't care about indexing into slices, but without type information we can do no better.
		id, ok = ident(ix.X)
	}
	if ok && n.Op == token.AND {
		if v, ok := bld.vars[id.Obj]; ok {
			v.escapes = true
		}
	}
}

func (bld *builder) processSelectorExpr(n *ast.SelectorExpr) {
	bld.maybePanic()
	// A method call (possibly delayed via a method value) might implicitly take
	// the address of its receiver, causing it to escape.
	// We can't do any better here without knowing the variable's type.
	if id, ok := ident(n.X); ok {
		if v, ok := bld.vars[id.Obj]; ok {
			v.escapes = true
		}
	}
}

func (bld *builder) processSliceExpr(n *ast.SliceExpr) {
	bld.maybePanic()
	// We don't care about slicing into slices, but without type information we can do no better.
	if id, ok := ident(n.X); ok {
		if v, ok := bld.vars[id.Obj]; ok {
			v.escapes = true
		}
	}
}

func isZeroInitializer(x ast.Expr) bool {
	// Assume that a call expression of a single argument is a conversion expression.  We can't do better without type information.
	if c, ok := x.(*ast.CallExpr); ok {
		switch c.Fun.(type) {
		case *ast.Ident, *ast.SelectorExpr:
		default:
			return false
		}
		if len(c.Args) != 1 {
			return false
		}
		x = c.Args[0]
	}

	switch x := x.(type) {
	case *ast.BasicLit:
		switch x.Value {
		case "0", "0.0", "0.", ".0", `""`:
			return true
		}
	case *ast.Ident:
		return x.Name == "false" && x.Obj == nil
	}

	return false
}

func (bld *builder) fun(typ *ast.FuncType, body *ast.BlockStmt) {
	for _, v := range bld.vars {
		v.fundept++
	}
	bld.results = append(bld.results, typ.Results)

	b := bld.block
	bld.newBlock()
	bld.roots = append(bld.roots, bld.block)
	bld.walk(typ)
	bld.walk(body)
	bld.block = b

	bld.results = bld.results[:len(bld.results)-1]
	for _, v := range bld.vars {
		v.fundept--
	}
}

func (bld *builder) swtch(stmt ast.Stmt, cases []ast.Stmt) {
	brek := bld.breaks.push(bld.stmtLabel(stmt))
	b0 := bld.block
	list := b0
	exits := make([]*block, 0, len(cases)+1)
	var dfault, fallthru *block
	for _, c := range cases {
		c, ok := c.(*ast.CaseClause)
		if !ok {
			continue
		}
		if c.List != nil {
			list = bld.newBlock(list)
			for _, x := range c.List {
				bld.walk(x)
			}
		}

		var parents []*block
		if c.List != nil {
			parents = append(parents, list)
		}
		if fallthru != nil {
			parents = append(parents, fallthru)
			fallthru = nil
		}
		bld.newBlock(parents...)
		if c.List == nil {
			dfault = bld.block
		}
		for _, s := range c.Body {
			bld.walk(s)
			if s, ok := s.(*ast.BranchStmt); ok && s.Tok == token.FALLTHROUGH {
				fallthru = bld.block
			}
		}

		if fallthru == nil {
			exits = append(exits, bld.block)
		}
	}
	if dfault != nil {
		list.addChild(dfault)
	} else {
		exits = append(exits, b0)
	}
	brek.setDestination(bld.newBlock(exits...))
	bld.breaks.pop()
}

// An operation that might panic marks named function results as used.
func (bld *builder) maybePanic() {
	if len(bld.results) == 0 {
		return
	}
	res := bld.results[len(bld.results)-1]
	if res == nil {
		return
	}
	for _, f := range res.List {
		for _, id := range f.Names {
			bld.use(id)
		}
	}
}

func (bld *builder) newBlock(parents ...*block) *block {
	bld.block = &block{ops: map[*ast.Object][]operation{}}
	for _, b := range parents {
		b.addChild(bld.block)
	}
	return bld.block
}

func (bld *builder) stmtLabel(s ast.Stmt) *ast.Object {
	if ls := bld.labelStmt; ls != nil && ls.Stmt == s {
		return ls.Label.Obj
	}
	return nil
}

func (bld *builder) assign(id *ast.Ident) {
	bld.newOp(id, true)
}

func (bld *builder) use(id *ast.Ident) {
	bld.newOp(id, false)
}

func (bld *builder) newOp(id *ast.Ident, assign bool) {
	if id.Name == "_" || id.Obj == nil {
		return
	}

	v, ok := bld.vars[id.Obj]
	if !ok {
		v = &variable{}
		bld.vars[id.Obj] = v
	}
	v.escapes = v.escapes || v.fundept > 0 || bld.block == nil

	if b := bld.block; b != nil {
		b.ops[id.Obj] = append(b.ops[id.Obj], operation{id, assign})
	}
}

type branchStack []*branch

type branch struct {
	label *ast.Object
	srcs  []*block
	dst   *block
}

func (s *branchStack) push(lbl *ast.Object) *branch {
	br := &branch{label: lbl}
	*s = append(*s, br)
	return br
}

func (s *branchStack) get(lbl *ast.Ident) *branch {
	for i := len(*s) - 1; i >= 0; i-- {
		if br := (*s)[i]; lbl == nil || br.label == lbl.Obj {
			return br
		}
	}

	// Guard against invalid code (break/continue outside of loop).
	if lbl == nil {
		return &branch{}
	}

	return s.push(lbl.Obj)
}

func (br *branch) addSource(src *block) {
	br.srcs = append(br.srcs, src)
	if br.dst != nil {
		src.addChild(br.dst)
	}
}

func (br *branch) setDestination(dst *block) {
	br.dst = dst
	for _, src := range br.srcs {
		src.addChild(dst)
	}
}

func (s *branchStack) pop() {
	*s = (*s)[:len(*s)-1]
}

func ident(x ast.Expr) (*ast.Ident, bool) {
	if p, ok := x.(*ast.ParenExpr); ok {
		return ident(p.X)
	}
	id, ok := x.(*ast.Ident)
	return id, ok
}

type checker struct {
	vars  map[*ast.Object]*variable
	seen  map[*block]bool
	ineff idents
}

func (chk *checker) check(b *block) {
	if chk.seen[b] {
		return
	}
	chk.seen[b] = true

	for obj, ops := range b.ops {
		if chk.vars[obj].escapes {
			continue
		}
	ops:
		for i, op := range ops {
			if !op.assign {
				continue
			}
			if i+1 < len(ops) {
				if ops[i+1].assign {
					chk.ineff = append(chk.ineff, op.id)
				}
				continue
			}
			seen := map[*block]bool{}
			for _, b := range b.children {
				if used(obj, b, seen) {
					continue ops
				}
			}
			chk.ineff = append(chk.ineff, op.id)
		}
	}

	for _, b := range b.children {
		chk.check(b)
	}
}

func used(obj *ast.Object, b *block, seen map[*block]bool) bool {
	if seen[b] {
		return false
	}
	seen[b] = true

	if ops := b.ops[obj]; len(ops) > 0 {
		return !ops[0].assign
	}
	for _, b := range b.children {
		if used(obj, b, seen) {
			return true
		}
	}
	return false
}

type idents []*ast.Ident

// Len --
func (ids idents) Len() int { return len(ids) }

// Less --
func (ids idents) Less(i, j int) bool { return ids[i].Pos() < ids[j].Pos() }

// Swap --
func (ids idents) Swap(i, j int) { ids[i], ids[j] = ids[j], ids[i] }
