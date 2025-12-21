package alerts

import "errors"

var (
	// 规则相关错误
	ErrRuleNotFound     = errors.New("alert rule not found")
	ErrRuleExists       = errors.New("alert rule already exists")
	ErrInvalidRuleID    = errors.New("invalid rule ID")
	ErrInvalidRuleName  = errors.New("invalid rule name")
	ErrInvalidMetric    = errors.New("invalid metric")
	ErrInvalidOperator  = errors.New("invalid operator")
	ErrInvalidThreshold = errors.New("invalid threshold")
	ErrInvalidDuration  = errors.New("invalid duration format")
	ErrBuiltinRule      = errors.New("cannot modify builtin rule ID")

	// 事件相关错误
	ErrEventNotFound = errors.New("alert event not found")

	// 配置相关错误
	ErrInvalidConfig = errors.New("invalid alert configuration")
)
