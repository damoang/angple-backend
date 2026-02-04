package plugin

import (
	"sort"
	"sync"
)

// HookType Action(반환값 없음) vs Filter(데이터 변환)
type HookType int

const (
	HookTypeAction HookType = iota
	HookTypeFilter
)

// HookContext Hook 핸들러에 전달되는 컨텍스트
type HookContext struct {
	Event  string
	Input  map[string]interface{}
	output map[string]interface{}
}

// SetOutput 출력 데이터 설정 (Filter Hook에서 사용)
func (c *HookContext) SetOutput(data map[string]interface{}) {
	c.output = data
}

// GetOutput 출력 데이터 반환
func (c *HookContext) GetOutput() map[string]interface{} {
	if c.output != nil {
		return c.output
	}
	return c.Input
}

// HookHandler Hook 핸들러 함수
type HookHandler func(ctx *HookContext) error

// hookEntry 등록된 Hook 정보
type hookEntry struct {
	pluginName string
	handler    HookHandler
	priority   int
	hookType   HookType
}

// HookManager Hook 등록/실행 관리자 (thread-safe)
type HookManager struct {
	hooks  map[string][]hookEntry
	mu     sync.RWMutex
	logger Logger
}

// NewHookManager 새 HookManager 생성
func NewHookManager(logger Logger) *HookManager {
	return &HookManager{
		hooks:  make(map[string][]hookEntry),
		logger: logger,
	}
}

// Register Action Hook 등록
func (hm *HookManager) Register(event string, pluginName string, handler HookHandler, priority int) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.hooks[event] = append(hm.hooks[event], hookEntry{
		pluginName: pluginName,
		handler:    handler,
		priority:   priority,
		hookType:   HookTypeAction,
	})
	hm.sortHooks(event)
}

// RegisterFilter Filter Hook 등록
func (hm *HookManager) RegisterFilter(event string, pluginName string, handler HookHandler, priority int) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.hooks[event] = append(hm.hooks[event], hookEntry{
		pluginName: pluginName,
		handler:    handler,
		priority:   priority,
		hookType:   HookTypeFilter,
	})
	hm.sortHooks(event)
}

// Do Action Hook 실행 (에러 로깅만, 블로킹 안 함)
func (hm *HookManager) Do(event string, data map[string]interface{}) {
	hm.mu.RLock()
	entries := make([]hookEntry, len(hm.hooks[event]))
	copy(entries, hm.hooks[event])
	hm.mu.RUnlock()

	for _, entry := range entries {
		ctx := &HookContext{
			Event: event,
			Input: data,
		}
		if err := entry.handler(ctx); err != nil {
			hm.logger.Error("Hook error [%s] plugin=%s: %v", event, entry.pluginName, err)
		}
	}
}

// Apply Filter Hook 실행 (결과 반환, 체이닝)
func (hm *HookManager) Apply(event string, data map[string]interface{}) map[string]interface{} {
	hm.mu.RLock()
	entries := make([]hookEntry, len(hm.hooks[event]))
	copy(entries, hm.hooks[event])
	hm.mu.RUnlock()

	current := data
	for _, entry := range entries {
		ctx := &HookContext{
			Event: event,
			Input: current,
		}
		if err := entry.handler(ctx); err != nil {
			hm.logger.Error("Filter error [%s] plugin=%s: %v", event, entry.pluginName, err)
			continue
		}
		current = ctx.GetOutput()
	}
	return current
}

// Unregister 특정 플러그인의 모든 Hook 해제
func (hm *HookManager) Unregister(pluginName string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	for event, entries := range hm.hooks {
		filtered := entries[:0]
		for _, e := range entries {
			if e.pluginName != pluginName {
				filtered = append(filtered, e)
			}
		}
		hm.hooks[event] = filtered
	}
}

// sortHooks priority 기준 오름차순 정렬 (낮은 priority가 먼저 실행)
// 호출자가 lock을 보유해야 함
func (hm *HookManager) sortHooks(event string) {
	sort.SliceStable(hm.hooks[event], func(i, j int) bool {
		return hm.hooks[event][i].priority < hm.hooks[event][j].priority
	})
}
