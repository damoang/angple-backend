package plugin

import (
	"sync"
	"time"
)

// Event 플러그인 간 이벤트
type Event struct {
	Topic     string                 `json:"topic"`
	Source    string                 `json:"source"` // 발행 플러그인
	Payload   map[string]interface{} `json:"payload"`
	Timestamp time.Time              `json:"timestamp"`
}

// EventHandler 이벤트 핸들러 함수
type EventHandler func(event Event)

type subscription struct {
	pluginName string
	handler    EventHandler
}

// EventBus 플러그인 간 이벤트 발행/구독 시스템
type EventBus struct {
	subscribers map[string][]subscription // topic -> handlers
	mu          sync.RWMutex
	logger      Logger
}

// NewEventBus 생성자
func NewEventBus(logger Logger) *EventBus {
	return &EventBus{
		subscribers: make(map[string][]subscription),
		logger:      logger,
	}
}

// Subscribe 토픽 구독
func (eb *EventBus) Subscribe(pluginName, topic string, handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.subscribers[topic] = append(eb.subscribers[topic], subscription{
		pluginName: pluginName,
		handler:    handler,
	})
	eb.logger.Debug("Plugin %s subscribed to topic: %s", pluginName, topic)
}

// Unsubscribe 플러그인의 모든 구독 해제
func (eb *EventBus) Unsubscribe(pluginName string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	for topic, subs := range eb.subscribers {
		var remaining []subscription
		for _, s := range subs {
			if s.pluginName != pluginName {
				remaining = append(remaining, s)
			}
		}
		if len(remaining) == 0 {
			delete(eb.subscribers, topic)
		} else {
			eb.subscribers[topic] = remaining
		}
	}
}

// Publish 이벤트 발행 (동기: 모든 핸들러 순차 실행)
func (eb *EventBus) Publish(source, topic string, payload map[string]interface{}) {
	eb.mu.RLock()
	subs := make([]subscription, len(eb.subscribers[topic]))
	copy(subs, eb.subscribers[topic])
	eb.mu.RUnlock()

	if len(subs) == 0 {
		return
	}

	event := Event{
		Topic:     topic,
		Source:    source,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	for _, s := range subs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					eb.logger.Error("Event handler panicked [%s/%s → %s]: %v", source, topic, s.pluginName, r)
				}
			}()
			s.handler(event)
		}()
	}
}

// PublishAsync 이벤트 비동기 발행
func (eb *EventBus) PublishAsync(source, topic string, payload map[string]interface{}) {
	go eb.Publish(source, topic, payload)
}

// GetSubscriptions 구독 현황 조회
func (eb *EventBus) GetSubscriptions() map[string][]string {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	result := make(map[string][]string)
	for topic, subs := range eb.subscribers {
		for _, s := range subs {
			result[topic] = append(result[topic], s.pluginName)
		}
	}
	return result
}
