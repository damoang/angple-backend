package plugin

import (
	"sync"
)

// PluginFactory 플러그인 인스턴스 생성 팩토리 함수
type PluginFactory func() Plugin

// PluginRegistration 플러그인 등록 정보
type PluginRegistration struct {
	Factory  PluginFactory
	Manifest *PluginManifest
}

var (
	// 내장 플러그인 팩토리 레지스트리
	builtInFactories = make(map[string]PluginRegistration)
	factoryMu        sync.RWMutex
)

// RegisterFactory 내장 플러그인 팩토리 등록
// 각 플러그인 패키지의 init()에서 호출됨
func RegisterFactory(name string, factory PluginFactory, manifest *PluginManifest) {
	factoryMu.Lock()
	defer factoryMu.Unlock()

	builtInFactories[name] = PluginRegistration{
		Factory:  factory,
		Manifest: manifest,
	}
}

// GetRegisteredFactories 등록된 모든 플러그인 팩토리 반환
func GetRegisteredFactories() map[string]PluginRegistration {
	factoryMu.RLock()
	defer factoryMu.RUnlock()

	result := make(map[string]PluginRegistration)
	for name, reg := range builtInFactories {
		result[name] = reg
	}
	return result
}

// GetFactory 특정 플러그인 팩토리 반환
func GetFactory(name string) (PluginFactory, *PluginManifest, bool) {
	factoryMu.RLock()
	defer factoryMu.RUnlock()

	if reg, exists := builtInFactories[name]; exists {
		return reg.Factory, reg.Manifest, true
	}
	return nil, nil, false
}

// IsRegistered 플러그인 등록 여부 확인
func IsRegistered(name string) bool {
	factoryMu.RLock()
	defer factoryMu.RUnlock()

	_, exists := builtInFactories[name]
	return exists
}

// GetRegisteredNames 등록된 플러그인 이름 목록 반환
func GetRegisteredNames() []string {
	factoryMu.RLock()
	defer factoryMu.RUnlock()

	names := make([]string, 0, len(builtInFactories))
	for name := range builtInFactories {
		names = append(names, name)
	}
	return names
}
