package mysql

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// toolCallModel 是只返回固定工具调用的假模型，用于把 mysql_exec 接到 ToolsNode 上。
type toolCallModel struct {
	args string
}

// Generate 返回带工具调用的助手消息。
func (m *toolCallModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID:       "call-1",
			Function: schema.FunctionCall{Name: "mysql_exec", Arguments: m.args},
		}},
	}, nil
}

// Stream 本测试不走流式。
func (m *toolCallModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("本测试不支持流式")
}

// WithTools 返回自身：本模型已经写死了要调用的工具。
func (m *toolCallModel) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

// memCheckPointStore 是 compose.CheckPointStore 的最小内存实现，让打断与恢复能跑完整流程。
type memCheckPointStore struct {
	mu sync.Mutex
	m  map[string][]byte
}

// Get 取出 checkpoint。
func (s *memCheckPointStore) Get(_ context.Context, id string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[id]
	return v, ok, nil
}

// Set 写入 checkpoint。
func (s *memCheckPointStore) Set(_ context.Context, id string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[id] = data
	return nil
}

// newExecGraph 把 mysql_exec 放进「假模型产出工具调用 → ToolsNode」的最小图里，
// 让打断与恢复走真实的编排链路，而不是只断言错误类型。
func newExecGraph(t *testing.T, tl tool.InvokableTool, args string) compose.Runnable[[]*schema.Message, []*schema.Message] {
	t.Helper()

	toolsNode, err := compose.NewToolNode(context.Background(), &compose.ToolsNodeConfig{
		Tools: []tool.BaseTool{tl},
	})
	if err != nil {
		t.Fatalf("创建 ToolsNode 失败: %v", err)
	}

	g := compose.NewGraph[[]*schema.Message, []*schema.Message]()
	if err := g.AddChatModelNode("caller", &toolCallModel{args: args}); err != nil {
		t.Fatalf("添加模型节点失败: %v", err)
	}
	if err := g.AddToolsNode("tools", toolsNode); err != nil {
		t.Fatalf("添加 tools 节点失败: %v", err)
	}
	if err := g.AddEdge(compose.START, "caller"); err != nil {
		t.Fatalf("添加边失败: %v", err)
	}
	if err := g.AddEdge("caller", "tools"); err != nil {
		t.Fatalf("添加边失败: %v", err)
	}
	if err := g.AddEdge("tools", compose.END); err != nil {
		t.Fatalf("添加边失败: %v", err)
	}

	compiled, err := g.Compile(context.Background(),
		compose.WithCheckPointStore(&memCheckPointStore{m: make(map[string][]byte)}),
	)
	if err != nil {
		t.Fatalf("编译图失败: %v", err)
	}
	return compiled
}

// interruptForDestructive 首次执行图，返回打断点；断言此时语句尚未下发。
func interruptForDestructive(t *testing.T, graph compose.Runnable[[]*schema.Message, []*schema.Message],
	fx *fakeFixture, checkPointID, sqlText string) *compose.InterruptCtx {
	t.Helper()

	input := []*schema.Message{schema.UserMessage("执行危险语句")}
	_, err := graph.Invoke(context.Background(), input, compose.WithCheckPointID(checkPointID))
	if err == nil {
		t.Fatalf("首次执行应被打断")
	}

	interruptInfo, isInterrupt := compose.ExtractInterruptInfo(err)
	if !isInterrupt {
		t.Fatalf("期望打断错误，实际: %v", err)
	}
	if len(interruptInfo.InterruptContexts) != 1 {
		t.Fatalf("期望 1 个打断点，实际 %d 个", len(interruptInfo.InterruptContexts))
	}

	rootCause := interruptInfo.InterruptContexts[0]
	if !rootCause.IsRootCause {
		t.Errorf("打断点应为根因")
	}
	if !strings.HasSuffix(rootCause.Address.String(), "node:tools;tool:mysql_exec:call-1") {
		t.Errorf("打断点地址不符，实际 %s", rootCause.Address.String())
	}

	info, ok := rootCause.Info.(destructiveInterruptInfo)
	if !ok {
		t.Fatalf("打断信息类型不符: %#v", rootCause.Info)
	}
	if info.Statement != sqlText {
		t.Errorf("打断信息应带上待确认语句，实际 %q", info.Statement)
	}
	if strings.Contains(info.Statement, "pass") {
		t.Errorf("打断信息不应带 DSN：%q", info.Statement)
	}

	if sent := fx.executedSQL(); len(sent) != 0 {
		t.Errorf("打断期间不应执行语句，实际下发: %v", sent)
	}

	return rootCause
}

// TestMysqlExecInterruptConfirm 校验 interrupt 模式下 DROP 会先挂起，用户确认后才执行。
func TestMysqlExecInterruptConfirm(t *testing.T) {
	const sqlText = "DROP TABLE t"

	fx := &fakeFixture{rowsAffected: 1}
	tl, err := newMysqlExecTool(newFakeOpen(t, fakeDSN, fx), ModeDangerousInterrupt)
	if err != nil {
		t.Fatalf("创建工具失败: %v", err)
	}

	args := `{"dsn":"` + fakeDSN + `","sql":"` + sqlText + `"}`
	graph := newExecGraph(t, tl, args)

	const checkPointID = "mysql-exec-interrupt-confirm"
	rootCause := interruptForDestructive(t, graph, fx, checkPointID, sqlText)

	// 用户确认后恢复执行。
	ctx := compose.ResumeWithData(context.Background(), rootCause.ID, true)
	out, err := graph.Invoke(ctx, []*schema.Message{schema.UserMessage("执行危险语句")}, compose.WithCheckPointID(checkPointID))
	if err != nil {
		t.Fatalf("恢复执行失败: %v", err)
	}
	if len(out) != 1 || !strings.Contains(out[0].Content, "rows_affected") {
		t.Errorf("期望返回受影响行数，实际: %v", out)
	}

	sent := fx.executedSQL()
	if len(sent) != 1 || sent[0] != sqlText {
		t.Errorf("确认后应执行语句，实际下发: %v", sent)
	}
}

// TestMysqlExecInterruptCancel 校验用户拒绝后不执行，且整图正常结束让模型能转达结果。
func TestMysqlExecInterruptCancel(t *testing.T) {
	const sqlText = "TRUNCATE TABLE t"

	fx := &fakeFixture{rowsAffected: 1}
	tl, err := newMysqlExecTool(newFakeOpen(t, fakeDSN, fx), ModeDangerousInterrupt)
	if err != nil {
		t.Fatalf("创建工具失败: %v", err)
	}

	args := `{"dsn":"` + fakeDSN + `","sql":"` + sqlText + `"}`
	graph := newExecGraph(t, tl, args)

	const checkPointID = "mysql-exec-interrupt-cancel"
	rootCause := interruptForDestructive(t, graph, fx, checkPointID, sqlText)

	ctx := compose.ResumeWithData(context.Background(), rootCause.ID, false)
	out, err := graph.Invoke(ctx, []*schema.Message{schema.UserMessage("执行危险语句")}, compose.WithCheckPointID(checkPointID))
	if err != nil {
		t.Fatalf("取消不应让整图失败（模型需要拿到结果），实际: %v", err)
	}
	if len(out) != 1 || !strings.Contains(out[0].Content, `"canceled":true`) {
		t.Errorf("期望返回 canceled 结果，实际: %v", out)
	}

	if sent := fx.executedSQL(); len(sent) != 0 {
		t.Errorf("用户拒绝后不应执行语句，实际下发: %v", sent)
	}
}

// TestParseDangerousMode 校验配置值解析与兜底。
func TestParseDangerousMode(t *testing.T) {
	cases := []struct {
		in   string
		want DangerousStatementMode
	}{
		{"deny", ModeDangerousDeny},
		{"interrupt", ModeDangerousInterrupt},
		{"", ModeDangerousDeny},
		{"Interrupt", ModeDangerousDeny},
		{"unknown", ModeDangerousDeny},
	}

	for _, c := range cases {
		if got := parseDangerousMode(c.in); got != c.want {
			t.Errorf("parseDangerousMode(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

// TestNewMysqlExecToolFromConfig 校验默认（配置缺失）时工具可以正常构建。
func TestNewMysqlExecToolFromConfig(t *testing.T) {
	tl, err := NewMysqlExecTool(context.Background())
	if err != nil {
		t.Fatalf("构建工具失败: %v", err)
	}

	info, err := tl.Info(context.Background())
	if err != nil {
		t.Fatalf("读取工具信息失败: %v", err)
	}
	if info.Name != "mysql_exec" {
		t.Errorf("工具名不符: %s", info.Name)
	}
}
