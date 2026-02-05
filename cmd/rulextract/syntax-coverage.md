# Supported Construct Variants

Extracted from tests and examples.

## AssignStmt

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `expr[expr] = call()` | 29 | tests/lang-constructs/main.go:1138 | `m[0] = make([]int, 3)` |
| `expr[expr] = expr` | 9 | examples/mos6502/cmd/c64/main.go:428 | `c.Memory[dstAddr+col] = c.Memory[srcAddr+col]` |
| `expr[expr] = ident` | 27 | tests/lang-constructs/main.go:876 | `m[key] = value` |
| `expr[expr] = ident.field` | 13 | examples/mos6502/lib/cpu/cpu.go:537 | `c.Memory[int(addr)] = c.A` |
| `expr[expr] = ident{...}` | 1 | tests/lang-constructs/main.go:985 | `m["first"] = MapTestStruct{Name: "hello", Value: 42}` |
| `expr[expr] = int` | 55 | tests/lang-constructs/main.go:74 | `a[0] = 1` |
| `expr[expr] = string` | 4 | tests/lang-constructs/main.go:917 | `m2[42] = "answer"` |
| `ident += call()` | 7 | examples/uql/emitter/pg_emitter.go:72 | `result += lexer.TokenToString(expr.Value)` |
| `ident += expr` | 2 | tests/lang-constructs/main.go:250 | `sumItems += items[i] // 10 + 20 + 30 = 60` |
| `ident += ident` | 14 | tests/lang-constructs/main.go:141 | `sum += i` |
| `ident += int` | 3 | tests/lang-constructs/main.go:158 | `for i := 0; i < 10; i += 2 {` |
| `ident += string` | 22 | examples/uql/emitter/pg_emitter.go:43 | `result += "("` |
| `ident -= int` | 2 | tests/lang-constructs/main.go:206 | `for i := 9; i > 0; i -= 3 {` |
| `ident := -expr` | 5 | examples/mos6502/lib/basic/codegen.go:611 | `loopIdx := -1` |
| `ident := [](ident, ident){...}` | 1 | tests/lang-constructs/main.go:285 | `x := []func(int, int){` |
| `ident := []ident{...}` | 71 | tests/lang-constructs/main.go:64 | `intSlice := []int{1, 2, 3}` |
| `ident := call()` | 233 | tests/lang-constructs/main.go:480 | `b := int8(a)` |
| `ident := expr` | 222 | tests/lang-constructs/main.go:292 | `f := x[0]` |
| `ident := float` | 3 | examples/gui-demo/main.go:20 | `volume := 0.5` |
| `ident := func(){...}` | 1 | examples/uql/lexer/tokenizer.go:57 | `addToken := func() {` |
| `ident := ident` | 37 | tests/lang-constructs/main.go:231 | `flag := false` |
| `ident := ident.field` | 14 | examples/ast-demo/main.go:47 | `tag := nodes[idx].Tag` |
| `ident := ident.field{...}` | 23 | tests/lang-constructs/main.go:544 | `record := types.DataRecord{` |
| `ident := ident{...}` | 8 | tests/lang-constructs/main.go:61 | `c := Composite{}` |
| `ident := int` | 211 | tests/lang-constructs/main.go:88 | `for x := 0; x < 10; x++ {` |
| `ident := string` | 41 | tests/lang-constructs/main.go:453 | `s := "hello"` |
| `ident = !expr` | 3 | examples/gui-demo/main.go:354 | `showDemo = !showDemo` |
| `ident = -expr` | 8 | examples/ast-demo/main.go:100 | `n = -n` |
| `ident = call()` | 1037 | tests/lang-constructs/main.go:487 | `a = append(a, 1)` |
| `ident = expr` | 363 | tests/lang-constructs/main.go:343 | `a = a + 5` |
| `ident = float` | 6 | tests/lang-constructs/main.go:745 | `x = 3.14` |
| `ident = ident` | 48 | tests/lang-constructs/main.go:235 | `flag = false` |
| `ident = ident.field` | 2 | tests/lang-constructs/main.go:560 | `kind = types.ExprLiteral` |
| `ident = ident.field{...}` | 1 | examples/uql/parser/parser.go:92 | `lhs = ast.LogicalExpr{` |
| `ident = ident{...}` | 2 | examples/uql/emitter/pg_emitter.go:225 | `state = EmitterState{Result: "", First: true}` |
| `ident = int` | 66 | tests/lang-constructs/main.go:342 | `a = 1` |
| `ident = string` | 3 | tests/lang-constructs/main.go:743 | `x = "hello"` |
| `ident, ident := call()` | 16 | examples/gui-demo/main.go:9 | `screenW, screenH := graphics.GetScreenSize()` |
| `ident, ident := expr` | 9 | tests/lang-constructs/main.go:999 | `val, ok := m["hello"]` |
| `ident, ident = call()` | 215 | tests/lang-constructs/main.go:600 | `plan, idx = types.AddLiteralToPlan(plan, "first")` |
| `ident, ident, ident := call()` | 5 | examples/mos6502/lib/basic/basic.go:364 | `varName, startVal, endVal := ParseFor(args)` |
| `ident, ident, ident = call()` | 20 | examples/gui-demo/main.go:80 | `ctx, menuState, fileMenuOpen = gui.Menu(ctx, w, menuState, "File")` |
| `ident, ident.field = call()` | 1 | examples/mos6502/lib/cpu/cpu.go:1217 | `c, c.A = PullByte(c)` |
| `ident.field += call()` | 33 | examples/uql/emitter/pg_emitter.go:130 | `s.Result += lexer.TokenToString(stmt.Fields[i].Alias)` |
| `ident.field += ident` | 20 | examples/uql/emitter/pg_emitter.go:127 | `s.Result += fieldStr` |
| `ident.field += string` | 42 | examples/uql/emitter/pg_emitter.go:106 | `s.Result += "SELECT "` |
| `ident.field = -expr` | 2 | examples/ast-demo/main.go:27 | `n.Left = -1` |
| `ident.field = []ident{...}` | 4 | tests/lang-constructs/main.go:585 | `plan.Literals = []string{}` |
| `ident.field = call()` | 63 | tests/lang-constructs/main.go:1077 | `s.Settings = make(map[string]int)` |
| `ident.field = expr` | 104 | examples/gui-demo/main.go:124 | `ctx.CursorY = ctx.CursorY + 4` |
| `ident.field = ident` | 48 | examples/ast-demo/main.go:25 | `n.Tag = NodeNum` |
| `ident.field = ident.field` | 14 | examples/mos6502/cmd/c64-v2/main.go:654 | `state.InputStartRow = state.CursorRow` |
| `ident.field = ident.field{...}` | 4 | examples/uql/transform/postgresql.go:80 | `result.From = ast.PgFromClause{` |
| `ident.field = int` | 8 | tests/lang-constructs/main.go:586 | `plan.Root = 0` |

## BinaryExpr

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `expr != expr` | 101 | tests/lang-constructs/main.go:400 | `if a != b {` |
| `expr % expr` | 11 | tests/lang-constructs/main.go:138 | `if i%2 == 0 {` |
| `expr & expr` | 217 | examples/mos6502/cmd/c64/main.go:59 | `high := (n >> 4) & 0x0F` |
| `expr && expr` | 237 | tests/lang-constructs/main.go:219 | `for i := 0; i < 10 && i < limit; i++ {` |
| `expr * expr` | 99 | tests/lang-constructs/main.go:362 | `prod := a * b` |
| `expr + expr` | 663 | tests/lang-constructs/main.go:343 | `a = a + 5` |
| `expr - expr` | 123 | tests/lang-constructs/main.go:361 | `diff := a - b` |
| `expr / expr` | 18 | tests/lang-constructs/main.go:363 | `quot := a / b` |
| `expr < expr` | 122 | tests/lang-constructs/main.go:88 | `for x := 0; x < 10; x++ {` |
| `expr << expr` | 10 | examples/mos6502/lib/cpu/cpu.go:791 | `c.A = uint8((int(c.A) << 1) & 0xFF)` |
| `expr <= expr` | 49 | tests/lang-constructs/main.go:182 | `for i := 1; i <= 5; i++ {` |
| `expr == expr` | 1182 | tests/lang-constructs/main.go:65 | `if len(intSlice) == 3 {` |
| `expr > expr` | 62 | tests/lang-constructs/main.go:170 | `for i := 5; i > 0; i-- {` |
| `expr >= expr` | 237 | tests/lang-constructs/main.go:125 | `if counter2 >= 3 {` |
| `expr >> expr` | 73 | examples/mos6502/cmd/c64/main.go:59 | `high := (n >> 4) & 0x0F` |
| `expr ^ expr` | 12 | examples/mos6502/lib/cpu/cpu.go:749 | `c.A = c.A ^ value` |
| `expr | expr` | 31 | examples/mos6502/lib/cpu/cpu.go:321 | `c.Status = c.Status | FlagZ` |
| `expr || expr` | 67 | tests/lang-constructs/main.go:232 | `for i := 0; i < 3 || flag; i++ {` |

## BranchStmt

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `break` | 217 | tests/lang-constructs/main.go:126 | `break` |
| `continue` | 37 | tests/lang-constructs/main.go:139 | `continue` |

## CallExpr

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `expr[expr](int, int)` | 1 | tests/lang-constructs/main.go:294 | `x[0](20, 30)` |
| `ident()` | 64 | tests/lang-constructs/main.go:47 | `testSliceOperations()` |
| `ident([]ident{...}, int)` | 3 | tests/lang-constructs/main.go:865 | `checkSliceSum([]int{10, 20, 30}, 60)` |
| `ident(call())` | 38 | examples/gui-demo/main.go:305 | `ctx = gui.AutoLabel(ctx, w, "  Spinner: "+intToString(int(spinnerValue)))` |
| `ident(call(), string, int)` | 1 | tests/lang-constructs/main.go:1202 | `m4 := addToMap(createEmptyMap(), "chained", 42)` |
| `ident(char)` | 35 | examples/mos6502/lib/basic/parser.go:86 | `lineNum = lineNum*10 + (ch - int('0'))` |
| `ident(expr)` | 291 | tests/lang-constructs/main.go:887 | `m := make(map[string]int)` |
| `ident(expr, ident)` | 1 | examples/uql/parser/parser.go:89 | `rhsExpr, nextPos := parseExpression(tokens[i+1:], nextPrecedence)` |
| `ident(expr, ident, ident)` | 4 | examples/uql/ast/ast.go:138 | `state = walkLogicalExpr(expr.Expressions[0], state, visitor)` |
| `ident(expr, int)` | 17 | tests/lang-constructs/main.go:1137 | `m = make([][]int, 2)` |
| `ident(expr, string)` | 5 | examples/mos6502/lib/assembler/assembler.go:355 | `if i < len(tokens) && tokens[i].Type == TokenTypeIdentifier && MatchToken(tok...` |
| `ident(ident)` | 611 | tests/lang-constructs/main.go:65 | `if len(intSlice) == 3 {` |
| `ident(ident, call())` | 313 | examples/mos6502/cmd/graphic/main.go:84 | `lines = append(lines, makeLdaImm(int(color)))` |
| `ident(ident, expr)` | 103 | examples/mos6502/cmd/c64/main.go:88 | `lines = append(lines, "LDA #"+toHex(charCode))` |
| `ident(ident, expr, expr, int, call())` | 1 | examples/gui-demo/main.go:318 | `drawSphere(w, sphereWin.X+125, sphereWin.Y+150, 80,` |
| `ident(ident, ident)` | 94 | examples/ast-demo/main.go:29 | `nodes = append(nodes, n)` |
| `ident(ident, ident, ident)` | 4 | examples/mos6502/cmd/textscroll/main.go:104 | `charCode := getCharAt(message, scrollOffset, col)` |
| `ident(ident, ident, ident, ident)` | 8 | examples/ast-demo/main.go:132 | `nodes, iAdd = addBinOp(nodes, NodeAdd, i3, i4)` |
| `ident(ident, ident, ident.field)` | 7 | examples/uql/transform/postgresql.go:59 | `result = transformFrom(state, result, stmt.FromF)` |
| `ident(ident, ident.field)` | 66 | examples/ast-demo/main.go:51 | `leftVal := evalNode(nodes, nodes[idx].Left)` |
| `ident(ident, ident.field, call())` | 2 | examples/mos6502/lib/cpu/cpu.go:1073 | `c = doCMP(c, c.A, ReadIndirectX(c, zp))` |
| `ident(ident, ident.field, expr)` | 9 | examples/mos6502/lib/cpu/cpu.go:1053 | `c = doCMP(c, c.A, c.Memory[int(addr)])` |
| `ident(ident, ident.field, ident)` | 3 | examples/mos6502/lib/cpu/cpu.go:1049 | `c = doCMP(c, c.A, value)` |
| `ident(ident, ident.field, ident.field, ident)` | 1 | examples/mos6502/lib/basic/basic.go:150 | `lineAsm, ctx = compileLine(line, state.CursorRow, state.CursorCol, ctx)` |
| `ident(ident, ident.field{...})` | 7 | examples/uql/parser/parser.go:287 | `resultAst = append(resultAst, ast.Statement{Type: int8(ast.StatementTypeFrom)...` |
| `ident(ident, ident{...})` | 22 | examples/mos6502/lib/assembler/assembler.go:118 | `tokens = append(tokens, Token{Type: TokenTypeNewline, Representation: []int8{...` |
| `ident(ident, int)` | 10 | tests/lang-constructs/main.go:487 | `a = append(a, 1)` |
| `ident(ident, int, int, ident)` | 1 | examples/mos6502/lib/basic/codegen.go:556 | `thenLines, ctx = compileLine(thenStmt, 0, 0, ctx) // Row/col not used for non...` |
| `ident(ident, string)` | 316 | tests/lang-constructs/main.go:908 | `delete(m, "hello")` |
| `ident(ident, string, int)` | 3 | tests/lang-constructs/main.go:888 | `m = setMapValue(m, "hello", 1)` |
| `ident(ident, string, int, int)` | 6 | examples/mos6502/cmd/c64/main.go:463 | `lines = addStringToScreen(lines, "**** COMMODORE 64 BASIC V2 ****", 1, 4)` |
| `ident(ident.field)` | 367 | tests/lang-constructs/main.go:78 | `if len(c.a) == 0 {` |
| `ident(ident.field, call())` | 2 | examples/uql/lexer/lexer_native.go:7 | `token.Representation = append(token.Representation, int8(r))` |
| `ident(ident.field, ident)` | 14 | tests/lang-constructs/types/types.go:76 | `plan.Literals = append(plan.Literals, value)` |
| `ident(ident.field, ident, ident)` | 2 | examples/uql/ast/ast.go:117 | `state = walkLogicalExpr(where.Expr, state, visitor)` |
| `ident(ident.field, ident, ident, ident)` | 2 | examples/mos6502/lib/basic/basic.go:266 | `lineAsm, ctx = compileLine(state.Lines[i].Text, row, col, ctx)` |
| `ident(ident.field, ident.field)` | 6 | examples/mos6502/cmd/c64-v2/main.go:510 | `line := readLineFromScreen(state.C, state.InputStartRow)` |
| `ident(ident.field, ident.field, ident.field)` | 2 | examples/mos6502/lib/basic/codegen.go:312 | `rightLines := genSimpleValue(expr.RightType, expr.RightValue, expr.RightVar)` |
| `ident(ident.field, string)` | 9 | tests/lang-constructs/main.go:1089 | `delete(s.Settings, "retries")` |
| `ident(int)` | 22 | examples/graphics-demo/main.go:33 | `x := int32(600)` |
| `ident(int, int)` | 1 | tests/lang-constructs/main.go:293 | `f(10, 20)` |
| `ident(int, int, int, int, call())` | 5 | examples/mos6502/cmd/graphic/main.go:195 | `rectProg := createDrawRectProgram(8, 4, 24, 12, uint8(2))` |
| `ident.field()` | 47 | tests/lang-constructs/main.go:466 | `fmt.Println()` |
| `ident.field([]ident.field{...})` | 1 | examples/uql/emitter/uql_emitter.go:204 | `newState.Result += lexer.DumpTokensString([]lexer.Token{expr.Value})` |
| `ident.field(call())` | 6 | examples/ast-demo/main.go:136 | `fmt.Println(printExpr(nodes, iMul))` |
| `ident.field(expr)` | 12 | examples/uql/emitter/uql_emitter.go:55 | `newState.Result += lexer.DumpTokenString(from.TableExpr[0])` |
| `ident.field(ident)` | 155 | tests/lang-constructs/main.go:104 | `fmt.Println(i)` |
| `ident.field(ident, call())` | 4 | examples/graphics-demo/main.go:12 | `graphics.Clear(w, graphics.NewColor(20, 20, 40, 255))` |
| `ident.field(ident, call(), call())` | 12 | examples/graphics-demo/main.go:17 | `graphics.FillRect(w, graphics.NewRect(50, 50, 200, 150), graphics.Red())` |
| `ident.field(ident, call(), ident)` | 3 | examples/mos6502/cmd/c64/main.go:751 | `graphics.FillRect(w, graphics.NewRect(screenX, screenY, scale, scale), textCo...` |
| `ident.field(ident, call(), ident.field)` | 1 | examples/mos6502/cmd/c64-v2/main.go:731 | `graphics.FillRect(w, graphics.NewRect(screenX, screenY, state.Scale, state.Sc...` |
| `ident.field(ident, expr, expr, expr, expr, ident)` | 4 | examples/gui-demo/main.go:466 | `graphics.DrawLine(w, cx+x00, cy-y0, cx+x01, cy-y0, color)` |
| `ident.field(ident, expr, expr, int)` | 2 | examples/gui-demo/main.go:119 | `ctx = gui.BeginLayout(ctx, demoWin.X+10, demoWin.Y+50, 6)` |
| `ident.field(ident, expr, ident, int)` | 4 | examples/gui-demo/main.go:184 | `ctx = gui.BeginLayout(ctx, widgetsWin.X+15, contentY, 6)` |
| `ident.field(ident, func(){...})` | 6 | examples/graphics-demo/main.go:10 | `graphics.RunLoop(w, func(w graphics.Window) bool {` |
| `ident.field(ident, ident)` | 34 | examples/gui-demo/main.go:71 | `ctx = gui.UpdateInput(ctx, w)` |
| `ident.field(ident, ident, expr)` | 8 | examples/gui-demo/main.go:221 | `ctx = gui.AutoLabel(ctx, w, "Selected: "+listItems[selectedItem])` |
| `ident.field(ident, ident, expr, expr, expr)` | 1 | examples/gui-demo/main.go:138 | `gui.Label(ctx, w, "Count: "+intToString(counter), demoWin.X+190, ctx.CursorY+4)` |
| `ident.field(ident, ident, expr, expr, int)` | 4 | examples/gui-demo/main.go:123 | `gui.Separator(ctx, w, demoWin.X+10, ctx.CursorY-2, 330)` |
| `ident.field(ident, ident, expr, ident.field, int)` | 3 | examples/gui-demo/main.go:200 | `gui.Separator(ctx, w, widgetsWin.X+15, ctx.CursorY, 350)` |
| `ident.field(ident, ident, expr, ident.field, int, int, ident)` | 1 | examples/gui-demo/main.go:162 | `gui.ProgressBar(ctx, w, demoWin.X+10, ctx.CursorY, 320, 20, progress)` |
| `ident.field(ident, ident, ident)` | 7 | examples/mos6502/cmd/c64/main.go:512 | `basicState = basic.SetCursor(basicState, cursorRow, cursorCol)` |
| `ident.field(ident, ident, ident, expr, ident.field, int)` | 2 | examples/gui-demo/main.go:268 | `ctx, textInput = gui.TextInput(ctx, w, textInput, widgetsWin.X+70, ctx.Cursor...` |
| `ident.field(ident, ident, ident, expr, ident.field, int, int, ident, ident)` | 1 | examples/gui-demo/main.go:218 | `ctx, selectedItem, scrollOffset = gui.ListBox(ctx, w, listItems, widgetsWin.X...` |
| `ident.field(ident, ident, ident, ident)` | 14 | examples/mos6502/cmd/c64/main.go:751 | `graphics.FillRect(w, graphics.NewRect(screenX, screenY, scale, scale), textCo...` |
| `ident.field(ident, ident, ident, ident, expr, expr)` | 1 | examples/gui-demo/main.go:178 | `ctx, tabState = gui.TabBar(ctx, w, tabState, tabLabels, widgetsWin.X+10, widg...` |
| `ident.field(ident, ident, ident, ident, int)` | 3 | examples/gui-demo/main.go:335 | `ctx, dropY = gui.BeginDropdown(ctx, w, menuState, dropX, 5) // 5 items in Fil...` |
| `ident.field(ident, ident, ident, int, int, call())` | 1 | examples/gui-demo/main.go:77 | `ctx, menuState = gui.BeginMenuBar(ctx, w, menuState, 0, 0, graphics.GetWidth(w))` |
| `ident.field(ident, ident, ident, string)` | 9 | examples/gui-demo/main.go:80 | `ctx, menuState, fileMenuOpen = gui.Menu(ctx, w, menuState, "File")` |
| `ident.field(ident, ident, ident, string, expr, ident, int)` | 3 | examples/gui-demo/main.go:232 | `ctx, treeState, expanded = gui.TreeNode(ctx, w, treeState, "Root", widgetsWin...` |
| `ident.field(ident, ident, ident, string, ident, ident, int)` | 7 | examples/gui-demo/main.go:336 | `ctx, menuState, clicked = gui.MenuItem(ctx, w, menuState, "New", dropX, dropY...` |
| `ident.field(ident, ident, ident.field, ident.field)` | 1 | examples/mos6502/cmd/c64-v2/main.go:731 | `graphics.FillRect(w, graphics.NewRect(screenX, screenY, state.Scale, state.Sc...` |
| `ident.field(ident, ident, int)` | 10 | examples/mos6502/cmd/c64/main.go:497 | `c = cpu.LoadProgram(c, program, 0x0600)` |
| `ident.field(ident, ident, int, call())` | 1 | examples/graphics-demo/main.go:38 | `graphics.DrawPoint(w, x, 300, graphics.White())` |
| `ident.field(ident, ident, int, ident.field, call())` | 1 | examples/gui-demo/main.go:94 | `ctx, toolbarState = gui.BeginToolbar(ctx, w, 0, menuState.MenuBarH, graphics....` |
| `ident.field(ident, ident, string)` | 9 | examples/gui-demo/main.go:121 | `ctx = gui.AutoLabel(ctx, w, "Hello from goany GUI!")` |
| `ident.field(ident, ident, string, expr)` | 3 | examples/gui-demo/main.go:187 | `ctx, clicked = gui.AutoRadioButton(ctx, w, "Option A", radioSelection == 0)` |
| `ident.field(ident, ident, string, expr, expr)` | 3 | examples/gui-demo/main.go:267 | `gui.Label(ctx, w, "Name:", widgetsWin.X+15, ctx.CursorY+4)` |
| `ident.field(ident, ident, string, expr, expr, int, int)` | 1 | examples/gui-demo/main.go:290 | `ctx, clicked = gui.Button(ctx, w, "Close", anotherWin.X+10, anotherWin.Y+90, ...` |
| `ident.field(ident, ident, string, expr, ident, int)` | 6 | examples/gui-demo/main.go:240 | `ctx, clicked = gui.TreeLeaf(ctx, w, "Leaf 1.1", widgetsWin.X+15, nodeY, 40)` |
| `ident.field(ident, ident, string, expr, ident.field, ident, int, int)` | 1 | examples/gui-demo/main.go:204 | `ctx, spinnerValue = gui.Spinner(ctx, w, "Value", widgetsWin.X+15, ctx.CursorY...` |
| `ident.field(ident, ident, string, expr, ident.field, int, ident)` | 1 | examples/gui-demo/main.go:211 | `ctx, pickedColor = gui.ColorPicker(ctx, w, "Color Picker:", widgetsWin.X+15, ...` |
| `ident.field(ident, ident, string, expr, ident.field, int, int)` | 2 | examples/gui-demo/main.go:127 | `ctx, clicked = gui.Button(ctx, w, "Click", demoWin.X+10, ctx.CursorY, 80, 26)` |
| `ident.field(ident, ident, string, ident)` | 9 | examples/gui-demo/main.go:117 | `ctx, demoWin = gui.DraggablePanel(ctx, w, "Demo Window", demoWin)` |
| `ident.field(ident, ident, string, ident.field, ident.field)` | 1 | examples/gui-demo/main.go:311 | `gui.Tooltip(ctx, w, "Drag to move this panel", ctx.MouseX, ctx.MouseY)` |
| `ident.field(ident, ident, string, int, float, float, ident)` | 2 | examples/gui-demo/main.go:154 | `ctx, volume = gui.AutoSlider(ctx, w, "Volume", 320, 0.0, 1.0, volume)` |
| `ident.field(ident, ident, string, int, int, int, int)` | 1 | examples/gui-demo/main.go:327 | `ctx, clicked = gui.Button(ctx, w, "Quit", 1150, 900, 100, 30)` |
| `ident.field(ident, ident.field)` | 1 | examples/mos6502/cmd/c64-v2/main.go:689 | `graphics.Clear(w, state.BgColor)` |
| `ident.field(ident, int)` | 58 | examples/containers/main.go:11 | `list = containers.Add(list, 10)` |
| `ident.field(ident, int, int)` | 3 | examples/containers/main.go:46 | `list = containers.InsertAt(list, 2, 25)` |
| `ident.field(ident, int, int, int, call())` | 2 | examples/graphics-demo/main.go:23 | `graphics.FillCircle(w, 150, 400, 80, graphics.Blue())` |
| `ident.field(ident, int, int, int, int, call())` | 2 | examples/graphics-demo/main.go:29 | `graphics.DrawLine(w, 550, 50, 750, 200, graphics.NewColor(255, 255, 0, 255))` |
| `ident.field(ident, string)` | 14 | tests/lang-constructs/main.go:600 | `plan, idx = types.AddLiteralToPlan(plan, "first")` |
| `ident.field(ident.field)` | 38 | tests/lang-constructs/types/types.go:83 | `fmt.Println(record.ID)` |
| `ident.field(ident.field, ident)` | 4 | examples/mos6502/cmd/c64-v2/main.go:569 | `pl := basic.GetLine(state.BasicState, i)` |
| `ident.field(ident.field, ident, ident)` | 9 | examples/mos6502/cmd/c64-v2/main.go:529 | `state.BasicState = basic.StoreLine(state.BasicState, lineNum, rest)` |
| `ident.field(ident.field, ident, int)` | 3 | examples/mos6502/cmd/c64-v2/main.go:537 | `state.C = cpu.LoadProgram(state.C, code, 0xC000)` |
| `ident.field(ident.field, ident.field, int)` | 2 | examples/mos6502/cmd/c64-v2/main.go:534 | `state.BasicState = basic.SetCursor(state.BasicState, state.CursorRow, 0)` |
| `ident.field(ident.field, int)` | 6 | examples/mos6502/cmd/c64-v2/main.go:538 | `state.C = cpu.SetPC(state.C, 0xC000)` |
| `ident.field(ident.field, int, int)` | 1 | examples/mos6502/cmd/c64-v2/main.go:613 | `state.BasicState = basic.SetCursor(state.BasicState, 0, 0)` |
| `ident.field(int)` | 1 | tests/lang-constructs/main.go:465 | `fmt.Println(42)` |
| `ident.field(int, int, int, int)` | 27 | examples/graphics-demo/main.go:12 | `graphics.Clear(w, graphics.NewColor(20, 20, 40, 255))` |
| `ident.field(string)` | 239 | tests/lang-constructs/main.go:66 | `fmt.Println("PASS: slice literal len")` |
| `ident.field(string, call())` | 16 | examples/containers/main.go:19 | `fmt.Printf("Size: %d\n", containers.Size(list))` |
| `ident.field(string, call(), call())` | 1 | examples/gui-demo/main.go:12 | `w := graphics.CreateWindow("GUI Widgets Demo", int32(winW), int32(winH))` |
| `ident.field(string, expr)` | 9 | examples/containers/main.go:36 | `fmt.Printf("%d ", slice[idx])` |
| `ident.field(string, ident)` | 9 | examples/containers/main.go:160 | `fmt.Printf("Popped: %d\n", sval)` |
| `ident.field(string, ident, ident)` | 5 | examples/mos6502/cmd/c64/main.go:485 | `w := graphics.CreateWindow("Commodore 64", windowWidth, windowHeight)` |
| `ident.field(string, ident.field)` | 3 | examples/uql/lexer/lexer.go:184 | `fmt.Printf("Token type: %d ", token.Type)` |
| `ident.field(string, int)` | 1 | tests/lang-constructs/main.go:473 | `fmt.Printf("%d\n", 100)` |
| `ident.field(string, int, int)` | 3 | tests/lang-constructs/main.go:578 | `fmt.Printf("a=%d, b=%d\n", 10, 20)` |
| `ident.field(string, string)` | 1 | tests/lang-constructs/main.go:474 | `fmt.Printf("%s\n", "test")` |
| `ident.field(string, string, int)` | 1 | tests/lang-constructs/main.go:579 | `fmt.Printf("name=%s, value=%d\n", "test", 100)` |

## CompositeLit

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `[](ident, ident){positional}` | 1 | tests/lang-constructs/main.go:285 | `x := []func(int, int){` |
| `[]ident.field{positional}` | 2 | examples/uql/emitter/uql_emitter.go:204 | `newState.Result += lexer.DumpTokensString([]lexer.Token{expr.Value})` |
| `[]ident{empty}` | 64 | tests/lang-constructs/main.go:308 | `a := []int8{}` |
| `[]ident{positional}` | 37 | tests/lang-constructs/main.go:64 | `intSlice := []int{1, 2, 3}` |
| `ident.field{empty}` | 3 | examples/uql/parser/parser.go:269 | `return ast.AST{}, -1` |
| `ident.field{keyed}` | 40 | tests/lang-constructs/main.go:544 | `record := types.DataRecord{` |
| `ident{empty}` | 5 | tests/lang-constructs/main.go:61 | `c := Composite{}` |
| `ident{keyed}` | 41 | tests/lang-constructs/main.go:505 | `p := Person{name: "Alice", age: 30}` |

## ConstDecl

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `const ident _ = int` | 246 | examples/mos6502/cmd/c64/main.go:13 | `const TextCols = 40` |
| `const ident ident = int` | 10 | tests/lang-constructs/types/types.go:7 | `ExprLiteral  ExprKind = 0` |

## ForStmt

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `for _; _; _` | 163 | tests/lang-constructs/main.go:123 | `for {` |
| `for _; ident < ident(ident); _` | 11 | tests/lang-constructs/main.go:793 | `for i < len(s) {` |
| `for _; ident < ident; _` | 2 | examples/gui-demo/main.go:112 | `for pass < numPasses {` |
| `for _; ident < int; _` | 8 | tests/lang-constructs/main.go:111 | `for counter < 5 {` |
| `for _; ident > int; _` | 2 | examples/ast-demo/main.go:103 | `for n > 0 {` |
| `for _; ident(ident) > int; _` | 1 | examples/uql/parser/parser.go:264 | `for len(tokens) > 0 {` |
| `for ident := int; ident < ident && ident < ident(ident); ident++` | 1 | tests/lang-constructs/main.go:249 | `for i := 0; i < maxItems && i < len(items); i++ {` |
| `for ident := int; ident < ident(ident); ident++` | 5 | tests/lang-constructs/main.go:1160 | `for i := 0; i < len(m); i++ {` |
| `for ident := int; ident < ident(ident.field); ident++` | 16 | examples/uql/emitter/pg_emitter.go:74 | `for i := 0; i < len(expr.Expressions); i++ {` |
| `for ident := int; ident < ident(ident.field.field); ident++` | 2 | examples/uql/emitter/pg_emitter.go:171 | `for i := 0; i < len(stmt.GroupBy.Fields); i++ {` |
| `for ident := int; ident < ident(ident[ident]); ident++` | 1 | tests/lang-constructs/main.go:1161 | `for j := 0; j < len(m[i]); j++ {` |
| `for ident := int; ident < int && ident < ident && ident < int; ident++` | 1 | tests/lang-constructs/main.go:262 | `for i := 0; i < 10 && i < limit2 && sumMulti < 20; i++ {` |
| `for ident := int; ident < int && ident < ident; ident++` | 1 | tests/lang-constructs/main.go:219 | `for i := 0; i < 10 && i < limit; i++ {` |
| `for ident := int; ident < int || ident; ident++` | 1 | tests/lang-constructs/main.go:232 | `for i := 0; i < 3 || flag; i++ {` |
| `for ident := int; ident < int; ident += int` | 1 | tests/lang-constructs/main.go:158 | `for i := 0; i < 10; i += 2 {` |
| `for ident := int; ident < int; ident++` | 3 | tests/lang-constructs/main.go:88 | `for x := 0; x < 10; x++ {` |
| `for ident := int; ident <= int; ident++` | 1 | tests/lang-constructs/main.go:182 | `for i := 1; i <= 5; i++ {` |
| `for ident := int; ident > int; ident -= int` | 1 | tests/lang-constructs/main.go:206 | `for i := 9; i > 0; i -= 3 {` |
| `for ident := int; ident > int; ident--` | 1 | tests/lang-constructs/main.go:170 | `for i := 5; i > 0; i-- {` |
| `for ident := int; ident >= int; ident--` | 1 | tests/lang-constructs/main.go:194 | `for i := 3; i >= 0; i-- {` |

## FuncDecl

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `func ident()` | 56 | tests/lang-constructs/main.go:59 | `func testSliceOperations() {` |
| `func ident() []ident` | 11 | examples/gui-demo/main.go:413 | `func sinTable() []int {` |
| `func ident() ident` | 5 | tests/lang-constructs/main.go:46 | `func testBasicConstructs() int8 {` |
| `func ident() ident, ident` | 2 | tests/lang-constructs/main.go:54 | `func testFunctionCalls() (int16, int16) {` |
| `func ident() map[ident]ident` | 2 | tests/lang-constructs/main.go:1113 | `func createStringIntMap() map[string]int {` |
| `func ident([]ident)` | 1 | examples/uql/lexer/lexer.go:201 | `func DumpTokens(tokens []Token) {` |
| `func ident([]ident) []ident` | 7 | examples/mos6502/cmd/c64/main.go:96 | `func clearScreen(lines []string) []string {` |
| `func ident([]ident) []ident, ident` | 1 | examples/mos6502/lib/assembler/assembler.go:1335 | `func AssembleLinesWithCount(lines []string) ([]uint8, int) {` |
| `func ident([]ident) ident` | 6 | examples/mos6502/lib/assembler/assembler.go:221 | `func ParseHex(bytes []int8) int {` |
| `func ident([]ident) ident, []ident` | 1 | examples/uql/lexer/lexer.go:278 | `func GetNextToken(tokens []Token) (Token, []Token) {` |
| `func ident([]ident, []ident) []ident` | 2 | examples/mos6502/cmd/textscroll/main.go:115 | `func createMultiLineTextProgram(messages []string, scrollOffsets []int) []uin...` |
| `func ident([]ident, []ident) ident` | 1 | examples/mos6502/lib/assembler/assembler.go:478 | `func BytesMatch(a []int8, b []int8) bool {` |
| `func ident([]ident, []ident) ident, ident` | 1 | examples/mos6502/lib/assembler/assembler.go:543 | `func findLabelAddr(labels []LabelEntry, target []int8) (int, bool) {` |
| `func ident([]ident, ident)` | 1 | tests/lang-constructs/main.go:790 | `func checkSliceSum(s []int, expected int) {` |
| `func ident([]ident, ident) []ident` | 2 | examples/mos6502/lib/assembler/assembler.go:558 | `func resolveLabels(instructions []Instruction, baseAddr int) []Instruction {` |
| `func ident([]ident, ident) []ident, ident` | 1 | examples/ast-demo/main.go:22 | `func addNum(nodes []Node, val int) ([]Node, int) {` |
| `func ident([]ident, ident) ident` | 3 | examples/ast-demo/main.go:46 | `func evalNode(nodes []Node, idx int) int {` |
| `func ident([]ident, ident, ident) ident` | 1 | examples/mos6502/lib/font/font.go:229 | `func GetRow(fontData []uint8, charCode int, y int) uint8 {` |
| `func ident([]ident, ident, ident, ident) []ident` | 2 | examples/mos6502/cmd/c64/main.go:79 | `func addStringToScreen(lines []string, text string, row int, col int) []string {` |
| `func ident([]ident, ident, ident, ident) []ident, ident` | 1 | examples/ast-demo/main.go:33 | `func addBinOp(nodes []Node, tag int, left int, right int) ([]Node, int) {` |
| `func ident([]ident, ident, ident, ident) ident` | 1 | examples/mos6502/lib/font/font.go:243 | `func GetPixel(fontData []uint8, charCode int, x int, y int) bool {` |
| `func ident([]ident.field) ident` | 1 | examples/uql/emitter/uql_emitter.go:38 | `func EmitUql(statements []ast.Statement) string {` |
| `func ident([]ident.field) ident.field` | 1 | examples/uql/transform/postgresql.go:27 | `func TransformToPostgreSQL(uqlAst []ast.Statement) ast.PgSelectStatement {` |
| `func ident([]ident.field) ident.field, ident` | 1 | examples/uql/parser/parser.go:64 | `func ParseExpression(tokens []lexer.Token) (ast.LogicalExpr, int) {` |
| `func ident([]ident.field, ident) ident.field, ident` | 1 | examples/uql/parser/parser.go:69 | `func parseExpression(tokens []lexer.Token, minPrecedence int8) (ast.LogicalEx...` |
| `func ident([]ident.field, ident.field) ident.field, []ident.field` | 7 | examples/uql/parser/parser.go:108 | `func parseFrom(tokens []lexer.Token, lhs lexer.Token) (ast.From, []lexer.Toke...` |
| `func ident(ident)` | 3 | tests/lang-constructs/main.go:303 | `func sink(p int8) {` |
| `func ident(ident) []ident` | 10 | examples/mos6502/lib/assembler/assembler.go:77 | `func StringToBytes(s string) []int8 {` |
| `func ident(ident) []ident, ident, ident, ident` | 1 | examples/mos6502/lib/basic/basic.go:288 | `func CompileProgramDebug(state BasicState) ([]uint8, int, int, int) {` |
| `func ident(ident) ident` | 107 | examples/ast-demo/main.go:67 | `func tagName(tag int) string {` |
| `func ident(ident) ident, ident` | 9 | examples/mos6502/lib/basic/codegen.go:465 | `func nextLabel(ctx CompileContext) (string, CompileContext) {` |
| `func ident(ident) ident, ident, ident` | 3 | examples/mos6502/lib/basic/parser.go:58 | `func parseLineNumber(line string) (int, string, bool) {` |
| `func ident(ident) ident.field, ident` | 1 | examples/uql/parser/parser.go:260 | `func Parse(text string) (ast.AST, int8) {` |
| `func ident(ident, []ident, ident) ident` | 1 | examples/mos6502/lib/cpu/cpu.go:267 | `func LoadProgram(c CPU, program []uint8, addr int) CPU {` |
| `func ident(ident, ident) []ident` | 4 | examples/mos6502/lib/basic/basic.go:133 | `func CompileImmediate(state BasicState, line string) []uint8 {` |
| `func ident(ident, ident) []ident, ident` | 1 | examples/mos6502/lib/basic/codegen.go:607 | `func genNext(varName string, ctx CompileContext) ([]string, CompileContext) {` |
| `func ident(ident, ident) ident` | 19 | examples/mos6502/lib/assembler/assembler.go:258 | `func MatchToken(token Token, s string) bool {` |
| `func ident(ident, ident) ident, ident` | 1 | tests/lang-constructs/types/types.go:75 | `func AddLiteralToPlan(plan Plan, value string) (Plan, int) {` |
| `func ident(ident, ident, ident) []ident` | 2 | examples/mos6502/cmd/textscroll/main.go:95 | `func createScrollingTextProgram(message string, scrollOffset int, row int) []...` |
| `func ident(ident, ident, ident) []ident, ident` | 1 | examples/mos6502/lib/basic/codegen.go:509 | `func genIf(cond Condition, thenStmt string, ctx CompileContext) ([]string, Co...` |
| `func ident(ident, ident, ident) ident` | 16 | examples/mos6502/cmd/textscroll/main.go:81 | `func getCharAt(message string, scrollOffset int, position int) int {` |
| `func ident(ident, ident, ident, ident) []ident, ident` | 4 | examples/mos6502/lib/basic/basic.go:333 | `func compileLine(line string, cursorRow int, cursorCol int, ctx CompileContex...` |
| `func ident(ident, ident, ident, ident, ident) []ident` | 2 | examples/mos6502/cmd/graphic/main.go:70 | `func createDrawRectProgram(startX int, startY int, endX int, endY int, color ...` |
| `func ident(ident, ident.field) ident` | 1 | examples/uql/transform/postgresql.go:17 | `func findBinding(state TransformState, name lexer.Token) int {` |
| `func ident(ident, ident.field, ident.field) ident.field` | 7 | examples/uql/transform/postgresql.go:78 | `func transformFrom(state TransformState, result ast.PgSelectStatement, from a...` |
| `func ident(ident.field) ident` | 5 | examples/uql/emitter/pg_emitter.go:13 | `func emitExpression(expr ast.PgExpression) string {` |
| `func ident(ident.field) ident.field` | 4 | examples/mos6502/cmd/c64/main.go:415 | `func scrollScreenUp(c cpu.CPU) cpu.CPU {` |
| `func ident(ident.field, ident) ident` | 2 | examples/mos6502/cmd/c64/main.go:358 | `func readLineFromScreen(c cpu.CPU, row int) string {` |
| `func ident(ident.field, ident) ident, ident` | 1 | examples/mos6502/cmd/c64-v2/main.go:498 | `func frame(w graphics.Window, state C64State) (C64State, bool) {` |
| `func ident(ident.field, ident) ident.field` | 2 | examples/mos6502/cmd/c64/main.go:400 | `func printReady(c cpu.CPU, row int) cpu.CPU {` |
| `func ident(ident.field, ident, ident, ident, ident.field)` | 1 | examples/gui-demo/main.go:433 | `func drawSphere(w graphics.Window, cx int32, cy int32, radius int32, color gr...` |
| `func ident(map[ident]ident) ident` | 1 | tests/lang-constructs/main.go:881 | `func getMapLen(m map[string]int) int {` |
| `func ident(map[ident]ident, ident, ident) map[ident]ident` | 2 | tests/lang-constructs/main.go:875 | `func setMapValue(m map[string]int, key string, value int) map[string]int {` |

## FuncLit

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `func()` | 1 | examples/uql/lexer/tokenizer.go:57 | `addToken := func() {` |
| `func(ident, ident)` | 1 | tests/lang-constructs/main.go:286 | `func(a int, b int) {` |
| `func(ident, ident.field) ident` | 20 | examples/uql/emitter/pg_emitter.go:102 | `PreVisitSelect: func(state any, stmt ast.PgSelectStatement) any {` |
| `func(ident.field) ident` | 6 | examples/graphics-demo/main.go:10 | `graphics.RunLoop(w, func(w graphics.Window) bool {` |

## IfStmt

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `if !expr [else-if]` | 1 | tests/lang-constructs/main.go:89 | `if !(len(a) == 0) {` |
| `if !expr [no-else]` | 16 | tests/lang-constructs/main.go:275 | `if !b {` |
| `if call() [else-if]` | 71 | examples/mos6502/cmd/c64/main.go:547 | `if basic.HasLineNumber(line) {` |
| `if call() [else]` | 12 | examples/containers/main.go:24 | `if containers.Contains(list, 20) {` |
| `if call() [no-else]` | 47 | examples/mos6502/cmd/c64/main.go:656 | `if basic.IsCommand(line, "PRINT") {` |
| `if expr != expr [else]` | 1 | examples/mos6502/lib/cpu/cpu.go:325 | `if (value & 0x80) != 0 {` |
| `if expr != expr [no-else]` | 31 | tests/lang-constructs/main.go:400 | `if a != b {` |
| `if expr && expr [else-if]` | 6 | examples/mos6502/lib/assembler/assembler.go:230 | `if b >= '0' && b <= '9' {` |
| `if expr && expr [else]` | 40 | tests/lang-constructs/main.go:506 | `if p.name == "Alice" && p.age == 30 {` |
| `if expr && expr [no-else]` | 70 | tests/lang-constructs/main.go:417 | `if a && b {` |
| `if expr < expr [else]` | 1 | examples/mos6502/lib/cpu/cpu.go:419 | `if offset < 128 {` |
| `if expr < expr [no-else]` | 40 | tests/lang-constructs/main.go:402 | `if a < b {` |
| `if expr <= expr [else]` | 2 | examples/mos6502/lib/assembler/assembler.go:398 | `if len(tokens[i].Representation) <= 2 {` |
| `if expr <= expr [no-else]` | 7 | tests/lang-constructs/main.go:406 | `if a <= b {` |
| `if expr == expr [else-if]` | 696 | examples/gui-demo/main.go:182 | `if tabState.ActiveTab == 0 {` |
| `if expr == expr [else]` | 52 | tests/lang-constructs/main.go:65 | `if len(intSlice) == 3 {` |
| `if expr == expr [no-else]` | 153 | tests/lang-constructs/main.go:73 | `if a[0] == 0 {` |
| `if expr > expr [else-if]` | 2 | examples/mos6502/cmd/c64/main.go:677 | `if cursorCol > 0 {` |
| `if expr > expr [no-else]` | 46 | tests/lang-constructs/main.go:404 | `if a > b {` |
| `if expr >= expr [no-else]` | 194 | tests/lang-constructs/main.go:125 | `if counter2 >= 3 {` |
| `if expr || expr [else-if]` | 4 | examples/mos6502/lib/assembler/assembler.go:893 | `if instr.Mode == ModeAccumulator || instr.Mode == ModeImplied {` |
| `if expr || expr [else]` | 4 | examples/mos6502/lib/assembler/assembler.go:601 | `if IsOpcode(instr.OpcodeBytes, "BPL") || IsOpcode(instr.OpcodeBytes, "BMI") ||` |
| `if expr || expr [no-else]` | 22 | tests/lang-constructs/main.go:419 | `if a || b {` |
| `if ident [else]` | 3 | tests/lang-constructs/main.go:778 | `if boolVal {` |
| `if ident [no-else]` | 32 | tests/lang-constructs/main.go:279 | `if c {` |
| `if ident, ident := expr.(ident); ident [else]` | 1 | tests/lang-constructs/main.go:1059 | `if val, ok := x.(int); ok {` |
| `if ident, ident := expr[expr]; ident [else]` | 2 | tests/lang-constructs/main.go:1041 | `if val, ok := m["hello"]; ok {` |
| `if ident.field [no-else]` | 11 | examples/gui-demo/main.go:118 | `if ctx.DrawContent {` |

## IncDecStmt

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `ident++` | 35 | tests/lang-constructs/main.go:88 | `for x := 0; x < 10; x++ {` |
| `ident--` | 3 | tests/lang-constructs/main.go:170 | `for i := 5; i > 0; i-- {` |
| `ident.field++` | 8 | examples/uql/emitter/uql_emitter.go:42 | `newState.Depth++` |
| `ident.field--` | 9 | examples/uql/emitter/uql_emitter.go:56 | `newState.Depth--` |

## IndexExpr

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `ident.field[(ident & int) | ((ident + int) & int)]` | 1 | examples/mos6502/lib/cpu/cpu.go:1188 | `high := int(c.Memory[(addr&0xFF00)|((addr+1)&0xFF)])` |
| `ident.field[(ident + int) & int]` | 2 | examples/mos6502/lib/cpu/cpu.go:377 | `high := int(c.Memory[(addr+1)&0xFF])` |
| `ident.field[(ident(ident) + int) & int]` | 2 | examples/mos6502/lib/cpu/cpu.go:384 | `high := int(c.Memory[(int(zp)+1)&0xFF])` |
| `ident.field[ident & int]` | 2 | examples/mos6502/lib/cpu/cpu.go:376 | `low := int(c.Memory[addr&0xFF])` |
| `ident.field[ident + (ident * int)]` | 1 | examples/mos6502/lib/cpu/cpu.go:378 | `return c.Memory[low+(high*256)]` |
| `ident.field[ident + ident(ident.field)]` | 18 | examples/mos6502/lib/cpu/cpu.go:461 | `c.A = c.Memory[addr+int(c.X)]` |
| `ident.field[ident + ident]` | 13 | examples/mos6502/cmd/c64/main.go:366 | `ch := int(c.Memory[baseAddr+col])` |
| `ident.field[ident + int]` | 3 | examples/mos6502/lib/basic/basic.go:96 | `if state.Lines[j].LineNum > state.Lines[j+1].LineNum {` |
| `ident.field[ident(ident + ident.field) & int]` | 12 | examples/mos6502/lib/cpu/cpu.go:451 | `c.A = c.Memory[int(addr+c.X)&0xFF]` |
| `ident.field[ident(ident)]` | 29 | examples/mos6502/lib/cpu/cpu.go:383 | `low := int(c.Memory[int(zp)])` |
| `ident.field[ident.field + int]` | 1 | examples/mos6502/lib/cpu/cpu.go:313 | `high := int(c.Memory[c.PC+1])` |
| `ident.field[ident.field()]` | 2 | examples/mos6502/cmd/c64/main.go:562 | `cursorRow = int(c.Memory[basic.GetCursorRowAddr()])` |
| `ident.field[ident.field]` | 2 | examples/mos6502/lib/cpu/cpu.go:305 | `value := c.Memory[c.PC]` |
| `ident.field[ident]` | 101 | examples/mos6502/cmd/c64/main.go:524 | `if c.Memory[oldCursorAddr] == 95 {` |
| `ident.field[int + ident(ident.field)]` | 2 | examples/mos6502/lib/cpu/cpu.go:406 | `c.Memory[0x100+int(c.SP)] = value` |
| `ident.field[int]` | 92 | examples/mos6502/lib/assembler/assembler.go:1374 | `return int(lastInstr.OpcodeBytes[0])` |
| `ident.field[string]` | 5 | tests/lang-constructs/main.go:1078 | `s.Settings["timeout"] = 30` |
| `ident[expr][ident]` | 1 | tests/lang-constructs/main.go:1162 | `sum = sum + m[i][j]` |
| `ident[expr][int]` | 64 | tests/lang-constructs/main.go:1140 | `m[0][0] = 1` |
| `ident[expr][string]` | 29 | tests/lang-constructs/main.go:1216 | `m["outer"]["inner"] = 42` |
| `ident[float]` | 2 | tests/lang-constructs/main.go:962 | `m2[3.14] = 314` |
| `ident[ident + ident]` | 3 | examples/mos6502/lib/basic/parser.go:1053 | `ch := int(args[i+k])` |
| `ident[ident + int]` | 5 | examples/gui-demo/main.go:447 | `li1 := latIdx[lat+1]` |
| `ident[ident - int]` | 4 | examples/mos6502/cmd/c64/main.go:378 | `if result[end-1] != ' ' {` |
| `ident[ident(ident) - int]` | 1 | examples/mos6502/lib/assembler/assembler.go:1372 | `lastInstr := instructions[len(instructions)-1]` |
| `ident[ident]` | 222 | tests/lang-constructs/main.go:250 | `sumItems += items[i] // 10 + 20 + 30 = 60` |
| `ident[int]` | 115 | tests/lang-constructs/main.go:73 | `if a[0] == 0 {` |
| `ident[string]` | 60 | tests/lang-constructs/main.go:890 | `if getMapLen(m) == 2 && m["hello"] == 1 && m["world"] == 2 {` |

## InterfaceType

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `interface{}` | 6 | tests/lang-constructs/main.go:741 | `var x interface{}` |

## MapType

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `map[ident][][]ident` | 2 | tests/lang-constructs/main.go:1321 | `var mapOfNestedSlices map[string][][]int` |
| `map[ident][]ident` | 5 | tests/lang-constructs/main.go:1259 | `var mapOfSlices map[string][]int` |
| `map[ident][]map[ident]ident` | 2 | tests/lang-constructs/main.go:1359 | `var intKeySliceOfMaps map[int][]map[string]int` |
| `map[ident]ident` | 47 | tests/lang-constructs/main.go:875 | `func setMapValue(m map[string]int, key string, value int) map[string]int {` |
| `map[ident]map[ident][]ident` | 2 | tests/lang-constructs/main.go:1302 | `var nestedMapsWithSlice map[string]map[string][]int` |
| `map[ident]map[ident]ident` | 1 | tests/lang-constructs/main.go:1214 | `m := make(map[string]map[string]int)` |

## RangeStmt

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `for ident := range ident` | 1 | tests/lang-constructs/main.go:151 | `for i := range nums {` |
| `for ident, ident := range ident` | 7 | tests/lang-constructs/main.go:95 | `for _, x := range a {` |
| `for ident, ident := range ident.field` | 4 | examples/uql/lexer/lexer.go:177 | `for _, b := range token.Representation {` |

## ReturnStmt

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `return -expr` | 8 | examples/mos6502/lib/assembler/assembler.go:1377 | `return -1` |
| `return []ident{...}` | 3 | examples/gui-demo/main.go:414 | `return []int{0, 105, 208, 309, 407, 500, 588, 669, 743, 809,` |
| `return []ident{...}, ident` | 2 | examples/mos6502/lib/basic/basic.go:371 | `return []string{}, ctx` |
| `return call()` | 35 | tests/lang-constructs/main.go:55 | `return testFunctionVariables()` |
| `return call(), call()` | 1 | examples/mos6502/lib/assembler/assembler.go:1351 | `return Assemble(instructions), len(instructions)` |
| `return call(), ident` | 9 | examples/mos6502/lib/basic/basic.go:346 | `return genPoke(addr, value), ctx` |
| `return char` | 9 | examples/uql/parser/parser.go:38 | `return 'L'` |
| `return expr` | 84 | examples/ast-demo/main.go:49 | `return nodes[idx].Value.(int)` |
| `return expr, expr` | 1 | examples/uql/lexer/lexer.go:282 | `return tokens[0], tokens[1:]` |
| `return expr, ident` | 1 | examples/mos6502/lib/basic/codegen.go:467 | `return "L" + intToString(ctx.LabelCounter), ctx` |
| `return ident` | 136 | tests/lang-constructs/main.go:877 | `return m` |
| `return ident, call(), ident, ident` | 1 | examples/mos6502/lib/basic/basic.go:329 | `return code, len(asmLines), instrCount, lastByte` |
| `return ident, call(), string` | 1 | examples/mos6502/lib/basic/parser.go:663 | `return ExprNumber, parseNumber(args), ""` |
| `return ident, expr` | 3 | tests/lang-constructs/types/types.go:77 | `return plan, len(plan.Literals) - 1` |
| `return ident, ident` | 24 | examples/ast-demo/main.go:30 | `return nodes, idx` |
| `return ident, ident, ident` | 2 | examples/mos6502/lib/basic/parser.go:111 | `return lineNum, rest, true` |
| `return ident, int` | 1 | examples/uql/parser/parser.go:334 | `return resultAst, 0` |
| `return ident, int, call()` | 1 | examples/mos6502/lib/basic/parser.go:667 | `return ExprVariable, 0, toUpperChar(int(args[0]))` |
| `return ident, int, string` | 1 | examples/mos6502/lib/basic/parser.go:669 | `return ExprNumber, 0, ""` |
| `return ident.field` | 3 | examples/mos6502/lib/cpu/cpu.go:1343 | `return c.Halted` |
| `return ident.field, ident` | 1 | examples/mos6502/lib/assembler/assembler.go:550 | `return labels[i].Addr, true` |
| `return ident.field{...}, -expr` | 3 | examples/uql/parser/parser.go:269 | `return ast.AST{}, -1` |
| `return ident.field{...}, ident` | 1 | examples/uql/parser/parser.go:132 | `return ast.Where{Expr: expr, ResultTableExpr: lhs}, tokens` |
| `return ident{...}` | 11 | examples/mos6502/lib/basic/basic.go:31 | `return BasicState{` |
| `return ident{...}, []ident{...}` | 1 | examples/uql/lexer/lexer.go:280 | `return Token{}, []Token{}` |
| `return ident{...}, string` | 1 | examples/mos6502/lib/basic/parser.go:1072 | `return Condition{}, ""` |
| `return int` | 23 | tests/lang-constructs/main.go:50 | `return 5` |
| `return int, ident` | 2 | examples/mos6502/lib/assembler/assembler.go:554 | `return 0, false` |
| `return int, ident, ident` | 1 | examples/mos6502/lib/basic/parser.go:73 | `return 0, line, false` |
| `return int, int` | 1 | tests/lang-constructs/main.go:299 | `return 10, 20` |
| `return string` | 474 | examples/ast-demo/main.go:69 | `return "+"` |

## SliceExpr

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `ident[_:ident(ident) - int]` | 1 | examples/mos6502/cmd/graphic/main.go:191 | `program = program[:len(program)-1]` |
| `ident[_:int]` | 1 | tests/lang-constructs/main.go:327 | `c := a[:2]` |
| `ident[ident + int:_]` | 1 | examples/uql/parser/parser.go:89 | `rhsExpr, nextPos := parseExpression(tokens[i+1:], nextPrecedence)` |
| `ident[ident:_]` | 2 | examples/uql/parser/parser.go:124 | `tokens = tokens[i:]` |
| `ident[int:_]` | 2 | tests/lang-constructs/main.go:322 | `b := a[1:]` |
| `ident[int:int]` | 1 | tests/lang-constructs/main.go:332 | `d := a[1:2]` |

## SliceType

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `[](ident, ident)` | 1 | tests/lang-constructs/main.go:285 | `x := []func(int, int){` |
| `[][][]map[ident]ident` | 2 | tests/lang-constructs/main.go:1343 | `var tripleNestedSliceOfMaps [][][]map[string]int` |
| `[][]ident` | 5 | tests/lang-constructs/main.go:1136 | `var m [][]int` |
| `[][]map[ident]ident` | 5 | tests/lang-constructs/main.go:1280 | `var nestedSliceOfMaps [][]map[string]int` |
| `[]ident` | 242 | tests/lang-constructs/main.go:36 | `a []int` |
| `[]ident.field` | 24 | examples/uql/ast/ast.go:31 | `TableExpr       []lexer.Token` |
| `[]map[ident]ident` | 13 | tests/lang-constructs/main.go:1240 | `var sliceOfMaps []map[string]int` |

## SwitchStmt

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `switch ident` | 1 | examples/uql/lexer/lexer.go:215 | `switch b {` |
| `switch ident.field` | 1 | examples/uql/transform/postgresql.go:57 | `switch stmt.Type {` |
| `switch ident[ident].field` | 1 | examples/uql/transform/postgresql.go:35 | `switch uqlAst[i].Type {` |

## TypeAssert

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `ident.(ident)` | 29 | tests/lang-constructs/main.go:761 | `intVal := a.(int)` |

## TypeDecl

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `type ident []ident` | 1 | examples/uql/ast/ast.go:28 | `type AST []Statement` |
| `type ident ident` | 2 | tests/lang-constructs/types/types.go:19 | `type ExprKind int` |
| `type ident struct{...}` | 56 | tests/lang-constructs/main.go:35 | `type Composite struct {` |

## UnaryExpr

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `!call` | 23 | examples/mos6502/lib/assembler/assembler.go:186 | `if !IsHexDigit(bytes[i]) {` |
| `!expr` | 1 | tests/lang-constructs/main.go:89 | `if !(len(a) == 0) {` |
| `!ident` | 10 | tests/lang-constructs/main.go:275 | `if !b {` |
| `!ident.field` | 1 | tests/lang-constructs/main.go:628 | `if f2.ID == 2 && !f2.Required {` |
| `-expr` | 79 | examples/ast-demo/main.go:27 | `n.Left = -1` |
| `-ident` | 8 | examples/ast-demo/main.go:100 | `n = -n` |

## VarDecl

| Signature | Count | Example Location | Source |
|-----------|-------|------------------|--------|
| `var ident [][][]map[ident]ident = _` | 1 | tests/lang-constructs/main.go:1343 | `var tripleNestedSliceOfMaps [][][]map[string]int` |
| `var ident [][]ident = _` | 1 | tests/lang-constructs/main.go:1136 | `var m [][]int` |
| `var ident [][]map[ident]ident = _` | 1 | tests/lang-constructs/main.go:1280 | `var nestedSliceOfMaps [][]map[string]int` |
| `var ident []ident = _` | 6 | tests/lang-constructs/main.go:60 | `var a []int` |
| `var ident []map[ident]ident = _` | 1 | tests/lang-constructs/main.go:1240 | `var sliceOfMaps []map[string]int` |
| `var ident ident = -int` | 1 | examples/uql/lexer/tokenizer.go:54 | `var currentType int8 = -1` |
| `var ident ident = _` | 181 | tests/lang-constructs/main.go:339 | `var a int8` |
| `var ident ident.field = _` | 25 | tests/lang-constructs/main.go:559 | `var kind types.ExprKind` |
| `var ident interface{} = _` | 5 | tests/lang-constructs/main.go:741 | `var x interface{}` |
| `var ident map[ident][][]ident = _` | 1 | tests/lang-constructs/main.go:1321 | `var mapOfNestedSlices map[string][][]int` |
| `var ident map[ident][]ident = _` | 1 | tests/lang-constructs/main.go:1259 | `var mapOfSlices map[string][]int` |
| `var ident map[ident][]map[ident]ident = _` | 1 | tests/lang-constructs/main.go:1359 | `var intKeySliceOfMaps map[int][]map[string]int` |
| `var ident map[ident]ident = _` | 1 | tests/lang-constructs/main.go:937 | `var m map[string]int` |
| `var ident map[ident]map[ident][]ident = _` | 1 | tests/lang-constructs/main.go:1302 | `var nestedMapsWithSlice map[string]map[string][]int` |
