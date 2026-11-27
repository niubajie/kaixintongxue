// Package category manages the 8 fixed resource categories:
// enable/disable, ordering, and per-category pricing.
package category

import "time"

type Category struct {
	Code              string    `json:"code"`
	Name              string    `json:"name"`
	Enabled           bool      `json:"enabled"`
	SortOrder         int       `json:"sortOrder"`
	ContactPriceCents int64     `json:"contactPriceCents"`
	AccessPriceCents  int64     `json:"accessPriceCents"`
	SensitiveLevel    string    `json:"sensitiveLevel"`
	DisclaimerText    string    `json:"disclaimerText"`
	CreatedAt         time.Time `json:"createdAt"`
}

// UpdateRequest — 后台可改字段（code 不可改）。
type UpdateRequest struct {
	Enabled           *bool   `json:"enabled"`
	SortOrder         *int    `json:"sortOrder"`
	ContactPriceCents *int64  `json:"contactPriceCents"`
	AccessPriceCents  *int64  `json:"accessPriceCents"`
	SensitiveLevel    *string `json:"sensitiveLevel"`
	DisclaimerText    *string `json:"disclaimerText"`
}
