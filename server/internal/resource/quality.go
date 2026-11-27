package resource

import "unicode/utf8"

// QualityScore 计算 0–100 质量分。
//
// 构成（上限 100）：
//
//	标题  20：≥10 字 20；6–9 字 12；其余 5
//	摘要  20：≥40 字 20；15–39 字 12；非空 6
//	详情  25：≥200 字 25；80–199 字 15；非空 6
//	地区  10：非空 10
//	联系方式 25：手机号 15 + 微信 10（任一至少 15）
//
// 低于 50 由检索层降权（见 S3）。
func QualityScore(title, summary, detail, region string, hasPhone, hasWechat bool) int {
	score := 0

	switch n := utf8.RuneCountInString(title); {
	case n >= 10:
		score += 20
	case n >= 6:
		score += 12
	case n > 0:
		score += 5
	}

	switch n := utf8.RuneCountInString(summary); {
	case n >= 40:
		score += 20
	case n >= 15:
		score += 12
	case n > 0:
		score += 6
	}

	switch n := utf8.RuneCountInString(detail); {
	case n >= 200:
		score += 25
	case n >= 80:
		score += 15
	case n > 0:
		score += 6
	}

	if region != "" {
		score += 10
	}
	if hasPhone {
		score += 15
	}
	if hasWechat {
		score += 10
	}

	if score > 100 {
		score = 100
	}
	return score
}
