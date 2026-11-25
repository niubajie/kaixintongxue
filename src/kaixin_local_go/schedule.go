package main

import (
	"context"
	"net/http"
	"strings"
	"time"
)

type SearchSchedule struct {
	ID          string `json:"id"`
	Keyword     string `json:"keyword"`
	Category    string `json:"category"`
	SubCategory string `json:"subCategory"`
	Freq        string `json:"freq"`
	Hour        int    `json:"hour"`
	Weekday     int    `json:"weekday"`
	Day         int    `json:"day"`
	Enabled     bool   `json:"enabled"`
	LastRunAt   string `json:"lastRunAt"`
	CreatedAt   string `json:"createdAt"`
}

func (a *App) loadSchedules(ctx context.Context, onlyEnabled bool) ([]SearchSchedule, error) {
	query := `SELECT id, keyword, category, sub_category, freq, hour, weekday, day, enabled, last_run_at, created_at
	          FROM search_schedules`
	if onlyEnabled {
		query += ` WHERE enabled = 1`
	}
	query += ` ORDER BY created_at DESC`
	rows, err := a.store.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]SearchSchedule, 0)
	for rows.Next() {
		var item SearchSchedule
		var enabled int
		if err := rows.Scan(&item.ID, &item.Keyword, &item.Category, &item.SubCategory,
			&item.Freq, &item.Hour, &item.Weekday, &item.Day, &enabled,
			&item.LastRunAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.Enabled = enabled == 1
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *App) handleScheduleList(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []SearchSchedule{}})
		return
	}
	items, err := a.loadSchedules(r.Context(), false)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取定时任务失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) handleScheduleSave(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	var request SearchSchedule
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	request.Keyword = strings.TrimSpace(request.Keyword)
	if request.Keyword == "" {
		fail(w, http.StatusBadRequest, "请输入搜索关键词")
		return
	}
	switch request.Freq {
	case "daily", "weekly", "monthly":
	default:
		fail(w, http.StatusBadRequest, "频率只能是每天 / 每周 / 每月")
		return
	}
	if request.Hour < 0 || request.Hour > 23 {
		request.Hour = 9
	}
	if request.Weekday < 1 || request.Weekday > 7 {
		request.Weekday = 1
	}
	if request.Day < 1 || request.Day > 28 {
		request.Day = 1
	}
	request.Category = normalizeCategory(request.Category)
	request.SubCategory = sanitizeSegment(request.SubCategory)

	now := time.Now().UTC().Format(time.RFC3339)
	if strings.TrimSpace(request.ID) == "" {
		request.ID = newRandomID()
		request.CreatedAt = now
		if _, err := a.store.db.ExecContext(r.Context(),
			`INSERT INTO search_schedules (id, keyword, category, sub_category, freq, hour, weekday, day, enabled, last_run_at, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?)`,
			request.ID, request.Keyword, request.Category, request.SubCategory,
			request.Freq, request.Hour, request.Weekday, request.Day, 1, now); err != nil {
			fail(w, http.StatusInternalServerError, "保存定时任务失败："+err.Error())
			return
		}
	} else {
		if _, err := a.store.db.ExecContext(r.Context(),
			`UPDATE search_schedules SET keyword = ?, category = ?, sub_category = ?, freq = ?, hour = ?, weekday = ?, day = ?, enabled = ?
			 WHERE id = ?`,
			request.Keyword, request.Category, request.SubCategory, request.Freq,
			request.Hour, request.Weekday, request.Day, boolToInt(request.Enabled), request.ID); err != nil {
			fail(w, http.StatusInternalServerError, "更新定时任务失败："+err.Error())
			return
		}
	}
	if request.CreatedAt == "" {
		request.CreatedAt = now
	}
	writeJSON(w, http.StatusOK, request)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (a *App) handleScheduleDelete(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		fail(w, http.StatusBadRequest, "缺少任务 ID")
		return
	}
	if _, err := a.store.db.ExecContext(r.Context(), `DELETE FROM search_schedules WHERE id = ?`, id); err != nil {
		fail(w, http.StatusInternalServerError, "删除定时任务失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

// scheduleDue 判断任务当前是否该执行：到点、当天未跑过、频率匹配。
func scheduleDue(item SearchSchedule, now time.Time) bool {
	if now.Hour() < item.Hour {
		return false
	}
	if item.LastRunAt != "" {
		if last, err := time.Parse(time.RFC3339, item.LastRunAt); err == nil {
			lastLocal := last.Local()
			if lastLocal.YearDay() == now.YearDay() && lastLocal.Year() == now.Year() {
				return false
			}
		}
	}
	switch item.Freq {
	case "weekly":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		return weekday == item.Weekday
	case "monthly":
		return now.Day() == item.Day
	default:
		return true
	}
}

// runScheduler 在应用运行期间按分钟扫描定时任务；应用关闭即暂停，不注册系统计划任务。
func (a *App) runScheduler(ctx context.Context) {
	if a.store == nil {
		return
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	a.tickSchedules(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.tickSchedules(ctx)
		}
	}
}

func (a *App) tickSchedules(ctx context.Context) {
	items, err := a.loadSchedules(ctx, true)
	if err != nil {
		return
	}
	now := time.Now()
	for _, item := range items {
		if !scheduleDue(item, now) {
			continue
		}
		sites, _ := a.loadSites(ctx, true)
		results, _, err := webSearchWithSites(ctx, item.Keyword, 5, sites)
		if err != nil {
			continue
		}
		for _, result := range results {
			_, _ = a.insertPending(ctx, item.Keyword, item.Category, item.SubCategory, result)
		}
		_, _ = a.store.db.ExecContext(ctx,
			`UPDATE search_schedules SET last_run_at = ? WHERE id = ?`,
			time.Now().UTC().Format(time.RFC3339), item.ID)
	}
}
