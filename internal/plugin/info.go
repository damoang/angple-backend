package plugin

// Overview 플러그인 전체 현황 요약
type Overview struct {
	TotalPlugins   int             `json:"total_plugins"`
	EnabledCount   int             `json:"enabled_count"`
	DisabledCount  int             `json:"disabled_count"`
	ErrorCount     int             `json:"error_count"`
	Plugins        []Summary `json:"plugins"`
	TotalRoutes    int             `json:"total_routes"`
	TotalSchedules int             `json:"total_schedules"`
}

// Summary 개별 플러그인 요약
type Summary struct {
	Name        string       `json:"name"`
	Version     string       `json:"version"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Status      PluginStatus `json:"status"`
	IsBuiltIn   bool         `json:"is_built_in"`
	Routes      int          `json:"routes"`
	HasSchedule bool         `json:"has_schedule"`
	HasEvents   bool         `json:"has_events"`
}

// GetOverview 플러그인 전체 현황 조회
func (m *Manager) GetOverview() Overview {
	m.mu.RLock()
	plugins := make([]*PluginInfo, 0, len(m.plugins))
	for _, info := range m.plugins {
		plugins = append(plugins, info)
	}
	m.mu.RUnlock()

	overview := Overview{}
	overview.TotalPlugins = len(plugins)

	routes := m.registry.GetRegisteredRoutes()
	schedules := m.scheduler.GetTasks()
	events := m.eventBus.GetSubscriptions()

	for _, info := range plugins {
		switch info.Status {
		case StatusEnabled:
			overview.EnabledCount++
		case StatusDisabled:
			overview.DisabledCount++
		case StatusError:
			overview.ErrorCount++
		}

		name := info.Manifest.Name
		routeCount := len(routes[name])
		overview.TotalRoutes += routeCount

		summary := Summary{
			Name:        name,
			Version:     info.Manifest.Version,
			Title:       info.Manifest.Title,
			Description: info.Manifest.Description,
			Status:      info.Status,
			IsBuiltIn:   info.IsBuiltIn,
			Routes:      routeCount,
		}

		// 스케줄 확인
		for _, task := range schedules {
			if task.PluginName == name {
				summary.HasSchedule = true
				overview.TotalSchedules++
				break
			}
		}

		// 이벤트 구독 확인
		for _, subscribers := range events {
			for _, sub := range subscribers {
				if sub == name {
					summary.HasEvents = true
					break
				}
			}
			if summary.HasEvents {
				break
			}
		}

		overview.Plugins = append(overview.Plugins, summary)
	}

	return overview
}

// Detail 플러그인 상세 정보 (capabilities 포함)
type Detail struct {
	Name        string              `json:"name"`
	Version     string              `json:"version"`
	Title       string              `json:"title"`
	Description string              `json:"description"`
	Author      string              `json:"author"`
	Homepage    string              `json:"homepage"`
	Status      PluginStatus        `json:"status"`
	IsBuiltIn   bool                `json:"is_built_in"`
	Routes      []RegisteredRoute   `json:"routes"`
	Schedules   []ScheduledTaskInfo `json:"schedules"`
	Events      []string            `json:"events"`
	Menus       []MenuConfig        `json:"menus"`
	Permissions []Permission        `json:"permissions"`
	Settings    []SettingConfig     `json:"settings"`
	Health      PluginHealth        `json:"health"`
}

// GetDetail 플러그인 상세 정보 조회
func (m *Manager) GetDetail(name string) *Detail {
	m.mu.RLock()
	info, exists := m.plugins[name]
	m.mu.RUnlock()

	if !exists {
		return nil
	}

	detail := &Detail{
		Name:        info.Manifest.Name,
		Version:     info.Manifest.Version,
		Title:       info.Manifest.Title,
		Description: info.Manifest.Description,
		Author:      info.Manifest.Author,
		Homepage:    info.Manifest.Homepage,
		Status:      info.Status,
		IsBuiltIn:   info.IsBuiltIn,
		Menus:       info.Manifest.Menus,
		Permissions: info.Manifest.Permissions,
		Settings:    info.Manifest.Settings,
		Health:      m.CheckHealth(name),
	}

	// 라우트
	detail.Routes = m.registry.GetPluginRoutes(name)
	if detail.Routes == nil {
		detail.Routes = []RegisteredRoute{}
	}

	// 스케줄
	for _, task := range m.scheduler.GetTasks() {
		if task.PluginName == name {
			detail.Schedules = append(detail.Schedules, task)
		}
	}
	if detail.Schedules == nil {
		detail.Schedules = []ScheduledTaskInfo{}
	}

	// 이벤트 구독 토픽 목록
	for topic, subs := range m.eventBus.GetSubscriptions() {
		for _, sub := range subs {
			if sub == name {
				detail.Events = append(detail.Events, topic)
				break
			}
		}
	}
	if detail.Events == nil {
		detail.Events = []string{}
	}

	// nil 슬라이스 방지
	if detail.Menus == nil {
		detail.Menus = []MenuConfig{}
	}
	if detail.Permissions == nil {
		detail.Permissions = []Permission{}
	}
	if detail.Settings == nil {
		detail.Settings = []SettingConfig{}
	}

	return detail
}
