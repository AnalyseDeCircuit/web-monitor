package alerts

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// generateID 生成唯一 ID
func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), hex.EncodeToString(b))
}

// Manager 告警管理器
type Manager struct {
	mu sync.RWMutex

	// 持久化数据
	config  AlertConfig
	rules   map[string]*AlertRule
	history []AlertEvent

	// 运行时状态
	states map[string]*RuleState

	// 通知器
	notifier *Notifier

	// 配置
	dataDir        string
	maxHistorySize int
}

// NewManager 创建告警管理器
func NewManager(dataDir string) *Manager {
	m := &Manager{
		rules:          make(map[string]*AlertRule),
		history:        make([]AlertEvent, 0),
		states:         make(map[string]*RuleState),
		dataDir:        dataDir,
		maxHistorySize: 1000, // 最多保留 1000 条历史
	}
	m.notifier = NewNotifier(m)
	return m
}

// Initialize 初始化告警管理器
func (m *Manager) Initialize() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 加载配置
	if err := m.loadConfig(); err != nil {
		log.Printf("[Alerts] Failed to load config: %v, using defaults", err)
		m.config = AlertConfig{
			Enabled:             false,
			NotifyOnResolved:    true,
			GlobalSilencePeriod: "5m",
			Channels:            []NotificationChannel{},
		}
	}

	// 加载规则
	if err := m.loadRules(); err != nil {
		log.Printf("[Alerts] Failed to load rules: %v, initializing with builtins", err)
		m.initBuiltinRules()
	}

	// 加载历史
	if err := m.loadHistory(); err != nil {
		log.Printf("[Alerts] Failed to load history: %v", err)
	}

	// 初始化状态
	for _, rule := range m.rules {
		m.states[rule.ID] = &RuleState{RuleID: rule.ID}
	}

	log.Printf("[Alerts] Initialized with %d rules, %d history events", len(m.rules), len(m.history))
	return nil
}

// initBuiltinRules 初始化内置规则
func (m *Manager) initBuiltinRules() {
	for _, builtin := range BuiltinRules {
		rule := builtin
		rule.CreatedAt = time.Now()
		rule.UpdatedAt = time.Now()
		m.rules[rule.ID] = &rule
	}
}

// ============================================================================
//  配置管理
// ============================================================================

// GetConfig 获取配置
func (m *Manager) GetConfig() AlertConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// UpdateConfig 更新配置
func (m *Manager) UpdateConfig(cfg AlertConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config = cfg
	return m.saveConfig()
}

// ============================================================================
//  规则管理
// ============================================================================

// GetRules 获取所有规则
func (m *Manager) GetRules() []AlertRule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rules := make([]AlertRule, 0, len(m.rules))
	for _, r := range m.rules {
		rules = append(rules, *r)
	}
	// 按 ID 排序
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].ID < rules[j].ID
	})
	return rules
}

// GetRule 获取单个规则
func (m *Manager) GetRule(id string) (*AlertRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rule, ok := m.rules[id]
	if !ok {
		return nil, ErrRuleNotFound
	}
	ruleCopy := *rule
	return &ruleCopy, nil
}

// CreateRule 创建规则
func (m *Manager) CreateRule(rule *AlertRule) error {
	if err := ValidateRule(rule); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.rules[rule.ID]; exists {
		return ErrRuleExists
	}

	now := time.Now()
	rule.CreatedAt = now
	rule.UpdatedAt = now
	rule.Builtin = false

	m.rules[rule.ID] = rule
	m.states[rule.ID] = &RuleState{RuleID: rule.ID}

	return m.saveRules()
}

// UpdateRule 更新规则
func (m *Manager) UpdateRule(id string, updates *AlertRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rule, ok := m.rules[id]
	if !ok {
		return ErrRuleNotFound
	}

	// 保留原始的一些属性
	updates.ID = id
	updates.Builtin = rule.Builtin
	updates.CreatedAt = rule.CreatedAt
	updates.UpdatedAt = time.Now()

	if err := ValidateRule(updates); err != nil {
		return err
	}

	m.rules[id] = updates

	// 如果禁用了规则，重置状态
	if !updates.Enabled && m.states[id] != nil {
		m.states[id] = &RuleState{RuleID: id}
	}

	return m.saveRules()
}

// DeleteRule 删除规则
func (m *Manager) DeleteRule(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rule, ok := m.rules[id]
	if !ok {
		return ErrRuleNotFound
	}

	if rule.Builtin {
		// 内置规则不删除，而是重置为默认状态
		builtinRule := GetBuiltinRule(id)
		if builtinRule != nil {
			m.rules[id] = builtinRule
			return m.saveRules()
		}
	}

	delete(m.rules, id)
	delete(m.states, id)

	return m.saveRules()
}

// EnableRule 启用规则
func (m *Manager) EnableRule(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rule, ok := m.rules[id]
	if !ok {
		return ErrRuleNotFound
	}

	rule.Enabled = true
	rule.UpdatedAt = time.Now()

	return m.saveRules()
}

// DisableRule 禁用规则
func (m *Manager) DisableRule(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rule, ok := m.rules[id]
	if !ok {
		return ErrRuleNotFound
	}

	rule.Enabled = false
	rule.UpdatedAt = time.Now()

	// 重置状态
	if state, exists := m.states[id]; exists {
		// 如果正在触发，创建恢复事件
		if state.IsFiring {
			m.resolveAlertLocked(id, state.LastValue)
		}
		m.states[id] = &RuleState{RuleID: id}
	}

	return m.saveRules()
}

// EnablePreset 启用预设组
func (m *Manager) EnablePreset(presetID string) error {
	preset := GetPreset(presetID)
	if preset == nil {
		return fmt.Errorf("preset not found: %s", presetID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, ruleID := range preset.RuleIDs {
		if rule, ok := m.rules[ruleID]; ok {
			rule.Enabled = true
			rule.UpdatedAt = time.Now()
		}
	}

	return m.saveRules()
}

// DisableAllRules 禁用所有规则
func (m *Manager) DisableAllRules() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, rule := range m.rules {
		rule.Enabled = false
		rule.UpdatedAt = time.Now()

		// 重置状态
		if state, exists := m.states[id]; exists && state.IsFiring {
			m.resolveAlertLocked(id, state.LastValue)
		}
		m.states[id] = &RuleState{RuleID: id}
	}

	return m.saveRules()
}

// ============================================================================
//  告警检查
// ============================================================================

// Check 检查指标并触发告警
func (m *Manager) Check(metrics map[string]float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.config.Enabled {
		return
	}

	now := time.Now()

	for id, rule := range m.rules {
		if !rule.Enabled {
			continue
		}

		value, ok := metrics[rule.Metric]
		if !ok {
			continue
		}

		state := m.states[id]
		if state == nil {
			state = &RuleState{RuleID: id}
			m.states[id] = state
		}

		state.LastValue = value
		triggered := evaluateCondition(value, rule.Operator, rule.Threshold)

		if triggered {
			if state.FirstTriggered == nil {
				// 首次触发，记录时间
				state.FirstTriggered = &now
			} else {
				// 检查是否超过持续时间
				duration, _ := time.ParseDuration(rule.Duration)
				if duration == 0 {
					duration = time.Minute // 默认 1 分钟
				}

				if now.Sub(*state.FirstTriggered) >= duration && !state.IsFiring {
					// 触发告警
					m.fireAlertLocked(rule, value)
					state.IsFiring = true
				}
			}
		} else {
			// 条件不再满足
			if state.IsFiring {
				// 恢复告警
				m.resolveAlertLocked(id, value)
			}
			// 重置状态
			state.FirstTriggered = nil
			state.IsFiring = false
		}
	}
}

// fireAlertLocked 触发告警（需持有锁）
func (m *Manager) fireAlertLocked(rule *AlertRule, value float64) {
	event := AlertEvent{
		ID:        generateID(),
		RuleID:    rule.ID,
		RuleName:  rule.Name,
		Metric:    rule.Metric,
		Status:    StatusFiring,
		Severity:  rule.Severity,
		Value:     value,
		Threshold: rule.Threshold,
		Operator:  rule.Operator,
		Message:   formatAlertMessage(rule, value, true),
		FiredAt:   time.Now(),
		Notified:  false,
	}

	m.history = append(m.history, event)
	m.states[rule.ID].FiringEventID = event.ID

	// 裁剪历史
	if len(m.history) > m.maxHistorySize {
		m.history = m.history[len(m.history)-m.maxHistorySize:]
	}

	// 异步保存和通知
	go func() {
		m.saveHistory()
		m.notifier.Notify(&event)
	}()

	log.Printf("[Alerts] FIRING: %s (%.2f %s %.2f)", rule.Name, value, rule.Operator, rule.Threshold)
}

// resolveAlertLocked 恢复告警（需持有锁）
func (m *Manager) resolveAlertLocked(ruleID string, value float64) {
	state := m.states[ruleID]
	if state == nil || !state.IsFiring {
		return
	}

	rule := m.rules[ruleID]
	if rule == nil {
		return
	}

	now := time.Now()

	// 查找并更新事件
	for i := len(m.history) - 1; i >= 0; i-- {
		if m.history[i].ID == state.FiringEventID {
			m.history[i].Status = StatusResolved
			m.history[i].ResolvedAt = &now

			if m.config.NotifyOnResolved {
				event := m.history[i]
				event.Message = formatAlertMessage(rule, value, false)
				go m.notifier.Notify(&event)
			}
			break
		}
	}

	state.IsFiring = false
	state.FiringEventID = ""

	go m.saveHistory()

	log.Printf("[Alerts] RESOLVED: %s (%.2f)", rule.Name, value)
}

// evaluateCondition 评估条件
func evaluateCondition(value float64, op Operator, threshold float64) bool {
	switch op {
	case OpGreaterThan:
		return value > threshold
	case OpLessThan:
		return value < threshold
	case OpEqual:
		return value == threshold
	case OpNotEqual:
		return value != threshold
	case OpGTE:
		return value >= threshold
	case OpLTE:
		return value <= threshold
	}
	return false
}

// formatAlertMessage 格式化告警消息
func formatAlertMessage(rule *AlertRule, value float64, firing bool) string {
	status := "🔴 触发"
	if !firing {
		status = "✅ 恢复"
	}

	unit := "%"
	if rule.Metric == "load1" || rule.Metric == "load5" || rule.Metric == "load15" {
		unit = ""
	}

	return fmt.Sprintf("%s [%s] %s: %.2f%s %s %.2f%s",
		status, rule.Severity, rule.Name, value, unit, rule.Operator, rule.Threshold, unit)
}

// ============================================================================
//  历史查询
// ============================================================================

// GetHistory 获取告警历史
func (m *Manager) GetHistory(query AlertHistoryQuery) PaginatedHistory {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 过滤
	filtered := make([]AlertEvent, 0)
	for _, e := range m.history {
		if query.RuleID != "" && e.RuleID != query.RuleID {
			continue
		}
		if query.Status != "" && e.Status != query.Status {
			continue
		}
		if query.Severity != "" && e.Severity != query.Severity {
			continue
		}
		if query.Since != nil && e.FiredAt.Before(*query.Since) {
			continue
		}
		if query.Until != nil && e.FiredAt.After(*query.Until) {
			continue
		}
		filtered = append(filtered, e)
	}

	// 按时间倒序
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].FiredAt.After(filtered[j].FiredAt)
	})

	total := len(filtered)

	// 分页
	limit := query.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}

	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	page := offset/limit + 1
	totalPages := (total + limit - 1) / limit

	return PaginatedHistory{
		Events:     filtered[start:end],
		Total:      total,
		Page:       page,
		PageSize:   limit,
		TotalPages: totalPages,
	}
}

// GetActiveAlerts 获取活跃告警
func (m *Manager) GetActiveAlerts() []ActiveAlert {
	m.mu.RLock()
	defer m.mu.RUnlock()

	active := make([]ActiveAlert, 0)
	now := time.Now()

	for _, event := range m.history {
		if event.Status == StatusFiring {
			alert := ActiveAlert{
				AlertEvent: event,
				Duration:   now.Sub(event.FiredAt).Round(time.Second).String(),
			}
			active = append(active, alert)
		}
	}

	// 按时间倒序
	sort.Slice(active, func(i, j int) bool {
		return active[i].FiredAt.After(active[j].FiredAt)
	})

	return active
}

// GetSummary 获取告警摘要
func (m *Manager) GetSummary() AlertSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	summary := AlertSummary{
		TotalRules: len(m.rules),
	}

	for _, rule := range m.rules {
		if rule.Enabled {
			summary.EnabledRules++
		}
	}

	today := time.Now().Truncate(24 * time.Hour)
	for _, event := range m.history {
		if event.Status == StatusFiring {
			summary.FiringAlerts++
		}
		if event.FiredAt.After(today) {
			summary.TodayEvents++
		}
	}

	return summary
}

// ============================================================================
//  持久化
// ============================================================================

func (m *Manager) configPath() string {
	return filepath.Join(m.dataDir, "alerts_config.json")
}

func (m *Manager) rulesPath() string {
	return filepath.Join(m.dataDir, "alerts_rules.json")
}

func (m *Manager) historyPath() string {
	return filepath.Join(m.dataDir, "alerts_history.json")
}

func (m *Manager) loadConfig() error {
	data, err := os.ReadFile(m.configPath())
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &m.config)
}

func (m *Manager) saveConfig() error {
	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.configPath(), data, 0644)
}

func (m *Manager) loadRules() error {
	data, err := os.ReadFile(m.rulesPath())
	if err != nil {
		return err
	}

	var rules []AlertRule
	if err := json.Unmarshal(data, &rules); err != nil {
		return err
	}

	m.rules = make(map[string]*AlertRule)
	for i := range rules {
		m.rules[rules[i].ID] = &rules[i]
	}

	// 确保所有内置规则都存在
	for _, builtin := range BuiltinRules {
		if _, exists := m.rules[builtin.ID]; !exists {
			rule := builtin
			rule.CreatedAt = time.Now()
			rule.UpdatedAt = time.Now()
			m.rules[rule.ID] = &rule
		}
	}

	return nil
}

func (m *Manager) saveRules() error {
	rules := make([]AlertRule, 0, len(m.rules))
	for _, r := range m.rules {
		rules = append(rules, *r)
	}

	// 排序以保证稳定输出
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].ID < rules[j].ID
	})

	data, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.rulesPath(), data, 0644)
}

func (m *Manager) loadHistory() error {
	data, err := os.ReadFile(m.historyPath())
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &m.history)
}

func (m *Manager) saveHistory() error {
	m.mu.RLock()
	data, err := json.MarshalIndent(m.history, "", "  ")
	m.mu.RUnlock()

	if err != nil {
		return err
	}
	return os.WriteFile(m.historyPath(), data, 0644)
}

// ============================================================================
//  测试通知
// ============================================================================

// TestNotification 发送测试通知
func (m *Manager) TestNotification(channelType string) error {
	testEvent := &AlertEvent{
		ID:        "test-" + generateID(),
		RuleID:    "test",
		RuleName:  "测试告警",
		Metric:    "cpu",
		Status:    StatusFiring,
		Severity:  SeverityWarning,
		Value:     50.0,
		Threshold: 80.0,
		Operator:  OpGreaterThan,
		Message:   "🧪 这是一条测试告警消息",
		FiredAt:   time.Now(),
	}

	return m.notifier.NotifyChannel(testEvent, channelType)
}
