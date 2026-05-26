package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLexerEmitsIndentation(t *testing.T) {
	source := "def main():\n    if True:\n        print(\"ok\")\n    return 0\n"
	tokens, err := lexPyrite(source)
	if err != nil {
		t.Fatalf("lexPyrite failed: %v", err)
	}
	var indents, dedents int
	for _, tok := range tokens {
		if tok.Type == tokenIndent {
			indents++
		}
		if tok.Type == tokenDedent {
			dedents++
		}
	}
	if indents != 2 || dedents != 2 {
		t.Fatalf("indent/dedent mismatch: got %d/%d", indents, dedents)
	}
}

func TestParserUnderstandsCompilerHelperShapes(t *testing.T) {
	source := `
class Token:
    def __init__(self, kind: int, text: string):
        self.kind = kind
        self.text = text

enum TokenKind:
    IDENT
    NUMBER
    STRING = 10
    END = 10 + 2

def main():
    source: string = "name"
    token: Token = Token(TokenKind.IDENT, source)
    tokens: list[Token] = []
    tokens = tokens.push(token)
    print(tokens[0].text)
    match token.kind:
        case TokenKind.IDENT:
            print("ident")
        case _:
            print("other")
    return 0
`
	program, err := parsePyriteProgram(source)
	if err != nil {
		t.Fatalf("parsePyriteProgram failed: %v", err)
	}
	if len(program.Items) != 3 {
		t.Fatalf("expected 3 top-level items, got %d", len(program.Items))
	}
	cls, ok := program.Items[0].(*pyriteClassDecl)
	if !ok || cls.Name != "Token" || len(cls.Methods) != 1 {
		t.Fatalf("unexpected class item: %#v", program.Items[0])
	}
	enum, ok := program.Items[1].(*pyriteEnumDecl)
	if !ok || enum.Name != "TokenKind" || len(enum.Members) != 4 || enum.Members[2].Value != "10" || enum.Members[3].ValueExpr == nil {
		t.Fatalf("unexpected enum item: %#v", program.Items[1])
	}
	fn, ok := program.Items[2].(*pyriteFunctionDecl)
	if !ok || fn.Name != "main" || len(fn.Body) == 0 {
		t.Fatalf("unexpected function item: %#v", program.Items[2])
	}
}

func TestCompilerSupportsCompilerShapedClassContainers(t *testing.T) {
	source := `
class Token:
    def __init__(self, kind: int, text: string):
        self.kind = kind
        self.text = text

class TokenStream:
    def __init__(self, tokens: list[Token]):
        self.tokens = tokens
        self.current = None

    def first(self):
        self.current = self.tokens[0]
        return self.current

def main():
    tokens: list[Token] = []
    tokens = tokens.push(Token(1, "name"))
    stream: TokenStream = TokenStream(tokens)
    first: Token = stream.first()
    print(first.text)
    print(stream.tokens.len())
    return first.kind
`
	compiler := NewCompiler("test.pyr", "/tmp/test")
	if err := compiler.translate(source); err != nil {
		t.Fatalf("translate failed: %v", err)
	}
	body := compiler.body.String() + compiler.funcs.String()
	for _, want := range []string{
		"pyrite_list_any_push",
		"pyrite_list_any_box",
		"pyrite_any_as_list",
		"pyrite_any_as_class",
		"PYRITE_ANY_NONE",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in compiler-shaped class output, got:\n%s", want, body)
		}
	}
}

func TestParserAcceptsInlineIfStatements(t *testing.T) {
	source := `
def state_name(state: int):
    if state == 0: return "ready"
    if state == 1:
        return "running"
    return "unknown"
`
	if _, err := parsePyriteProgram(source); err != nil {
		t.Fatalf("parsePyriteProgram failed: %v", err)
	}
}

func TestParserAcceptsMultilineExpressions(t *testing.T) {
	source := `
def add(left: int, right: int):
    return left + right

def main():
    values: list[int] = [
        1,
        2,
        3,
    ]
    obj = {
        "name": "Ada",
        "kind": "compiler",
    }
    total: int = add(
        values[0],
        values[1],
    )
    print(obj.get("name"))
    return total
`
	program, err := parsePyriteProgram(source)
	if err != nil {
		t.Fatalf("parsePyriteProgram failed: %v", err)
	}
	mainFn := program.Items[1].(*pyriteFunctionDecl)
	if len(mainFn.Body) != 5 {
		t.Fatalf("expected 5 main statements, got %d: %#v", len(mainFn.Body), mainFn.Body)
	}
	values := mainFn.Body[0].(*pyriteVarStmt)
	list, ok := values.Value.(*pyriteListExpr)
	if !ok || len(list.Items) != 3 {
		t.Fatalf("expected multiline list literal, got %#v", values.Value)
	}
	callStmt := mainFn.Body[2].(*pyriteVarStmt)
	call, ok := callStmt.Value.(*pyriteCallExpr)
	if !ok || len(call.Args) != 2 {
		t.Fatalf("expected multiline call expression, got %#v", callStmt.Value)
	}
}

func TestParserBuildsStructuredStatements(t *testing.T) {
	source := `
def total(items: list[int]):
    out: int = 0
    for item in items:
        out = out + item
    if out > 10: return out
    return -1
`
	program, err := parsePyriteProgram(source)
	if err != nil {
		t.Fatalf("parsePyriteProgram failed: %v", err)
	}
	fn := program.Items[0].(*pyriteFunctionDecl)
	if _, ok := fn.Body[0].(*pyriteVarStmt); !ok {
		t.Fatalf("expected var statement, got %#v", fn.Body[0])
	}
	loop, ok := fn.Body[1].(*pyriteForStmt)
	if !ok || loop.Target != "item" || len(loop.Children) != 1 {
		t.Fatalf("expected for statement with one child, got %#v", fn.Body[1])
	}
	inlineIf, ok := fn.Body[2].(*pyriteIfStmt)
	if !ok || inlineIf.Inline == nil {
		t.Fatalf("expected inline if statement, got %#v", fn.Body[2])
	}
	if _, ok := inlineIf.Inline.(*pyriteReturnStmt); !ok {
		t.Fatalf("expected inline return, got %#v", inlineIf.Inline)
	}
	if _, ok := fn.Body[3].(*pyriteReturnStmt); !ok {
		t.Fatalf("expected return statement, got %#v", fn.Body[3])
	}
}

func TestParserAttachesBlockChildren(t *testing.T) {
	source := `
def main():
    value = 1
    if value == 1:
        return 1
    return 0
`
	program, err := parsePyriteProgram(source)
	if err != nil {
		t.Fatalf("parsePyriteProgram failed: %v", err)
	}
	fn := program.Items[0].(*pyriteFunctionDecl)
	ifBlock, ok := fn.Body[1].(*pyriteIfStmt)
	if !ok {
		t.Fatalf("expected if statement, got %#v", fn.Body[1])
	}
	if len(ifBlock.Children) != 1 {
		t.Fatalf("expected one child in if block, got %d: %#v", len(ifBlock.Children), ifBlock.Children)
	}
	if _, ok := ifBlock.Children[0].(*pyriteReturnStmt); !ok {
		t.Fatalf("expected return child, got %#v", ifBlock.Children[0])
	}
	if len(fn.Body) != 3 {
		t.Fatalf("expected return after if to remain sibling, got body len %d", len(fn.Body))
	}
}

func TestCompilerCollectsFromAST(t *testing.T) {
	source := `
enum Mode:
    OFF
    ON = 10
    AUTO = 10 + 2

def helper(value: int):
    return value

def main():
    print(helper(Mode.ON))
    return 0
`
	compiler := NewCompiler("test.pyr", "/tmp/test")
	if err := compiler.collectSource(source, ""); err != nil {
		t.Fatalf("collectSource failed: %v", err)
	}
	if compiler.enums["Mode"]["ON"] != 10 {
		t.Fatalf("expected enum value from AST collector, got %#v", compiler.enums["Mode"])
	}
	if compiler.enums["Mode"]["AUTO"] != 12 {
		t.Fatalf("expected enum expression value from AST collector, got %#v", compiler.enums["Mode"])
	}
	if compiler.functions["helper"] == nil || compiler.functions["main"] == nil {
		t.Fatalf("expected functions from AST collector, got %#v", compiler.functions)
	}
	if compiler.modules["<main>"] == nil || len(compiler.moduleOrder) != 1 {
		t.Fatalf("expected parsed main module to be retained, got modules=%#v order=%#v", compiler.modules, compiler.moduleOrder)
	}
	if _, ok := compiler.functions["helper"].astBody[0].(*pyriteReturnStmt); !ok {
		t.Fatalf("expected AST return statement, got %#v", compiler.functions["helper"].astBody[0])
	}
}

func TestCompilerInfersInlineReturnFromAST(t *testing.T) {
	source := `
def digit(ch: string):
    if ch == "0": return 0
    if ch == "1": return 1
`
	compiler := NewCompiler("test.pyr", "/tmp/test")
	if err := compiler.collectSource(source, ""); err != nil {
		t.Fatalf("collectSource failed: %v", err)
	}
	compiler.globalTypes = copyStringMap(compiler.types)
	fn := compiler.functions["digit"]
	if err := compiler.inferFunctionReturn(fn); err != nil {
		t.Fatalf("inferFunctionReturn failed: %v", err)
	}
	if fn.returnType != "int" {
		t.Fatalf("expected int return type, got %q", fn.returnType)
	}
}

func TestCompilerEmitsExpressionsFromAST(t *testing.T) {
	source := `
def plus(left: int, right: int):
    return left + right

def add(value: int):
    out: int = plus(value, 2 * 3)
    items: list[int] = [out, 4]
    if out == 7:
        return out
    return -1

def main():
    print(add(1))
    return 0
`
	compiler := NewCompiler("test.pyr", "/tmp/test")
	if err := compiler.translate(source); err != nil {
		t.Fatalf("translate failed: %v", err)
	}
	body := compiler.funcs.String()
	if !strings.Contains(body, "out = pyrite_fn_plus(value, (2 * 3));") {
		t.Fatalf("expected AST binary expression emission, got:\n%s", body)
	}
	if !strings.Contains(body, "items = pyrite_list_int_new((long[]){out, 4}, 2);") {
		t.Fatalf("expected AST list literal emission, got:\n%s", body)
	}
	if !strings.Contains(body, "if (out == 7) {") {
		t.Fatalf("expected AST condition emission, got:\n%s", body)
	}
	if !strings.Contains(body, "return (-1);") {
		t.Fatalf("expected AST unary return emission, got:\n%s", body)
	}
	if compiler.hir == nil || compiler.hir.Functions["add"] == nil {
		t.Fatalf("expected analyzed compiler to retain HIR, got %#v", compiler.hir)
	}
	addHIR := compiler.hir.Functions["add"]
	if addHIR.ReturnType != "int" {
		t.Fatalf("expected HIR return type int, got %q", addHIR.ReturnType)
	}
	if len(addHIR.Body) == 0 || addHIR.Body[0].Kind != "var" || addHIR.Body[0].Name != "out" {
		t.Fatalf("expected lowered var statement in HIR, got %#v", addHIR.Body)
	}
}

func TestCompilerEmitsMethodsAndIntrinsicsFromAST(t *testing.T) {
	source := `
def main():
    name: string = "Ada"
    trimmed = name.strip()
    lowered = name.lower()
    parsed = "42".to_int()
    digit = "9".is_digit()
    alpha = "Ada".is_alpha()
    alnum = "Ada9".is_alnum()
    space = "  ".is_space()
    numbers: list[int] = [1, 2]
    numbers = numbers.push(3)
    first = numbers.get(0)
    raw = bytes(numbers)
    again = raw.to_string()
    print(trimmed)
    print(first)
    print(again)
    return 0
`
	compiler := NewCompiler("test.pyr", "/tmp/test")
	if err := compiler.translate(source); err != nil {
		t.Fatalf("translate failed: %v", err)
	}
	body := compiler.body.String()
	for _, want := range []string{
		"pyrite_string_strip(name)",
		"pyrite_string_lower(name)",
		"pyrite_string_to_int(\"42\")",
		"pyrite_string_is_digit(\"9\")",
		"pyrite_string_is_alpha(\"Ada\")",
		"pyrite_string_is_alnum(\"Ada9\")",
		"pyrite_string_is_space(\"  \")",
		"pyrite_list_int_push(numbers, 3)",
		"pyrite_list_int_get(numbers, 0)",
		"pyrite_bytes_from_list(numbers)",
		"pyrite_bytes_to_string(raw)",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in AST emitted body, got:\n%s", want, body)
		}
	}
}

func TestCompilerEmitsControlStatementsFromAST(t *testing.T) {
	source := `
def make_values():
    return [1, 2]

def shout(value: int):
    print(value)

def main():
    total: int = 0
    for value in make_values():
        total = total + value
    foreach(make_values(), each):
        total = total + each
    match total:
        case 6:
            print("six")
        case _:
            print("other")
    lock = mux()
    routine(print(total), lock)
    async(shout(total), lock)
    if total == 3:
        print("ok")
    else:
        print("bad")
    try:
        raise "boom"
    except err:
        print(err)
    return total
`
	compiler := NewCompiler("test.pyr", "/tmp/test")
	if err := compiler.translate(source); err != nil {
		t.Fatalf("translate failed: %v", err)
	}
	body := compiler.body.String()
	for _, want := range []string{
		"__pyrite_foreach_",
		"for (size_t __i_value = 0;",
		"__pyrite_switch_",
		"if (__pyrite_switch_",
		"pyrite_routine_print_int(total, lock);",
		"pyrite_start_task(pyrite_routine_call_",
		"} else {",
		"goto __pyrite_except_",
		"char *err = __pyrite_error ? __pyrite_error : \"\";",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in AST emitted body, got:\n%s", want, body)
		}
	}
}

func TestCompilerClosesASTIfBlocks(t *testing.T) {
	source := `
def main():
    value = 1
    if value == 1:
        return 1
    return 0
`
	compiler := NewCompiler("test.pyr", "/tmp/test")
	if err := compiler.translate(source); err != nil {
		t.Fatalf("translate failed: %v", err)
	}
	body := compiler.body.String()
	if !strings.Contains(body, "if (value == 1) {") ||
		!strings.Contains(body, "return __pyrite_return;\n    }\n    int __pyrite_return = (int)(0);") {
		t.Fatalf("expected if block to close before following return, got:\n%s", body)
	}
}

func TestCompilerPipelineRejectsInvalidSemanticsBeforeEmission(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "bad assignment target",
			source: `
def main():
    (1 + 2) = 3
    return 0
`,
			want: "unsupported assignment target",
		},
		{
			name: "bad loop variable",
			source: `
def main():
    items: list[int] = [1]
    for self.value in items:
        print(1)
    return 0
`,
			want: "invalid loop variable",
		},
		{
			name: "bad local name",
			source: `
def main():
    self.value: int = 1
    return 0
`,
			want: "invalid variable name",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compiler := NewCompiler("test.pyr", "/tmp/test")
			err := compiler.translate(tc.source)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
			if compiler.body.Len() != 0 && !strings.Contains(compiler.body.String(), "int main(void)") {
				t.Fatalf("expected semantic error before full emission, got body:\n%s", compiler.body.String())
			}
		})
	}
}

func TestExpressionParserBuildsPrecedenceAndPostfixAST(t *testing.T) {
	expr, err := parsePyriteExpression(`tokens[0].text + source.slice(0, 2)`)
	if err != nil {
		t.Fatalf("parse expression failed: %v", err)
	}
	binary, ok := expr.(*pyriteBinaryExpr)
	if !ok || binary.Op != "+" {
		t.Fatalf("expected top-level + binary expression, got %#v", expr)
	}
	left, ok := binary.Left.(*pyriteMemberExpr)
	if !ok || left.Field != "text" {
		t.Fatalf("expected indexed member on left, got %#v", binary.Left)
	}
	if _, ok := left.Base.(*pyriteIndexExpr); !ok {
		t.Fatalf("expected member base to be an index expression, got %#v", left.Base)
	}
	right, ok := binary.Right.(*pyriteCallExpr)
	if !ok || len(right.Args) != 2 {
		t.Fatalf("expected method call with two args on right, got %#v", binary.Right)
	}
}

func TestExpressionParserRejectsMalformedExpression(t *testing.T) {
	_, err := parsePyriteExpression(`tokens[0.text`)
	if err == nil {
		t.Fatalf("expected malformed expression to fail")
	}
}

func TestParserAcceptsExamplesAndStdlib(t *testing.T) {
	roots := []string{"examples", "stdlib"}
	for _, root := range roots {
		matches, err := filepath.Glob(filepath.Join(root, "*.pyr"))
		if err != nil {
			t.Fatalf("glob failed: %v", err)
		}
		for _, path := range matches {
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if _, err := parsePyriteProgram(string(source)); err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
		}
	}
}
