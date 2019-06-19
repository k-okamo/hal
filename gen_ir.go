package main

import (
	"fmt"
	"os"
)

var (
	code   *Vector
	nreg   int
	nlabel int
)

var irinfo = map[int]IRInfo{
	IR_ADD:         {name: "ADD", ty: IR_TY_REG_REG},
	IR_SUB:         {name: "SUB", ty: IR_TY_REG_REG},
	IR_MUL:         {name: "MUL", ty: IR_TY_REG_REG},
	IR_DIV:         {name: "DIV", ty: IR_TY_REG_REG},
	IR_IMM:         {name: "MOV", ty: IR_TY_REG_IMM},
	IR_SUB_IMM:     {name: "SUB", ty: IR_TY_REG_IMM},
	IR_MOV:         {name: "MOV", ty: IR_TY_REG_REG},
	IR_LABEL:       {name: "", ty: IR_TY_LABEL},
	IR_LT:          {name: "LT", ty: IR_TY_REG_REG},
	IR_JMP:         {name: "JMP", ty: IR_TY_JMP},
	IR_UNLESS:      {name: "UNLESS", ty: IR_TY_REG_LABEL},
	IR_CALL:        {name: "CALL", ty: IR_TY_CALL},
	IR_RETURN:      {name: "RET", ty: IR_TY_REG},
	IR_LOAD32:      {name: "LOAD32", ty: IR_TY_REG_REG},
	IR_LOAD64:      {name: "LOAD64", ty: IR_TY_REG_REG},
	IR_STORE32:     {name: "STORE32", ty: IR_TY_REG_REG},
	IR_STORE64:     {name: "STORE64", ty: IR_TY_REG_REG},
	IR_STORE32_ARG: {name: "STORE32_ARG", ty: IR_TY_IMM_IMM},
	IR_STORE64_ARG: {name: "STORE64_ARG", ty: IR_TY_IMM_IMM},
	IR_KILL:        {name: "KILL", ty: IR_TY_REG},
	IR_NOP:         {name: "NOP", ty: IR_TY_NOARG},
	0:              {name: "", ty: 0},
}

const (
	IR_ADD = iota + 256
	IR_SUB
	IR_MUL
	IR_DIV
	IR_IMM
	IR_SUB_IMM
	IR_MOV
	IR_RETURN
	IR_CALL
	IR_LABEL
	IR_LT
	IR_JMP
	IR_UNLESS
	IR_LOAD32
	IR_LOAD64
	IR_STORE32
	IR_STORE64
	IR_STORE32_ARG
	IR_STORE64_ARG
	IR_KILL
	IR_NOP
)

const (
	IR_TY_NOARG = iota + 256
	IR_TY_REG
	IR_TY_IMM
	IR_TY_JMP
	IR_TY_LABEL
	IR_TY_REG_REG
	IR_TY_REG_IMM
	IR_TY_IMM_IMM
	IR_TY_REG_LABEL
	IR_TY_CALL
)

type IR struct {
	op  int
	lhs int
	rhs int

	// Function call
	name  string
	nargs int
	args  [6]int
}

type IRInfo struct {
	name string
	ty   int
}

type Function struct {
	name      string
	stacksize int
	ir        *Vector
}

func tostr(ir *IR) string {
	info := irinfo[ir.op]
	switch info.ty {
	case IR_TY_LABEL:
		return format(".L%d:", ir.lhs)
	case IR_TY_IMM:
		return format("\t%s %d", info.name, ir.lhs)
	case IR_TY_REG:
		return format("\t%s r%d", info.name, ir.lhs)
	case IR_TY_JMP:
		return format("\t%s r%d", info.name, ir.lhs)
	case IR_TY_REG_REG:
		return format("\t%s r%d, r%d", info.name, ir.lhs, ir.rhs)
	case IR_TY_REG_IMM:
		return format("\t%s r%d, %d", info.name, ir.lhs, ir.rhs)
	case IR_TY_IMM_IMM:
		return format("\t%s %d, %d", info.name, ir.lhs, ir.rhs)
	case IR_TY_REG_LABEL:
		return format("\t%s r%d, .L%d", info.name, ir.lhs, ir.rhs)
	case IR_TY_CALL:
		{
			sb := new_sb()
			sb_append(sb, format("r%d = %s(", ir.lhs, ir.name))
			for i := 0; i < ir.nargs; i++ {
				sb_append(sb, format(", r%d", ir.args))
			}
			sb_append(sb, ")\n")
			return sb_get(sb)
		}
	default:
		//asset(info.ty == IR_TY_NOARG)
		return format("\t%s", info.name)
	}
	return ""
}

func dump_ir(irv *Vector) {
	for i := 0; i < irv.len; i++ {
		fn := irv.data[i].(*Function)
		fmt.Fprintf(os.Stderr, "%s():\n", fn.name)
		for j := 0; j < fn.ir.len; j++ {
			fmt.Fprintf(os.Stderr, "%s\n", tostr(fn.ir.data[j].(*IR)))
		}
	}
}

func add(op, lhs, rhs int) *IR {
	ir := new(IR)
	ir.op = op
	ir.lhs = lhs
	ir.rhs = rhs
	vec_push(code, ir)
	return ir
}

func kill(r int) {
	add(IR_KILL, r, -1)
}

func label(x int) {
	add(IR_LABEL, x, -1)
}

func gen_lval(node *Node) int {
	if node.op == ND_DEREF {
		return gen_expr(node.expr)
	}

	if node.op == ND_LVAR {
		r := nreg
		nreg++
		add(IR_MOV, r, 0)
		add(IR_SUB_IMM, r, node.offset)
		return r
	}

	error("not an lvalue: %d (%s)", node.op, node.name)
	return -1
}

func gen_binop(ty int, lhs, rhs *Node) int {
	r1, r2 := gen_expr(lhs), gen_expr(rhs)
	add(ty, r1, r2)
	kill(r2)
	return r1
}

func gen_expr(node *Node) int {

	switch node.op {
	case ND_NUM:
		{
			r := nreg
			nreg++
			add(IR_IMM, r, node.val)
			return r
		}
	case ND_LOGAND:
		{
			x := nlabel
			nlabel++
			r1 := gen_expr(node.lhs)
			add(IR_UNLESS, r1, x)
			r2 := gen_expr(node.rhs)
			add(IR_MOV, r1, r2)
			kill(r2)
			add(IR_UNLESS, r1, x)
			add(IR_IMM, r1, 1)
			label(x)
			return r1
		}
	case ND_LOGOR:
		{
			x := nlabel
			nlabel++
			y := nlabel
			nlabel++
			r1 := gen_expr(node.lhs)
			add(IR_UNLESS, r1, x)
			add(IR_IMM, r1, 1)
			add(IR_JMP, y, -1)
			label(x)
			r2 := gen_expr(node.rhs)
			add(IR_MOV, r1, r2)
			kill(r2)
			add(IR_UNLESS, r1, y)
			add(IR_IMM, r1, 1)
			label(y)
			return r1
		}
	case ND_LVAR:
		{
			r := gen_lval(node)
			if node.ty.ty == PTR {
				add(IR_LOAD64, r, r)
			} else {
				add(IR_LOAD32, r, r)
			}
			return r
		}

	case ND_CALL:
		{
			var args [6]int
			for i := 0; i < node.args.len; i++ {
				args[i] = gen_expr(node.args.data[i].(*Node))
			}
			r := nreg
			nreg++

			ir := add(IR_CALL, r, -1)
			ir.name = node.name
			ir.nargs = node.args.len
			for i := 0; i < 6; i++ {
				ir.args[i] = args[i]
			}
			for i := 0; i < ir.nargs; i++ {
				kill(ir.args[i])
			}
			return r
		}
	case ND_ADDR:
		{
			return gen_lval(node.expr)
		}
	case ND_DEREF:
		{
			r := gen_expr(node.expr)
			add(IR_LOAD64, r, r)
			return r
		}
	case '=':
		{
			rhs, lhs := gen_expr(node.rhs), gen_lval(node.lhs)
			if node.lhs.ty.ty == PTR {
				add(IR_STORE64, lhs, rhs)
			} else {
				add(IR_STORE32, lhs, rhs)
			}
			kill(rhs)
			return lhs
		}
	case '+', '-':
		{
			insn := IR_SUB
			if node.op == '+' {
				insn = IR_ADD
			}
			if node.lhs.ty.ty != PTR {
				return gen_binop(insn, node.lhs, node.rhs)
			}

			rhs := gen_expr(node.rhs)
			r := nreg
			nreg++
			add(IR_IMM, r, size_of(node.lhs.ty.ptr_of))
			add(IR_MUL, rhs, r)
			kill(r)

			lhs := gen_expr(node.lhs)
			add(insn, lhs, rhs)
			kill(rhs)
			return lhs
		}
	case '*':
		return gen_binop(IR_MUL, node.lhs, node.rhs)
	case '/':
		return gen_binop(IR_DIV, node.lhs, node.rhs)
	case '<':
		return gen_binop(IR_LT, node.lhs, node.rhs)
	default:
		//assert(0 && "unknown AST type")
	}

	return 0
}

func gen_stmt(node *Node) {

	if node.op == ND_VARDEF {
		if node.init == nil {
			return
		}
		rhs := gen_expr(node.init)
		lhs := nreg
		nreg++
		add(IR_MOV, lhs, 0)
		add(IR_SUB_IMM, lhs, node.offset)
		if node.ty.ty == PTR {
			add(IR_STORE64, lhs, rhs)
		} else {
			add(IR_STORE32, lhs, rhs)
		}
		kill(lhs)
		kill(rhs)
		return
	}

	if node.op == ND_IF {

		if node.els != nil {
			x := nlabel
			nlabel++
			y := nlabel
			nlabel++
			r := gen_expr(node.cond)
			add(IR_UNLESS, r, x)
			kill(r)
			gen_stmt(node.then)
			add(IR_JMP, y, -1)
			label(x)
			gen_stmt(node.els)
			label(y)
		}
		x := nlabel
		nlabel++
		r := gen_expr(node.cond)
		add(IR_UNLESS, r, x)
		kill(r)
		gen_stmt(node.then)
		label(x)
		return
	}
	if node.op == ND_FOR {
		x := nlabel
		nlabel++
		y := nlabel
		nlabel++

		gen_stmt(node.init)
		add(IR_LABEL, x, -1)
		r := gen_expr(node.cond)
		add(IR_UNLESS, r, y)
		kill(r)
		gen_stmt(node.body)
		kill(gen_expr(node.inc))
		add(IR_JMP, x, -1)
		label(y)
		return
	}
	if node.op == ND_RETURN {
		r := gen_expr(node.expr)
		add(IR_RETURN, r, -1)
		kill(r)
		return
	}
	if node.op == ND_EXPR_STMT {
		kill(gen_expr(node.expr))
		return
	}
	if node.op == ND_COMP_STMT {
		for i := 0; i < node.stmts.len; i++ {
			gen_stmt((node.stmts.data[i]).(*Node))
		}
		return
	}
	error("unknown node: %d", node.op)
}

func gen_ir(nodes *Vector) *Vector {
	v := new_vec()

	for i := 0; i < nodes.len; i++ {
		node := nodes.data[i].(*Node)
		//assert(node.op == ND_FUNC)

		code = new_vec()
		nreg = 1

		//if nodes.len > 0 {
		//	add(IR_SAVE_ARGS, node.args.len, -1)
		//}
		for i := 0; i < node.args.len; i++ {
			arg := node.args.data[i].(*Node)
			op := IR_STORE32_ARG
			if arg.ty.ty == PTR {
				op = IR_STORE64_ARG
			}
			add(op, arg.offset, i)
		}

		gen_stmt(node.body)

		fn := new(Function)
		fn.name = node.name
		fn.stacksize = node.stacksize
		fn.ir = code
		vec_push(v, fn)
	}
	return v
}

// [Debug] intermediate reprensations
func print_irs(fns *Vector) {
	if !debug {
		return
	}
	fmt.Println("-- intermediate reprensetations --")
	for i := 0; i < fns.len; i++ {
		fn := fns.data[i].(*Function)
		for j := 0; j < fn.ir.len; j++ {
			ir := fn.ir.data[j].(*IR)
			op := ""
			switch ir.op {
			case IR_IMM:
				op = "IR_IMM      "
			case IR_SUB_IMM:
				op = "IR_SUB_IMM  "
			case IR_MOV:
				op = "IR_MOV      "
			case IR_RETURN:
				op = "IR_RETURN   "
			case IR_LABEL:
				op = "IR_LABEL    "
			case IR_LT:
				op = "IR_LT       "
			case IR_JMP:
				op = "IR_JMP      "
			case IR_UNLESS:
				op = "IR_UNLESS   "
			case IR_LOAD32:
				op = "IR_LOAD32   "
			case IR_LOAD64:
				op = "IR_LOAD64   "
			case IR_STORE32:
				op = "IR_STORE32  "
			case IR_STORE64:
				op = "IR_STORE64  "
			case IR_STORE32_ARG:
				op = "IR_STORE32_ARG  "
			case IR_STORE64_ARG:
				op = "IR_STORE64_ARG  "
			case IR_KILL:
				op = "IR_KILL     "
			case IR_NOP:
				op = "IR_NOP      "
			case IR_ADD:
				op = "IR_ADD      "
			case IR_SUB:
				op = "IR_SUB      "
			case IR_MUL:
				op = "IR_MUL      "
			case IR_DIV:
				op = "IR_DIV      "
			default:
				op = "            "
			}
			fmt.Printf("[%02d:%02d] op: %s, lhs: %d, rhs: %d\n", i, j, op, ir.lhs, ir.rhs)
		}
	}
	fmt.Println("")
}