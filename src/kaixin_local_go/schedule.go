package main

import (
	"context"
	"net/http"
	"sort"
	"strconv"
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
	Weekdays    []int  `json:"weekdays"`
	Days        []int  `json:"days"`
	// Types 是资料选项：电子书 / 文档 / 视频 / 杂志 / 留言 / 全部。
	// 「全部」表示把该网站所有资料都下载，不再按关键词过滤。
	Types []string `json:"types"`
	// SiteIDs 是本次任务要搜索的网站（来自「网页设置」）；空表示全部启用网站。
	SiteIDs []string `json:"siteIds"`
	// PublishWithin 是发布时间：1d / 3d / 1w / 1m / 3m，空表示不限。
	PublishWithin string `json:"publishWithin"`
	// DownloadLimitBytes 是单轮下载总大小上限（字节）；0 表示不下载任何资料。
	DownloadLimitBytes int64 `json:"downloadLimitBytes"`
	Enabled            bool  `json:"enabled"`
	Collected          int   `json:"collected"`
	Running            bool  `json:"running"`
	// Phase 是运行阶段：search（检索中）/ download（下载中）/ repair（自动修复中）。
	Phase string `json:"phase"`
	// 自动修复后仍需用户处理的事项；任务暂停后也会保留，供前端弹出登录或重试窗口。
	IssueKind        string `json:"issueKind,omitempty"`
	IssueSiteID      string `json:"issueSiteId,omitempty"`
	IssueMessage     string `json:"issueMessage,omitempty"`
	IssueFailureCode string `json:"issueFailureCode,omitempty"`
	IssueNonce       int    `json:"issueNonce,omitempty"`
	LastRunAt        string `json:"lastRunAt"`
	CreatedAt        string `json:"createdAt"`
}

type scheduleIssue struct {
	Kind        string
	SiteID      string
	Message     string
	FailureCode string
	Nonce       int
}

// progressStamp 是监督程序判断下载是否停滞的依据：累计搜集数与记录时间。
type progressStamp struct {
	collected int
	at        time.Time
}

// bumpRunNonce 给一次运行或一次手动重试生成新序号，前端据此区分“同一条错误的新一轮”。
func (a *App) bumpRunNonce(scheduleID string) int {
	var next int
	if value, ok := a.scheduleRunNonce.Load(scheduleID); ok {
		if n, isInt := value.(int); isInt {
			next = n
		}
	}
	next++
	a.scheduleRunNonce.Store(scheduleID, next)
	return next
}

func (a *App) currentRunNonce(scheduleID string) int {
	if value, ok := a.scheduleRunNonce.Load(scheduleID); ok {
		if n, isInt := value.(int); isInt {
			return n
		}
	}
	return 0
}

// requestRepair 由监督程序在下载停滞时调用，runScheduleSite 循环里会让出进入修复。
func (a *App) requestRepair(scheduleID string) {
	a.scheduleRepairRequested.Store(scheduleID, true)
}

func (a *App) repairRequested(scheduleID string) bool {
	value, ok := a.scheduleRepairRequested.Load(scheduleID)
	return ok && value != nil
}

func (a *App) clearRepairRequest(scheduleID string) {
	a.scheduleRepairRequested.Delete(scheduleID)
}

func (a *App) setCurrentSite(scheduleID, siteID string) {
	if siteID == "" {
		a.scheduleCurrentSite.Delete(scheduleID)
		return
	}
	a.scheduleCurrentSite.Store(scheduleID, siteID)
}

func (a *App) currentSiteID(scheduleID string) string {
	value, ok := a.scheduleCurrentSite.Load(scheduleID)
	if !ok {
		return ""
	}
	siteID, _ := value.(string)
	return siteID
}

// currentCollected 读数据库里的累计搜集数，作为监督程序的客观进度来源。
func (a *App) currentCollected(ctx context.Context, scheduleID string) int {
	if a.store == nil {
		return 0
	}
	var collected int
	if err := a.store.db.QueryRowContext(ctx, `SELECT collected FROM search_schedules WHERE id = ?`, scheduleID).Scan(&collected); err != nil {
		return 0
	}
	return collected
}

// storeAssistResults 保存修复向导验证过的结果，并唤醒正在等待的 runScheduleSite。
func (a *App) storeAssistResults(assistKey string, results []webResult) {
	a.scheduleAssistResults.Store(assistKey, results)
	if value, ok := a.scheduleAssistWait.Load(assistKey); ok {
		if ch, isChan := value.(chan struct{}); isChan {
			select {
			case <-ch:
			default:
				close(ch)
			}
		}
	}
}

// consumeAssistResults 取出并删除修复向导的结果；没有则返回 nil。
func (a *App) consumeAssistResults(assistKey string) []webResult {
	if cached, ok := a.scheduleAssistResults.LoadAndDelete(assistKey); ok {
		if results, valid := cached.([]webResult); valid {
			return results
		}
	}
	return nil
}

// waitForAssistResults 等待修复向导产出结果，可被 ctx 取消或超时打断。
func (a *App) waitForAssistResults(ctx context.Context, assistKey string, timeout time.Duration) ([]webResult, bool) {
	if results := a.consumeAssistResults(assistKey); len(results) > 0 {
		return results, true
	}
	ch := make(chan struct{})
	if actual, loaded := a.scheduleAssistWait.LoadOrStore(assistKey, ch); loaded {
		if existing, isChan := actual.(chan struct{}); isChan {
			ch = existing
		}
	} else {
		// 刚创建的通道，再检查一次结果，避免错过向导在 LoadOrStore 之前写入的窗口。
		if results := a.consumeAssistResults(assistKey); len(results) > 0 {
			a.scheduleAssistWait.Delete(assistKey)
			return results, true
		}
	}
	defer a.scheduleAssistWait.Delete(assistKey)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ch:
		return a.consumeAssistResults(assistKey), true
	case <-ctx.Done():
		return nil, false
	case <-timer.C:
		return nil, false
	}
}

func joinIntList(values []int) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, ",")
}

func parseIntList(value string) []int {
	out := make([]int, 0)
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if number, err := strconv.Atoi(part); err == nil {
			out = append(out, number)
		}
	}
	return out
}

// normalizeIntList 去重、排序并过滤到 [min,max]；空结果表示「全选」。
func normalizeIntList(values []int, min, max int) []int {
	seen := make(map[int]bool, len(values))
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value < min || value > max || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// 资料选项：默认勾选前四项，「留言」和「全部」默认不选。
var scheduleTypeOptions = []string{"电子书", "文档", "视频", "杂志", "留言", "全部"}

// normalizeTypes 去重、只保留已知选项；勾了「全部」就只按「全部」处理。
func normalizeTypes(values []string) []string {
	allowed := make(map[string]bool, len(scheduleTypeOptions))
	for _, option := range scheduleTypeOptions {
		allowed[option] = true
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !allowed[value] || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	if seen["全部"] {
		return []string{"全部"}
	}
	return out
}

func parseStringList(value string) []string {
	out := make([]string, 0)
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// normalizePublishWithin 只接受预设的发布时间，其余一律按「不限」处理。
func normalizePublishWithin(value string) string {
	switch strings.TrimSpace(value) {
	case "1d", "3d", "1w", "1m", "3m":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

// maxScheduleDownloadBytes 是下载上限的天花板：1T。
const maxScheduleDownloadBytes int64 = 1000000000000

// normalizeDownloadBytes 把下载上限收敛到 [0, 1T]；0 表示不下载任何资料。
func normalizeDownloadBytes(value int64) int64 {
	if value < 0 {
		return 0
	}
	if value > maxScheduleDownloadBytes {
		return maxScheduleDownloadBytes
	}
	return value
}

// bingFreshness 把发布时间映射成必应的 freshness 参数；不限返回空。
func bingFreshness(publishWithin string) string {
	switch publishWithin {
	case "1d", "3d":
		return "Day"
	case "1w":
		return "Week"
	case "1m", "3m":
		return "Month"
	default:
		return ""
	}
}

// normalizeSiteIDs 去重、去空；空结果表示「全部启用网站」。
func normalizeSiteIDs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func (a *App) loadSchedules(ctx context.Context, onlyEnabled bool) ([]SearchSchedule, error) {
	// 已搜集数量按当前待审核里的实际条数统计，避免删除/忽略后仍显示旧数字。
	query := `SELECT id, keyword, category, sub_category, freq, hour, weekday, day, weekdays, days, doc_types, site_ids, publish_within, download_limit_bytes, enabled,
	                 (SELECT COUNT(*) FROM pending_docs p WHERE p.schedule_id = search_schedules.id AND p.status = 'pending'),
	                 last_run_at, created_at
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
		var weekdays, days, docTypes, siteIDs string
		if err := rows.Scan(&item.ID, &item.Keyword, &item.Category, &item.SubCategory,
			&item.Freq, &item.Hour, &item.Weekday, &item.Day, &weekdays, &days, &docTypes, &siteIDs,
			&item.PublishWithin, &item.DownloadLimitBytes, &enabled,
			&item.Collected, &item.LastRunAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.Weekdays = parseIntList(weekdays)
		item.Days = parseIntList(days)
		item.Types = parseStringList(docTypes)
		item.SiteIDs = parseStringList(siteIDs)
		if len(item.Types) == 0 {
			item.Types = normalizeTypes([]string{"电子书", "文档", "视频", "杂志"})
		}
		item.Enabled = enabled == 1
		items = append(items, item)
	}
	return items, rows.Err()
}

// loadSchedule 按 ID 取出单个任务，点「开始」时用来立即执行。
func (a *App) loadSchedule(ctx context.Context, id string) (SearchSchedule, bool) {
	items, err := a.loadSchedules(ctx, false)
	if err != nil {
		return SearchSchedule{}, false
	}
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return SearchSchedule{}, false
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
	for index := range items {
		if value, running := a.scheduleRuns.Load(items[index].ID); running {
			items[index].Running = true
			if phase, ok := value.(string); ok {
				items[index].Phase = phase
			}
		}
		if value, issue := a.scheduleIssues.Load(items[index].ID); issue {
			if state, ok := value.(scheduleIssue); ok {
				items[index].IssueKind = state.Kind
				items[index].IssueSiteID = state.SiteID
				items[index].IssueMessage = state.Message
				items[index].IssueFailureCode = state.FailureCode
				items[index].IssueNonce = state.Nonce
			}
		}
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
	case "instant", "daily", "weekly", "monthly":
	default:
		fail(w, http.StatusBadRequest, "频率只能是即时 / 每天 / 每周 / 每月")
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
	request.Weekdays = normalizeIntList(request.Weekdays, 1, 7)
	request.Days = normalizeIntList(request.Days, 1, 28)
	request.Types = normalizeTypes(request.Types)
	if len(request.Types) == 0 {
		fail(w, http.StatusBadRequest, "请至少选择一种资料类型")
		return
	}
	request.SiteIDs = normalizeSiteIDs(request.SiteIDs)
	request.PublishWithin = normalizePublishWithin(request.PublishWithin)
	request.DownloadLimitBytes = normalizeDownloadBytes(request.DownloadLimitBytes)

	now := time.Now().UTC().Format(time.RFC3339)
	isNew := strings.TrimSpace(request.ID) == ""
	if isNew {
		request.ID = newRandomID()
		request.CreatedAt = now
		if _, err := a.store.db.ExecContext(r.Context(),
			`INSERT INTO search_schedules (id, keyword, category, sub_category, freq, hour, weekday, day, weekdays, days, doc_types, site_ids, publish_within, download_limit_bytes, enabled, last_run_at, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?)`,
			request.ID, request.Keyword, request.Category, request.SubCategory,
			request.Freq, request.Hour, request.Weekday, request.Day,
			joinIntList(request.Weekdays), joinIntList(request.Days),
			strings.Join(request.Types, ","), strings.Join(request.SiteIDs, ","),
			request.PublishWithin, request.DownloadLimitBytes, 1, now); err != nil {
			fail(w, http.StatusInternalServerError, "保存定时任务失败："+err.Error())
			return
		}
	} else {
		if _, err := a.store.db.ExecContext(r.Context(),
			`UPDATE search_schedules SET keyword = ?, category = ?, sub_category = ?, freq = ?, hour = ?, weekday = ?, day = ?, weekdays = ?, days = ?, doc_types = ?, site_ids = ?, publish_within = ?, download_limit_bytes = ?, enabled = ?
			 WHERE id = ?`,
			request.Keyword, request.Category, request.SubCategory, request.Freq,
			request.Hour, request.Weekday, request.Day,
			joinIntList(request.Weekdays), joinIntList(request.Days),
			strings.Join(request.Types, ","), strings.Join(request.SiteIDs, ","),
			request.PublishWithin, request.DownloadLimitBytes,
			boolToInt(request.Enabled), request.ID); err != nil {
			fail(w, http.StatusInternalServerError, "更新定时任务失败："+err.Error())
			return
		}
	}
	if request.CreatedAt == "" {
		request.CreatedAt = now
	}
	// 新建的「即时」任务马上开跑，不必等下一次 30 秒调度；后续仍由调度器负责周期任务。
	if isNew && request.Freq == "instant" {
		request.Enabled = true
		if _, err := a.store.db.ExecContext(r.Context(),
			`UPDATE search_schedules SET last_run_at = ? WHERE id = ?`, now, request.ID); err == nil {
			item := request
			go a.runScheduleTask(context.Background(), item)
		}
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

// handleScheduleEnabled 暂停 / 继续一个任务：暂停后调度器不再触发，正在执行的也会在下一个网站前停下。
func (a *App) handleScheduleEnabled(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		fail(w, http.StatusBadRequest, "缺少任务 ID")
		return
	}
	var request struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	// 开始（重新启用）时清空上次执行时间，让调度器在下一轮立即重跑；
	// 否则「即时」任务当天已跑过会被同日判断拦住，看起来像没执行。
	if request.Enabled {
		if _, err := a.store.db.ExecContext(r.Context(),
			`UPDATE search_schedules SET enabled = 1, last_run_at = '' WHERE id = ?`, id); err != nil {
			fail(w, http.StatusInternalServerError, "更新任务状态失败："+err.Error())
			return
		}
	} else if _, err := a.store.db.ExecContext(r.Context(),
		`UPDATE search_schedules SET enabled = 0 WHERE id = ?`, id); err != nil {
		fail(w, http.StatusInternalServerError, "更新任务状态失败："+err.Error())
		return
	}
	// 暂停：立刻 cancel 掉正在执行的任务 context，正在下载的 HTTP 请求会立即中断。
	if !request.Enabled {
		a.scheduleIssues.Delete(id)
		if cancel, ok := a.scheduleCancels.LoadAndDelete(id); ok {
			if fn, isFunc := cancel.(context.CancelFunc); isFunc && fn != nil {
				fn()
			}
		}
	}
	// 点「开始」立刻执行，不等下一次 30 秒调度；重复点击会被运行态去重。
	if request.Enabled {
		if item, ok := a.loadSchedule(r.Context(), id); ok {
			go a.runScheduleTask(context.Background(), item)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "enabled": request.Enabled})
}

// scheduleDue 判断任务当前是否该执行：到点、当天未跑过、频率匹配。
func scheduleDue(item SearchSchedule, now time.Time) bool {
	if item.Freq != "instant" && now.Hour() < item.Hour {
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
	case "instant":
		// 即时任务只执行一次；重复执行已被上面的 last_run_at 同日判断拦截。
		return true
	case "weekly":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		if len(item.Weekdays) == 0 {
			return true
		}
		return containsInt(item.Weekdays, weekday)
	case "monthly":
		if len(item.Days) == 0 {
			return true
		}
		return containsInt(item.Days, now.Day())
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
		if _, running := a.scheduleRuns.Load(item.ID); running {
			continue
		}
		// 先落 last_run_at，避免下一轮 tick 重复触发同一个任务。
		_, _ = a.store.db.ExecContext(ctx,
			`UPDATE search_schedules SET last_run_at = ? WHERE id = ?`,
			time.Now().UTC().Format(time.RFC3339), item.ID)
		go a.runScheduleTask(ctx, item)
	}
}

// runScheduleTask 一轮完整的搜集：按「网站设置」逐个网站、按「资料选项」逐个类型搜到底。
// 全程有监督程序盯着：下载停滞随时重新进入修复；单个网站修复期间任务保持运行，不再“修一次就退出”。
func (a *App) runScheduleTask(ctx context.Context, item SearchSchedule) {
	// 同一任务只允许一个实例在跑：手动点「开始」和调度器同时触发时，后来者直接返回。
	if _, loaded := a.scheduleRuns.LoadOrStore(item.ID, schedulePhaseSearch); loaded {
		return
	}
	defer a.scheduleRuns.Delete(item.ID)
	// 新一轮开始时清除上一轮遗留的登录/修复提示，并生成新的运行序号。
	a.scheduleIssues.Delete(item.ID)
	a.clearRepairRequest(item.ID)
	a.bumpRunNonce(item.ID)
	// 用 WithCancel 包住：暂停时调 cancel 可立即打断正在进行的 HTTP 下载。
	cancelCtx, cancel := context.WithCancel(ctx)
	a.scheduleCancels.Store(item.ID, cancel)
	defer func() {
		cancel()
		a.scheduleCancels.Delete(item.ID)
		a.setCurrentSite(item.ID, "")
		a.scheduleBudget.Delete(item.ID)
		a.clearRepairRequest(item.ID)
		a.scheduleLastProgress.Delete(item.ID)
	}()
	// 记录本次执行时间，手动点「开始」与定时调度共用同一套状态。
	_, _ = a.store.db.ExecContext(cancelCtx,
		`UPDATE search_schedules SET last_run_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), item.ID)

	// 文字类资料（留言）先按类型汇总，一轮结束后合并写成一个文档。
	textChunks := map[string][]string{}
	// 本轮下载预算：0 表示不下载任何资料，达到上限后立即停止。
	budget := &downloadBudget{limit: item.DownloadLimitBytes}
	if budget.limit <= 0 {
		a.finishScheduleRun(cancelCtx, item)
		return
	}
	sites, _ := a.loadSites(cancelCtx, true)
	sites = filterScheduleSites(sites, item.SiteIDs)
	if len(sites) == 0 {
		// 任务没有选中任何网站（或系统里没有启用中的网站）：不搜索、不下载，直接结束本轮。
		a.finishScheduleRun(cancelCtx, item)
		return
	}
	// 监督程序：下载停滞时主动要求修复，贯穿整轮任务的生命周期。
	a.scheduleBudget.Store(item.ID, budget)
	go a.runScheduleSupervisor(cancelCtx, item.ID, budget)
	for _, site := range sites {
		issue := a.runScheduleSite(cancelCtx, item, site, budget, textChunks)
		if issue != nil {
			a.scheduleIssues.Store(item.ID, *issue)
			a.saveTextCollections(cancelCtx, item, textChunks)
			a.finishScheduleRun(cancelCtx, item)
			return
		}
		if !a.scheduleActive(cancelCtx, item.ID) || budget.exhausted() {
			a.saveTextCollections(cancelCtx, item, textChunks)
			a.finishScheduleRun(cancelCtx, item)
			return
		}
	}
	a.collectScheduleVideos(cancelCtx, item, sites, budget)
	a.saveTextCollections(cancelCtx, item, textChunks)
	a.finishScheduleRun(cancelCtx, item)
}

// runScheduleSite 搜索并下载一个网站。
// 没有新增下载时进入“修复循环”：先自动验证，失败则交给分步修复向导，并阻塞等待向导产出
// 经过真实下载校验的结果后再继续；监督程序发现停滞也会把执行流让到这里。任务在此期间保持运行。
func (a *App) runScheduleSite(ctx context.Context, item SearchSchedule, site SearchSite, budget *downloadBudget, textChunks map[string][]string) *scheduleIssue {
	assistKey := item.ID + "|" + site.ID
	a.setCurrentSite(item.ID, site.ID)
	defer a.setCurrentSite(item.ID, "")

	// 修复向导已经在站内找到并验证过结果，恢复任务时优先下载这些结果。
	if results := a.consumeAssistResults(assistKey); len(results) > 0 {
		if saved := a.downloadResults(ctx, item, results, budget); saved > 0 {
			return nil
		}
	}
	queries := buildScheduleQueries(item.Keyword, item.Types, site.Domain)
	hasDownloadQuery := false
	for _, query := range queries {
		if query.Type == "视频" && (strings.Contains(strings.ToLower(site.Domain), "bilibili") || strings.Contains(site.Name, "哔哩")) {
			continue
		}
		if !isTextDocType(query.Type) {
			hasDownloadQuery = true
		}
	}

	runQueries := func() int {
		saved := 0
		for _, query := range queries {
			if !a.scheduleActive(ctx, item.ID) || budget.exhausted() || a.repairRequested(item.ID) {
				return saved
			}
			if query.Type == "视频" && (strings.Contains(strings.ToLower(site.Domain), "bilibili") || strings.Contains(site.Name, "哔哩")) {
				continue
			}
			results, err := runScheduleQuery(ctx, site, query, bingFreshness(item.PublishWithin))
			if err != nil {
				continue
			}
			if isTextDocType(query.Type) {
				textChunks[query.Type] = appendTextChunks(textChunks[query.Type], site.Name, results)
				continue
			}
			saved += a.downloadResults(ctx, item, results, budget)
		}
		return saved
	}

	saved := runQueries()
	if !hasDownloadQuery || saved > 0 || budget.exhausted() || !a.scheduleActive(ctx, item.ID) {
		a.clearRepairRequest(item.ID)
		return nil
	}

	// 没有新增下载：进入修复循环。任务保持运行（phase=repair），修好了继续，不再一次失败就退出。
	const maxManualCycles = 3
	for cycle := 0; cycle < maxManualCycles; cycle++ {
		if !a.scheduleActive(ctx, item.ID) || budget.exhausted() {
			return nil
		}
		a.clearRepairRequest(item.ID)
		a.scheduleRuns.Store(item.ID, schedulePhaseRepair)

		// 先用无头自动验证兜一遍简单可恢复的情况（百度风控恢复、换关键词命中等）。
		autoOK := false
		var lastMessage, lastHint string
		var lastNeedLogin bool
		for attempt := 0; attempt < 2; attempt++ {
			repairCtx, cancelRepair := context.WithTimeout(ctx, 90*time.Second)
			ok, message, hint, needLogin := verifySiteDownloadable(repairCtx, site.Domain, item.Keyword)
			cancelRepair()
			if !a.scheduleActive(ctx, item.ID) {
				return nil
			}
			if ok {
				autoOK = true
				break
			}
			lastMessage, lastHint, lastNeedLogin = message, hint, needLogin
			if needLogin {
				break // 登录必须人工处理，不再浪费自动尝试
			}
		}
		if autoOK {
			a.scheduleIssues.Delete(item.ID)
			a.scheduleRuns.Store(item.ID, schedulePhaseSearch)
			saved = runQueries()
			if saved > 0 || !hasDownloadQuery || budget.exhausted() || !a.scheduleActive(ctx, item.ID) {
				return nil
			}
			// 自动验证说可以，但实际还是下不到 → 升级到人工向导。
		}

		// 升级到分步修复向导：发布带序号的事项，前端弹出浏览器向导；这里阻塞等待真实结果。
		kind := "failed"
		failureCode := "no_download"
		if lastNeedLogin {
			kind = "login"
			failureCode = "login"
		}
		baseMessage := "搜索后没有新增下载，已进入分步修复向导；请在弹出的浏览器里处理弹窗、验证码或登录"
		if lastMessage != "" {
			baseMessage += "（" + lastMessage + "）"
		}
		a.publishIssue(item.ID, site.ID, kind, baseMessage, lastHint, failureCode, a.bumpRunNonce(item.ID))
		results, ok := a.waitForAssistResults(ctx, assistKey, 15*time.Minute)
		if !a.scheduleActive(ctx, item.ID) {
			return nil
		}
		if !ok {
			// 等待被取消（暂停/超时）：继续下一轮，重新发布事项让前端再次弹出向导。
			continue
		}
		if len(results) == 0 {
			// 向导多次失败，监督程序判定放弃，结束本站并保留最终事项。
			return &scheduleIssue{
				Kind:        "failed",
				SiteID:      site.ID,
				Message:     "多次修复后仍没有真实下载；请更换关键词，或确认该站确实有可下载资料后再点击开始",
				FailureCode: "exhausted",
				Nonce:       a.currentRunNonce(item.ID),
			}
		}
		a.scheduleIssues.Delete(item.ID)
		a.scheduleRuns.Store(item.ID, schedulePhaseSearch)
		if saved := a.downloadResults(ctx, item, results, budget); saved > 0 {
			a.clearRepairRequest(item.ID)
			return nil
		}
		// 向导给了真实结果，但一条也没存下来：多半是这些资料此前已收录（去重命中）或文件无法下载。
		// 这属于“已处理完、无新增”，不应继续空转修复。
		saved = runQueries()
		if saved > 0 {
			return nil
		}
		return &scheduleIssue{
			Kind:        "failed",
			SiteID:      site.ID,
			Message:     "已找到并验证可下载资料，但均已在库中或无法下载，本轮没有新增内容",
			FailureCode: "nothing_new",
			Nonce:       a.bumpRunNonce(item.ID),
		}
	}
	return &scheduleIssue{
		Kind:        "failed",
		SiteID:      site.ID,
		Message:     "多次修复后仍没有新增下载；请更换关键词，或确认该站有可下载资料后再次点击开始",
		FailureCode: "exhausted",
		Nonce:       a.bumpRunNonce(item.ID),
	}
}

// publishIssue 把一条待处理事项写进内存，供前端弹出修复/登录窗口；带序号避免与历史事项去重冲突。
func (a *App) publishIssue(scheduleID, siteID, kind, message, hint, failureCode string, nonce int) {
	if hint != "" {
		message += "；" + hint
	}
	a.scheduleIssues.Store(scheduleID, scheduleIssue{
		Kind: kind, SiteID: siteID, Message: message, FailureCode: failureCode, Nonce: nonce,
	})
}

// runScheduleSupervisor 全程监督一轮任务：累计搜集数长时间不增长就要求重新进入修复。
func (a *App) runScheduleSupervisor(ctx context.Context, scheduleID string, budget *downloadBudget) {
	const tick = 8 * time.Second
	const stallThreshold = 45 * time.Second
	lastCollected := -1
	var stalledAt time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(tick):
		}
		if _, running := a.scheduleRuns.Load(scheduleID); !running {
			return
		}
		if !a.scheduleActive(ctx, scheduleID) || budget.exhausted() {
			continue
		}
		siteID := a.currentSiteID(scheduleID)
		if siteID == "" {
			lastCollected = -1
			stalledAt = time.Time{}
			continue
		}
		if phase, ok := a.scheduleRuns.Load(scheduleID); ok && phase == schedulePhaseRepair {
			continue // 已经在修复中，不叠加触发
		}
		collected := a.currentCollected(ctx, scheduleID)
		now := time.Now()
		if collected != lastCollected {
			lastCollected = collected
			stalledAt = now
			continue
		}
		if stalledAt.IsZero() {
			stalledAt = now
		}
		if now.Sub(stalledAt) >= stallThreshold && !a.repairRequested(scheduleID) {
			a.requestRepair(scheduleID)
			a.publishIssue(scheduleID, siteID, "failed",
				"下载进度停滞，监督程序已重新进入修复流程", "", "stall", a.bumpRunNonce(scheduleID))
		}
	}
}

// 运行阶段：前端据此切换「正在搜索」「正在下载」与「修正中」动图。
const (
	schedulePhaseSearch   = "search"
	schedulePhaseDownload = "download"
	schedulePhaseRepair   = "repair"
)

// downloadBudget 跟踪本轮已下载字节，用来执行「下载大小」上限。
type downloadBudget struct {
	limit int64 // 上限（字节），<=0 表示不下载
	used  int64
}

// remaining 返回还可下载的字节数；已用满返回 0。
func (b *downloadBudget) remaining() int64 {
	if b == nil || b.limit <= 0 {
		return 0
	}
	left := b.limit - b.used
	if left < 0 {
		return 0
	}
	return left
}

// exhausted 本轮是否已经下满上限。
func (b *downloadBudget) exhausted() bool {
	return b == nil || b.remaining() <= 0
}

// add 累加实际下载字节。
func (b *downloadBudget) add(size int64) {
	if b != nil && size > 0 {
		b.used += size
	}
}

// downloadResults 进入下载阶段后再落库，前端能看到下载动图。
func (a *App) downloadResults(ctx context.Context, item SearchSchedule, results []webResult, budget *downloadBudget) int {
	if len(results) == 0 || budget.exhausted() || !a.scheduleActive(ctx, item.ID) {
		return 0
	}
	a.scheduleRuns.Store(item.ID, schedulePhaseDownload)
	saved := a.saveScheduleResults(ctx, item, results, budget)
	a.scheduleRuns.Store(item.ID, schedulePhaseSearch)
	return saved
}

// hasDocType 任务是否勾选了某个资料类型。
func hasDocType(types []string, target string) bool {
	for _, value := range types {
		if value == target {
			return true
		}
	}
	return false
}

// collectScheduleVideos 视频类型只从「哔哩哔哩」抓真实可下载的 mp4；
// 用户没勾选哔哩哔哩时不抓，尊重网站选择。
func (a *App) collectScheduleVideos(ctx context.Context, item SearchSchedule, sites []SearchSite, budget *downloadBudget) {
	if !hasDocType(item.Types, "视频") || budget.exhausted() || !a.scheduleActive(ctx, item.ID) {
		return
	}
	allowed := false
	for _, site := range sites {
		if strings.Contains(site.Name, "哔哩") || strings.Contains(strings.ToLower(site.Domain), "bilibili") {
			allowed = true
			break
		}
	}
	if !allowed {
		return
	}
	// 一次最多收 100 条，分页翻到平台没有更多为止；翻页中随时检查暂停。
	check := func() bool { return a.scheduleActive(ctx, item.ID) }
	results, err := bilibiliVideoSearch(ctx, item.Keyword, 100, check)
	if err != nil || len(results) == 0 {
		return
	}
	if !a.scheduleActive(ctx, item.ID) {
		return
	}
	a.downloadResults(ctx, item, results, budget)
}

// finishScheduleRun 收尾：即时任务跑完一次就关掉启用状态，
// 按钮从「暂停」变「开始」，用户再点开始会清空上次执行时间并重新执行。
func (a *App) finishScheduleRun(ctx context.Context, item SearchSchedule) {
	if item.Freq != "instant" {
		return
	}
	_, _ = a.store.db.ExecContext(ctx,
		`UPDATE search_schedules SET enabled = 0 WHERE id = ?`, item.ID)
}

// isTextDocType 文字类资料（目前是留言）：不单独下载文件，按内容合并成一个文档。
func isTextDocType(docType string) bool {
	return docType == "留言"
}

// appendTextChunks 把文字类结果整理成「来源 + 内容」的段落，供合并文档使用。
func appendTextChunks(current []string, source string, results []webResult) []string {
	for _, result := range results {
		content := strings.TrimSpace(result.Snippet)
		if content == "" {
			content = strings.TrimSpace(result.Title)
		}
		if content == "" {
			continue
		}
		current = append(current, "来源："+source+"\n"+content)
	}
	return current
}

// saveTextCollections 把一轮里的留言合并成一个本地文档，只生成一条待审核记录。
func (a *App) saveTextCollections(ctx context.Context, item SearchSchedule, buckets map[string][]string) {
	for docType, chunks := range buckets {
		doc, count, err := a.insertTextPending(ctx, item.ID, item.Keyword, item.Category, item.SubCategory, docType, chunks)
		if err != nil || doc.ID == "" || count == 0 {
			continue
		}
		_, _ = a.store.db.ExecContext(ctx,
			`UPDATE search_schedules SET collected = collected + ? WHERE id = ?`, count, item.ID)
	}
}

// filterScheduleSites 只保留任务选中的网站；没选（空）表示不搜索任何网站。
func filterScheduleSites(sites []SearchSite, siteIDs []string) []SearchSite {
	if len(siteIDs) == 0 {
		return nil
	}
	selected := make(map[string]bool, len(siteIDs))
	for _, id := range siteIDs {
		selected[id] = true
	}
	out := make([]SearchSite, 0, len(sites))
	for _, site := range sites {
		if selected[site.ID] {
			out = append(out, site)
		}
	}
	return out
}

// scheduleQuery 一条检索：类型用于区分来源，Query 是实际发给搜索源的语句。
type scheduleQuery struct {
	Type    string
	Query   string
	Keyword string
}

// buildScheduleQueries 把资料选项翻译成检索语句。
// 「全部」= 整站所有资料，只按 site: 抓，不再叠加关键词。
func buildScheduleQueries(keyword string, types []string, domain string) []scheduleQuery {
	domain = siteHostname(domain)
	queries := make([]scheduleQuery, 0, len(types))
	for _, docType := range types {
		if docType == "全部" {
			query := strings.TrimSpace(keyword)
			if domain != "" {
				query = "site:" + domain
			}
			queries = append(queries, scheduleQuery{Type: docType, Query: query, Keyword: keyword})
			continue
		}
		query := keyword + " " + docType
		if domain != "" {
			query += " site:" + domain
		}
		queries = append(queries, scheduleQuery{Type: docType, Query: query, Keyword: keyword})
	}
	return queries
}

// runScheduleQuery 只跑一条检索：哔哩哔哩视频由 collectScheduleVideos 处理；
// 百度等搜索源的视频结果也会进入下载流程，但只有真实视频文件才会被保存。
func runScheduleQuery(ctx context.Context, site SearchSite, query scheduleQuery, freshness string) ([]webResult, error) {
	if query.Type == "视频" && (strings.Contains(strings.ToLower(site.Domain), "bilibili") || strings.Contains(site.Name, "哔哩")) {
		// 已选哔哩哔哩时由 collectScheduleVideos 统一抓取，避免重复下载。
		return nil, nil
	}
	provider := resolveSearchProvider()
	if provider == "demo" {
		return demoSiteSearch(query.Query, 3, []SearchSite{site}), nil
	}
	var results []webResult
	var err error
	if provider == "bing" {
		results, err = bingSearchFreshness(ctx, query.Query, 8, freshness)
	} else if isCSDNDomain(site.Domain) {
		// CSDN 有公开搜索接口，直接用站内检索。
		results, err = csdnSearch(ctx, query.Keyword, 8)
	} else {
		// 百度：关键词 + 站点限定。不限文件类型，网页正文也会抽成 Markdown 资料。
		search := query.Keyword
		if query.Type == "视频" && isBaiduDomain(site.Domain) {
			// 百度作为搜索入口查找可解析的视频页，不把普通网页冒充成视频。
			search += " 视频 site:bilibili.com"
		} else if site.Domain != "" && !isBaiduDomain(site.Domain) {
			search += " site:" + siteHostname(site.Domain)
		}
		results, err = baiduSearch(ctx, search, 12)
		if query.Type == "视频" && isBaiduDomain(site.Domain) {
			videoResults := make([]webResult, 0, len(results))
			for _, result := range results {
				if bvidPattern.MatchString(result.URL) {
					videoResults = append(videoResults, result)
				}
			}
			results = videoResults
			// 百度偶尔不返回 B 站链接，使用视频站公开检索兜底，来源会明确保留为哔哩哔哩。
			if len(results) == 0 {
				videos, videoErr := bilibiliVideoSearch(ctx, query.Keyword, 12, func() bool { return ctx.Err() == nil })
				if videoErr == nil {
					for index := range videos {
						videos[index].Source = "哔哩哔哩"
					}
					results = videos
				}
			}
		}
	}
	if err != nil {
		return nil, err
	}
	for index := range results {
		if results[index].Source == "" {
			results[index].Source = site.Name
		}
		if query.Type == "视频" {
			results[index].Kind = "视频"
		}
	}
	return results, nil
}

// scheduleActive 任务是否还在跑：被暂停或删除时返回 false。
func (a *App) scheduleActive(ctx context.Context, id string) bool {
	var enabled int
	err := a.store.db.QueryRowContext(ctx, `SELECT enabled FROM search_schedules WHERE id = ?`, id).Scan(&enabled)
	return err == nil && enabled == 1
}

// saveScheduleResults 写入待审核并累加「已搜集」数量，前端据此显示进度。
// 文件与网页正文保存到本地后才计入，返回本轮实际新增条数。
// 每下载一条前检查暂停与下载大小上限，任一条件满足就立刻退出循环。
func (a *App) saveScheduleResults(ctx context.Context, item SearchSchedule, results []webResult, budget *downloadBudget) int {
	saved := 0
	for _, result := range results {
		if !a.scheduleActive(ctx, item.ID) || budget.exhausted() {
			return saved
		}
		_, size, ok, err := a.insertPending(ctx, item.ID, item.Keyword, item.Category, item.SubCategory, result, budget.remaining())
		if err != nil || !ok {
			continue
		}
		saved++
		budget.add(size)
		_, _ = a.store.db.ExecContext(ctx,
			`UPDATE search_schedules SET collected = collected + 1 WHERE id = ?`, item.ID)
	}
	return saved
}
