package main

// This is recursice-descendent parser which constructs abstract
// syntax tree from input tokens.
//
// This parser knows only about BNF of the C grammer and doesn't care
// about its semantics. Therefore, some invalid expressions, such as
// `1+2=3`, are accepted by this parser, but that's intentional.
// Semantic errors are detected in a later pass.

var (
	pos       = 0
	int_ty    = Type{ty: INT, size: 4, align: 4}
	null_stmt = Node{op: ND_NULL}
	penv      *PEnv
)

const (
	ND_NUM       = iota + 256 // Number literal
	ND_STR                    // String literal
	ND_IDENT                  // Identigier
	ND_STRUCT                 // Struct
	ND_VARDEF                 // Variable definition
	ND_LVAR                   // Local variable reference
	ND_GVAR                   // Global variable reference
	ND_IF                     // "if"
	ND_FOR                    // "for"
	ND_DO_WHILE               // do ~ while
	ND_ADDR                   // address-of operator ("&")
	ND_DEREF                  // pointer dereference ("*")
	ND_DOT                    // Struct member access
	ND_EQ                     // ==
	ND_NE                     // !=
	ND_LOGOR                  // ||
	ND_LOGAND                 // &&
	ND_RETURN                 // "return"
	ND_SIZEOF                 // "sizeof"
	ND_ALIGNOF                // "_Alignof"
	ND_CALL                   // Function call
	ND_FUNC                   // Function definition
	ND_COMP_STMT              // Compound statement
	ND_EXPR_STMT              // Expressions statement
	ND_STMT_EXPR              // Statement expression (GUN extn.)
	ND_NULL                   // Null statement
)

const (
	INT = iota
	CHAR
	PTR
	ARY
	STRUCT
)

type Node struct {
	op    int     // Node type
	ty    *Type   // C type
	lhs   *Node   // left-hand side
	rhs   *Node   // right-hand side
	val   int     // Number literal
	expr  *Node   // "return" or expression stmt
	stmts *Vector // Compound statement

	name string // Identifier

	// Global variable
	is_extern bool
	data      string
	len       int

	// "if" ( cond ) then "else" els
	// "for" ( init; cond; inc ) body
	cond *Node
	then *Node
	els  *Node
	init *Node
	body *Node
	inc  *Node

	// Function definition
	stacksize int
	globals   *Vector

	// Offset from BP or beginning of a struct
	offset int

	// Function call
	args *Vector
}

type Type struct {
	ty    int
	size  int
	align int

	// Pointer
	ptr_to *Type

	// Array
	ary_of *Type
	len    int

	// Struct
	members *Vector
	offset  int
}

type PEnv struct {
	tags *Map
	next *PEnv
}

func new_penv(next *PEnv) *PEnv {
	env := new(PEnv)
	env.tags = new_map()
	env.next = next
	return env
}

func expect(ty int) {
	t := tokens.data[pos].(*Token)
	if t.ty != ty {
		error("%c (%d) expected, but got %c (%d)", ty, ty, t.ty, t.ty)
	}
	pos++
}

func new_prim_ty(ty, size int) *Type {
	ret := new(Type)
	ret.ty = ty
	ret.size = size
	ret.align = size
	return ret
}

func char_tyf() *Type { return new_prim_ty(CHAR, 1) }
func int_tyf() *Type  { return new_prim_ty(INT, 4) }

func consume(ty int) bool {
	t := tokens.data[pos].(*Token)
	if t.ty != ty {
		return false
	}
	pos++
	return true
}

func is_typename() bool {
	t := tokens.data[pos].(*Token)
	return t.ty == TK_INT || t.ty == TK_CHAR || t.ty == TK_STRUCT
}

func read_type() *Type {
	t := tokens.data[pos].(*Token)

	if t.ty == TK_INT {
		pos++
		return int_tyf()
	}

	if t.ty == TK_CHAR {
		pos++
		return char_tyf()
	}

	if t.ty == TK_STRUCT {
		pos++

		var tag string
		t := tokens.data[pos].(*Token)
		if t.ty == TK_IDENT {
			pos++
			tag = t.name
		}

		var members *Vector
		if consume('{') {
			members = new_vec()
			for !consume('}') {
				vec_push(members, decl())
			}
		}

		if tag == "" && members == nil {
			error("bad struct definition")
		}

		if tag != "" && members != nil {
			map_put(penv.tags, tag, members)
		} else if tag != "" && members == nil {
			members = map_get(penv.tags, tag).(*Vector)
			if members == nil {
				error("incomplete type: %s", tag)
			}
		}

		return struct_of(members)
	}

	return nil
}

func new_binop(op int, lhs, rhs *Node) *Node {
	node := new(Node)
	node.op = op
	node.lhs = lhs
	node.rhs = rhs
	return node
}

func new_expr(op int, expr *Node) *Node {
	node := new(Node)
	node.op = op
	node.expr = expr
	return node
}

func ident() string {
	t := tokens.data[pos].(*Token)
	pos++
	if t.ty != TK_IDENT {
		error("identifier expected, but got %s", t.input)
	}
	return t.name
}

func primary() *Node {
	t := tokens.data[pos].(*Token)
	pos++

	if t.ty == '(' {
		if consume('{') {
			node := new(Node)
			node.op = ND_STMT_EXPR
			node.body = compound_stmt()
			expect(')')
			return node
		}
		node := assign()
		expect(')')
		return node
	}

	node := new(Node)
	if t.ty == TK_NUM {
		node.ty = int_tyf()
		node.op = ND_NUM
		node.val = t.val
		return node
	}

	if t.ty == TK_STR {
		node.ty = ary_of(char_tyf(), len(t.str))
		node.op = ND_STR
		node.data = t.str
		node.len = t.len
		return node
	}

	if t.ty == TK_IDENT {
		node.name = t.name

		if !consume('(') {
			node.op = ND_IDENT
			return node
		}

		node.op = ND_CALL
		node.args = new_vec()
		if consume(')') {
			return node
		}

		vec_push(node.args, assign())
		for consume(',') {
			vec_push(node.args, assign())
		}
		expect(')')
		return node
	}

	error("number expected, but got %s", t.input)
	return nil
}

func postfix() *Node {
	lhs := primary()

	for {
		if consume('.') {
			node := new(Node)
			node.op = ND_DOT
			node.expr = lhs
			node.name = ident()
			lhs = node
			continue
		}

		if consume(TK_ARROW) {
			node := new(Node)
			node.op = ND_DOT
			node.expr = new_expr(ND_DEREF, lhs)
			node.name = ident()
			lhs = node
			continue
		}

		if consume('[') {
			lhs = new_expr(ND_DEREF, new_binop('+', lhs, assign()))
			expect(']')
			continue
		}
		return lhs
	}
	return nil
}

func unary() *Node {
	if consume('*') {
		return new_expr(ND_DEREF, mul())
	}
	if consume('&') {
		return new_expr(ND_ADDR, mul())
	}
	if consume(TK_SIZEOF) {
		return new_expr(ND_SIZEOF, unary())
	}
	if consume(TK_ALIGNOF) {
		return new_expr(ND_ALIGNOF, unary())
	}
	return postfix()
}

func mul() *Node {
	lhs := unary()
	for {
		t := tokens.data[pos].(*Token)
		if t.ty != '*' && t.ty != '/' {
			return lhs
		}
		pos++
		lhs = new_binop(t.ty, lhs, unary())
	}
	return lhs
}

func read_array(ty *Type) *Type {
	v := new_vec()
	for consume('[') {
		l := primary()
		if l.op != ND_NUM {
			error("number expected")
		}
		vec_push(v, l)
		expect(']')
	}
	for i := v.len - 1; i >= 0; i-- {
		l := v.data[i].(*Node)
		ty = ary_of(ty, l.val)
	}
	return ty
}

func parse_add() *Node {

	lhs := mul()
	for {
		t := tokens.data[pos].(*Token)
		if t.ty != '+' && t.ty != '-' {
			return lhs
		}
		pos++
		lhs = new_binop(t.ty, lhs, mul())
	}
	return lhs
}

func rel() *Node {
	lhs := parse_add()
	for {
		t := tokens.data[pos].(*Token)
		if t.ty == '<' {
			pos++
			lhs = new_binop('<', lhs, parse_add())
			continue
		}
		if t.ty == '>' {
			pos++
			lhs = new_binop('<', parse_add(), lhs)
			continue
		}
		return lhs
	}
}

func equality() *Node {
	lhs := rel()
	for {
		t := tokens.data[pos].(*Token)
		if t.ty == TK_EQ {
			pos++
			lhs = new_binop(ND_EQ, lhs, rel())
			continue
		}
		if t.ty == TK_NE {
			pos++
			lhs = new_binop(ND_NE, lhs, rel())
			continue
		}
		return lhs
	}
}

func logand() *Node {
	lhs := equality()
	for {
		t := tokens.data[pos].(*Token)
		if t.ty != TK_LOGAND {
			return lhs
		}
		pos++
		lhs = new_binop(ND_LOGAND, lhs, equality())
	}
	return lhs
}

func logor() *Node {
	lhs := logand()
	for {
		t := tokens.data[pos].(*Token)
		if t.ty != TK_LOGOR {
			return lhs
		}
		pos++
		lhs = new_binop(ND_LOGOR, lhs, logand())
	}
	return lhs
}

func assign() *Node {
	lhs := logor()
	if consume('=') {
		return new_binop('=', lhs, logor())
	}
	return lhs
}

func ttype() *Type {
	t := tokens.data[pos].(*Token)
	ty := read_type()
	if ty == nil {
		error("typename expected, but got %s", t.input)
	}
	for consume('*') {
		ty = ptr_to(ty)
	}
	return ty
}

func decl() *Node {
	node := new(Node)
	node.op = ND_VARDEF

	// Read the first half of type name (e.g. `int *`).
	node.ty = ttype()

	// Read an identifier.
	node.name = ident()

	// Read the second half of type name (e.g. `[3][5]`).
	node.ty = read_array(node.ty)

	// Read an initializer.
	if consume('=') {
		node.init = assign()
	}
	expect(';')
	return node
}

func param() *Node {
	node := new(Node)
	node.op = ND_VARDEF
	node.ty = ttype()
	node.name = ident()
	return node
}

func expr_stmt() *Node {
	node := new_expr(ND_EXPR_STMT, assign())
	expect(';')
	return node
}

func stmt() *Node {
	node := new(Node)
	t := tokens.data[pos].(*Token)

	switch t.ty {
	case TK_INT, TK_CHAR, TK_STRUCT:
		return decl()
	case TK_IF:
		pos++
		node.op = ND_IF
		expect('(')
		node.cond = assign()
		expect(')')

		node.then = stmt()

		if consume(TK_ELSE) {
			node.els = stmt()
		}
		return node
	case TK_FOR:
		pos++
		node.op = ND_FOR
		expect('(')
		if is_typename() {
			node.init = decl()
		} else {
			node.init = expr_stmt()
		}
		node.cond = assign()
		expect(';')
		node.inc = new_expr(ND_EXPR_STMT, assign())
		expect(')')
		node.body = stmt()
		return node
	case TK_WHILE:
		pos++
		node.op = ND_FOR
		node.init = &null_stmt
		node.inc = &null_stmt
		expect('(')
		node.cond = assign()
		expect(')')
		node.body = stmt()
		return node
	case TK_DO:
		pos++
		node.op = ND_DO_WHILE
		node.body = stmt()
		expect(TK_WHILE)
		expect('(')
		node.cond = assign()
		expect(')')
		expect(';')
		return node
	case TK_RETURN:
		pos++
		node.op = ND_RETURN
		node.expr = assign()
		expect(';')
		return node
	case '{':
		pos++
		node.op = ND_COMP_STMT
		node.stmts = new_vec()
		for !consume('}') {
			vec_push(node.stmts, stmt())
		}
		return node
	case ';':
		pos++
		return &null_stmt
	default:
		return expr_stmt()
	}
	return nil
}

func compound_stmt() *Node {

	node := new(Node)
	node.op = ND_COMP_STMT
	node.stmts = new_vec()

	penv = new_penv(penv)
	for !consume('}') {
		vec_push(node.stmts, stmt())
	}
	penv = penv.next
	return node
}

func toplevel() *Node {
	is_extern := consume(TK_EXTERN)
	ty := ttype()
	if ty == nil {
		t := tokens.data[pos].(*Token)
		error("typename expected, but got %s", t.input)
	}

	name := ident()

	// Function
	if consume('(') {
		node := new(Node)
		node.op = ND_FUNC
		node.ty = ty
		node.name = name
		node.args = new_vec()

		if !consume(')') {
			vec_push(node.args, param())
			for consume(',') {
				vec_push(node.args, param())
			}
			expect(')')
		}

		expect('{')
		node.body = compound_stmt()
		return node
	}

	// Global variable
	node := new(Node)
	node.op = ND_VARDEF
	node.ty = read_array(ty)
	node.name = name

	if is_extern {
		node.is_extern = true
	} else {
		node.data = ""
		node.len = node.ty.size
	}
	expect(';')
	return node
}

func parse(tokens_ *Vector) *Vector {
	tokens = tokens_
	pos = 0
	penv = new_penv(penv)

	v := new_vec()
	for (tokens.data[pos].(*Token)).ty != TK_EOF {
		vec_push(v, toplevel())
	}
	return v
}
