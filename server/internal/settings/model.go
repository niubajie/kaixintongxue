// Package settings 管理后台运营配置（circle.settings）。
// 只允许白名单键；写入一律记录审计。
package settings

import "time"

// Setting 一条配置（值为整数，避免浮点误差）。
type Setting struct {
	Key       string    `json:"key"`
	Value     int64     `json:"value"`
	UpdatedBy string    `json:"updatedBy"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// UpdateRequest 修改配置。
type UpdateRequest struct {
	Value *int64 `json:"value"`
}

// Rule 白名单键的取值范围。
type Rule struct {
	Label string
	Min   int64
	Max   int64
}

// Rules 允许后台修改的配置键。
var Rules = map[string]Rule{
	"resource.default_expire_days":       {Label: "资源默认有效期（天）", Min: 7, Max: 365},
	"resource.simhash_hanning_threshold": {Label: "弱去重汉明距离阈值", Min: 0, Max: 10},
	"resource.quality_threshold":         {Label: "质量分检索阈值", Min: 0, Max: 100},
	"resource.review_sla_hours":          {Label: "审核 SLA（小时）", Min: 1, Max: 168},
}
