package plugin

import (
	"errors"
	"testing"
)

// testLogger is a no-op logger for tests
type testLogger struct{}

func (l *testLogger) Debug(_ string, _ ...interface{}) {}
func (l *testLogger) Info(_ string, _ ...interface{})  {}
func (l *testLogger) Warn(_ string, _ ...interface{})  {}
func (l *testLogger) Error(_ string, _ ...interface{}) {}

func newTestHookManager() *HookManager {
	return NewHookManager(&testLogger{})
}

func TestHookActionRegisterAndDo(t *testing.T) {
	hm := newTestHookManager()

	called := false
	hm.Register("post.after_create", "test-plugin", func(ctx *HookContext) error {
		called = true
		if ctx.Event != "post.after_create" {
			t.Errorf("expected event post.after_create, got %s", ctx.Event)
		}
		if ctx.Input["title"] != "Hello" {
			t.Errorf("expected title Hello, got %v", ctx.Input["title"])
		}
		return nil
	}, 10)

	hm.Do("post.after_create", map[string]interface{}{"title": "Hello"})

	if !called {
		t.Error("action hook was not called")
	}
}

func TestHookFilterChaining(t *testing.T) {
	hm := newTestHookManager()

	// First filter: append " [filtered]"
	hm.RegisterFilter("post.content", "plugin-a", func(ctx *HookContext) error {
		content := ctx.Input["content"].(string)
		ctx.SetOutput(map[string]interface{}{
			"content": content + " [filtered]",
		})
		return nil
	}, 10)

	// Second filter: append " [sanitized]"
	hm.RegisterFilter("post.content", "plugin-b", func(ctx *HookContext) error {
		content := ctx.Input["content"].(string)
		ctx.SetOutput(map[string]interface{}{
			"content": content + " [sanitized]",
		})
		return nil
	}, 20)

	result := hm.Apply("post.content", map[string]interface{}{"content": "Hello"})

	expected := "Hello [filtered] [sanitized]"
	if result["content"] != expected {
		t.Errorf("expected %q, got %q", expected, result["content"])
	}
}

func TestHookPriorityOrder(t *testing.T) {
	hm := newTestHookManager()

	var order []string

	hm.Register("test.event", "plugin-c", func(_ *HookContext) error {
		order = append(order, "C")
		return nil
	}, 30)

	hm.Register("test.event", "plugin-a", func(_ *HookContext) error {
		order = append(order, "A")
		return nil
	}, 10)

	hm.Register("test.event", "plugin-b", func(_ *HookContext) error {
		order = append(order, "B")
		return nil
	}, 20)

	hm.Do("test.event", nil)

	if len(order) != 3 || order[0] != "A" || order[1] != "B" || order[2] != "C" {
		t.Errorf("expected [A B C], got %v", order)
	}
}

func TestHookErrorIsolation(t *testing.T) {
	hm := newTestHookManager()

	secondCalled := false

	hm.Register("test.event", "bad-plugin", func(_ *HookContext) error {
		return errors.New("something broke")
	}, 10)

	hm.Register("test.event", "good-plugin", func(_ *HookContext) error {
		secondCalled = true
		return nil
	}, 20)

	hm.Do("test.event", nil)

	if !secondCalled {
		t.Error("second hook should still be called after first hook error")
	}
}

func TestHookFilterErrorSkips(t *testing.T) {
	hm := newTestHookManager()

	hm.RegisterFilter("test.filter", "bad-plugin", func(ctx *HookContext) error {
		ctx.SetOutput(map[string]interface{}{"value": "bad"})
		return errors.New("filter error")
	}, 10)

	hm.RegisterFilter("test.filter", "good-plugin", func(ctx *HookContext) error {
		content := ctx.Input["value"].(string)
		ctx.SetOutput(map[string]interface{}{"value": content + " [ok]"})
		return nil
	}, 20)

	result := hm.Apply("test.filter", map[string]interface{}{"value": "original"})

	// bad-plugin errored, so its output is skipped; good-plugin receives "original"
	expected := "original [ok]"
	if result["value"] != expected {
		t.Errorf("expected %q, got %q", expected, result["value"])
	}
}

func TestHookUnregister(t *testing.T) {
	hm := newTestHookManager()

	called := false
	hm.Register("test.event", "removable", func(_ *HookContext) error {
		called = true
		return nil
	}, 10)

	hm.Unregister("removable")
	hm.Do("test.event", nil)

	if called {
		t.Error("hook should not be called after unregister")
	}
}

func TestHookDoNoHandlers(_ *testing.T) {
	hm := newTestHookManager()
	// Should not panic
	hm.Do("nonexistent.event", map[string]interface{}{"key": "value"})
}

func TestHookApplyNoHandlers(t *testing.T) {
	hm := newTestHookManager()
	data := map[string]interface{}{"key": "value"}
	result := hm.Apply("nonexistent.event", data)

	if result["key"] != "value" {
		t.Error("Apply with no handlers should return original data")
	}
}
