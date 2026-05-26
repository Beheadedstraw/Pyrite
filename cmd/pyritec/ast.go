package main

type pyriteProgram struct {
	Items []pyriteTopLevel
}

type pyriteTopLevel interface {
	topLevelNode()
}

type pyriteImportDecl struct {
	Name string
	Line int
}

func (*pyriteImportDecl) topLevelNode() {}

type pyriteBindingDecl struct {
	Name       string
	Type       string
	Value      string
	ValueExpr  pyriteExpr
	Const      bool
	Global     bool
	Line       int
	LineText   string
	LineIndent int
}

func (*pyriteBindingDecl) topLevelNode() {}

type pyriteParam struct {
	Name string
	Type string
}

type pyriteFunctionDecl struct {
	Name         string
	Params       []pyriteParam
	ReturnType   string
	NativeSymbol string
	Body         []pyriteStmt
	Line         int
	Indent       int
}

func (*pyriteFunctionDecl) topLevelNode() {}

type pyriteClassDecl struct {
	Name    string
	Methods []*pyriteFunctionDecl
	Line    int
	Indent  int
}

func (*pyriteClassDecl) topLevelNode() {}

type pyriteEnumDecl struct {
	Name    string
	Members []pyriteEnumMember
	Line    int
	Indent  int
}

func (*pyriteEnumDecl) topLevelNode() {}

type pyriteEnumMember struct {
	Name     string
	Value    string
	HasValue bool
	Line     int
}

type pyriteStmt interface {
	stmtNode()
	stmtBase() *pyriteStmtBase
}

type pyriteStmtBase struct {
	Line     int
	Indent   int
	Text     string
	Tokens   []pyriteToken
	Children []pyriteStmt
}

func (s *pyriteStmtBase) stmtBase() *pyriteStmtBase { return s }

type pyriteReturnStmt struct {
	pyriteStmtBase
	Value pyriteExpr
}

func (*pyriteReturnStmt) stmtNode() {}

type pyriteRaiseStmt struct {
	pyriteStmtBase
	Value pyriteExpr
}

func (*pyriteRaiseStmt) stmtNode() {}

type pyriteIfStmt struct {
	pyriteStmtBase
	Condition pyriteExpr
	Inline    pyriteStmt
}

func (*pyriteIfStmt) stmtNode() {}

type pyriteWhileStmt struct {
	pyriteStmtBase
	Condition pyriteExpr
	Inline    pyriteStmt
}

func (*pyriteWhileStmt) stmtNode() {}

type pyriteForStmt struct {
	pyriteStmtBase
	Target   string
	Iterable pyriteExpr
}

func (*pyriteForStmt) stmtNode() {}

type pyriteMatchStmt struct {
	pyriteStmtBase
	Value pyriteExpr
}

func (*pyriteMatchStmt) stmtNode() {}

type pyriteCaseStmt struct {
	pyriteStmtBase
	Pattern  pyriteExpr
	Wildcard bool
	Inline   pyriteStmt
}

func (*pyriteCaseStmt) stmtNode() {}

type pyriteControlStmt struct {
	pyriteStmtBase
	Kind string
}

func (*pyriteControlStmt) stmtNode() {}

type pyriteExceptStmt struct {
	pyriteStmtBase
	Name string
}

func (*pyriteExceptStmt) stmtNode() {}

type pyriteVarStmt struct {
	pyriteStmtBase
	Name  string
	Type  string
	Value pyriteExpr
}

func (*pyriteVarStmt) stmtNode() {}

type pyriteAssignStmt struct {
	pyriteStmtBase
	Target string
	Value  pyriteExpr
}

func (*pyriteAssignStmt) stmtNode() {}

type pyriteExprStmt struct {
	pyriteStmtBase
	Expr pyriteExpr
}

func (*pyriteExprStmt) stmtNode() {}

type pyriteExpr interface {
	exprNode()
}

type pyriteNameExpr struct {
	Name string
	Line int
}

func (*pyriteNameExpr) exprNode() {}

type pyriteLiteralExpr struct {
	Value string
	Kind  string
	Line  int
}

func (*pyriteLiteralExpr) exprNode() {}

type pyriteListExpr struct {
	Items []pyriteExpr
	Line  int
}

func (*pyriteListExpr) exprNode() {}

type pyriteObjectEntry struct {
	Key   pyriteExpr
	Value pyriteExpr
}

type pyriteObjectExpr struct {
	Entries []pyriteObjectEntry
	Line    int
}

func (*pyriteObjectExpr) exprNode() {}

type pyriteUnaryExpr struct {
	Op    string
	Right pyriteExpr
	Line  int
}

func (*pyriteUnaryExpr) exprNode() {}

type pyriteBinaryExpr struct {
	Left  pyriteExpr
	Op    string
	Right pyriteExpr
	Line  int
}

func (*pyriteBinaryExpr) exprNode() {}

type pyriteCallExpr struct {
	Callee pyriteExpr
	Args   []pyriteExpr
	Line   int
}

func (*pyriteCallExpr) exprNode() {}

type pyriteMemberExpr struct {
	Base  pyriteExpr
	Field string
	Line  int
}

func (*pyriteMemberExpr) exprNode() {}

type pyriteIndexExpr struct {
	Base  pyriteExpr
	Index pyriteExpr
	Line  int
}

func (*pyriteIndexExpr) exprNode() {}
