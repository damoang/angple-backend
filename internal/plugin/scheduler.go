package plugin

import (
	"sync"
	"time"
)

// ScheduledTask 등록된 주기적 작업
type ScheduledTask struct {
	Name       string
	PluginName string
	Interval   time.Duration
	Handler    func() error
	LastRun    time.Time
	NextRun    time.Time
	RunCount   int64
	LastError  error
}

// Scheduler 플러그인 스케줄러 (in-process)
type Scheduler struct {
	tasks  []*ScheduledTask
	mu     sync.RWMutex
	logger Logger
	stop   chan struct{}
	wg     sync.WaitGroup
}

// NewScheduler 스케줄러 생성
func NewScheduler(logger Logger) *Scheduler {
	return &Scheduler{
		tasks:  make([]*ScheduledTask, 0),
		logger: logger,
		stop:   make(chan struct{}),
	}
}

// Register 주기적 작업 등록
func (s *Scheduler) Register(pluginName, taskName string, interval time.Duration, handler func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tasks = append(s.tasks, &ScheduledTask{
		Name:       taskName,
		PluginName: pluginName,
		Interval:   interval,
		Handler:    handler,
		NextRun:    time.Now().Add(interval),
	})

	s.logger.Info("Scheduled task registered: %s/%s (every %s)", pluginName, taskName, interval)
}

// Unregister 플러그인의 모든 작업 해제
func (s *Scheduler) Unregister(pluginName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := s.tasks[:0]
	for _, t := range s.tasks {
		if t.PluginName != pluginName {
			filtered = append(filtered, t)
		}
	}
	s.tasks = filtered
}

// Start 스케줄러 시작 (백그라운드 goroutine)
func (s *Scheduler) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-s.stop:
				return
			case now := <-ticker.C:
				s.tick(now)
			}
		}
	}()
	s.logger.Info("Plugin scheduler started")
}

// Stop 스케줄러 중지
func (s *Scheduler) Stop() {
	close(s.stop)
	s.wg.Wait()
	s.logger.Info("Plugin scheduler stopped")
}

// tick 실행 대상 작업 체크 및 실행
func (s *Scheduler) tick(now time.Time) {
	s.mu.RLock()
	tasks := make([]*ScheduledTask, len(s.tasks))
	copy(tasks, s.tasks)
	s.mu.RUnlock()

	for _, task := range tasks {
		if now.Before(task.NextRun) {
			continue
		}

		s.logger.Info("Running scheduled task: %s/%s", task.PluginName, task.Name)

		if err := task.Handler(); err != nil {
			s.logger.Error("Scheduled task error [%s/%s]: %v", task.PluginName, task.Name, err)
			task.LastError = err
		} else {
			task.LastError = nil
		}

		task.LastRun = now
		task.NextRun = now.Add(task.Interval)
		task.RunCount++
	}
}

// GetTasks 등록된 작업 목록 조회 (모니터링용)
func (s *Scheduler) GetTasks() []ScheduledTaskInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ScheduledTaskInfo, 0, len(s.tasks))
	for _, t := range s.tasks {
		info := ScheduledTaskInfo{
			Name:       t.Name,
			PluginName: t.PluginName,
			Interval:   t.Interval.String(),
			LastRun:    t.LastRun,
			NextRun:    t.NextRun,
			RunCount:   t.RunCount,
		}
		if t.LastError != nil {
			errMsg := t.LastError.Error()
			info.LastError = &errMsg
		}
		result = append(result, info)
	}
	return result
}

// ScheduledTaskInfo 작업 정보 (JSON 응답용)
type ScheduledTaskInfo struct {
	Name       string    `json:"name"`
	PluginName string    `json:"plugin_name"`
	Interval   string    `json:"interval"`
	LastRun    time.Time `json:"last_run"`
	NextRun    time.Time `json:"next_run"`
	RunCount   int64     `json:"run_count"`
	LastError  *string   `json:"last_error,omitempty"`
}
