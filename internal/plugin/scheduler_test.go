package plugin

import (
	"errors"
	"testing"
	"time"
)

func TestScheduler_RegisterAndGetTasks(t *testing.T) {
	logger := NewDefaultLogger("test")
	s := NewScheduler(logger)

	s.Register("test-plugin", "cleanup", time.Hour, func() error { return nil })
	s.Register("test-plugin", "report", 24*time.Hour, func() error { return nil })

	tasks := s.GetTasks()
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].Name != "cleanup" {
		t.Errorf("expected cleanup, got %s", tasks[0].Name)
	}
}

func TestScheduler_Unregister(t *testing.T) {
	logger := NewDefaultLogger("test")
	s := NewScheduler(logger)

	s.Register("plugin-a", "task1", time.Hour, func() error { return nil })
	s.Register("plugin-b", "task2", time.Hour, func() error { return nil })

	s.Unregister("plugin-a")

	tasks := s.GetTasks()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task after unregister, got %d", len(tasks))
	}
	if tasks[0].PluginName != "plugin-b" {
		t.Errorf("expected plugin-b, got %s", tasks[0].PluginName)
	}
}

func TestScheduler_Tick(t *testing.T) {
	logger := NewDefaultLogger("test")
	s := NewScheduler(logger)

	var count int
	s.Register("test-plugin", "counter", time.Millisecond, func() error {
		count++
		return nil
	})

	// Force NextRun to past
	s.tasks[0].NextRun = time.Now().Add(-time.Second)

	s.tick(time.Now())

	if count != 1 {
		t.Errorf("expected handler called once, got %d", count)
	}
	if s.tasks[0].RunCount != 1 {
		t.Errorf("expected RunCount 1, got %d", s.tasks[0].RunCount)
	}
}

func TestScheduler_TickError(t *testing.T) {
	logger := NewDefaultLogger("test")
	s := NewScheduler(logger)

	s.Register("test-plugin", "failing", time.Millisecond, func() error {
		return errors.New("db down")
	})
	s.tasks[0].NextRun = time.Now().Add(-time.Second)

	s.tick(time.Now())

	if s.tasks[0].LastError == nil {
		t.Error("expected LastError to be set")
	}

	tasks := s.GetTasks()
	if tasks[0].LastError == nil || *tasks[0].LastError != "db down" {
		t.Errorf("expected error message 'db down', got %v", tasks[0].LastError)
	}
}

func TestScheduler_SkipNotReady(t *testing.T) {
	logger := NewDefaultLogger("test")
	s := NewScheduler(logger)

	var count int
	s.Register("test-plugin", "future", time.Hour, func() error {
		count++
		return nil
	})

	s.tick(time.Now())

	if count != 0 {
		t.Error("expected handler NOT called for future task")
	}
}
