package plugin

import (
	"fmt"
	"sync"

	"github.com/gin-gonic/gin"
)

// Registry 플러그인 라우트 레지스트리
type Registry struct {
	router           gin.IRouter
	pluginRouters    map[string]gin.IRouter
	registeredAPIs   map[string][]RegisteredRoute
	routesRegistered map[string]bool // Gin 라우트는 한번 등록하면 제거 불가
	mu               sync.RWMutex
}

// RegisteredRoute 등록된 라우트 정보
type RegisteredRoute struct {
	Method  string
	Path    string
	Handler string
	Auth    string
}

// NewRegistry 새 레지스트리 생성
func NewRegistry() *Registry {
	return &Registry{
		pluginRouters:    make(map[string]gin.IRouter),
		registeredAPIs:   make(map[string][]RegisteredRoute),
		routesRegistered: make(map[string]bool),
	}
}

// SetRouter 기본 라우터 설정 (main.go에서 호출)
func (r *Registry) SetRouter(router gin.IRouter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.router = router
}

// GetPluginRouter 플러그인용 라우터 그룹 반환
// 경로: /api/plugins/{plugin-name}
func (r *Registry) GetPluginRouter(pluginName string) gin.IRouter {
	r.mu.Lock()
	defer r.mu.Unlock()

	if router, exists := r.pluginRouters[pluginName]; exists {
		return router
	}

	if r.router == nil {
		return nil
	}

	// 플러그인 전용 라우트 그룹 생성
	path := fmt.Sprintf("/api/plugins/%s", pluginName)
	pluginRouter := r.router.Group(path)
	r.pluginRouters[pluginName] = pluginRouter

	return pluginRouter
}

// RegisterPlugin 플러그인 라우트 등록
func (r *Registry) RegisterPlugin(info *PluginInfo) {
	if info.Instance == nil {
		return
	}

	// 이미 라우트 등록된 플러그인이면 스킵 (Gin은 라우트 재등록 시 panic)
	r.mu.RLock()
	alreadyRegistered := r.routesRegistered[info.Manifest.Name]
	r.mu.RUnlock()
	if alreadyRegistered {
		return
	}

	pluginRouter := r.GetPluginRouter(info.Manifest.Name)
	if pluginRouter == nil {
		return
	}

	// 플러그인이 자체적으로 라우트 등록
	info.Instance.RegisterRoutes(pluginRouter)

	// 매니페스트에 정의된 라우트 기록
	r.mu.Lock()
	r.routesRegistered[info.Manifest.Name] = true
	routes := make([]RegisteredRoute, 0, len(info.Manifest.Routes))
	for _, route := range info.Manifest.Routes {
		routes = append(routes, RegisteredRoute{
			Method:  route.Method,
			Path:    route.Path,
			Handler: route.Handler,
			Auth:    route.Auth,
		})
	}
	r.registeredAPIs[info.Manifest.Name] = routes
	r.mu.Unlock()
}

// UnregisterPlugin 플러그인 라우트 해제
func (r *Registry) UnregisterPlugin(pluginName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.pluginRouters, pluginName)
	delete(r.registeredAPIs, pluginName)
}

// GetRegisteredRoutes 등록된 모든 라우트 조회
func (r *Registry) GetRegisteredRoutes() map[string][]RegisteredRoute {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string][]RegisteredRoute)
	for name, routes := range r.registeredAPIs {
		result[name] = append([]RegisteredRoute{}, routes...)
	}
	return result
}

// GetPluginRoutes 특정 플러그인의 라우트 조회
func (r *Registry) GetPluginRoutes(pluginName string) []RegisteredRoute {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if routes, exists := r.registeredAPIs[pluginName]; exists {
		return append([]RegisteredRoute{}, routes...)
	}
	return nil
}

// HasPlugin 플러그인 등록 여부 확인
func (r *Registry) HasPlugin(pluginName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.pluginRouters[pluginName]
	return exists
}
