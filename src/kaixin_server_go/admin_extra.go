package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// 管理端补齐（CLD-04 / CLD-07）：
//   资料项目与子项目（关键词、指定站点、定时计划、评审）
//   充值记录与用户余额
//   模型 Token 价格
//   系统自主优化记录（可由第三方写入）
//
// 这些表都在服务端；客户端看不到别人的数据。

func (a *App) ensureExtraSchema(ctx context.Context) error {
	statements := []string{
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS balance_cents BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS recharge_cents BIGINT NOT NULL DEFAULT 0`,

		`CREATE TABLE IF NOT EXISTS material_projects (
			id BIGSERIAL PRIMARY KEY,
			parent_id BIGINT REFERENCES material_projects(id) ON DELETE CASCADE,
			title TEXT NOT NULL,
			keywords TEXT NOT NULL DEFAULT '',
			sites TEXT NOT NULL DEFAULT '',
			schedule TEXT NOT NULL DEFAULT '',
			review TEXT NOT NULL DEFAULT 'pending',
			note TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS material_runs (
			id BIGSERIAL PRIMARY KEY,
			project_id BIGINT NOT NULL REFERENCES material_projects(id) ON DELETE CASCADE,
			triggered_by TEXT NOT NULL DEFAULT 'manual',
			found INTEGER NOT NULL DEFAULT 0,
			detail TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS recharges (
			id BIGSERIAL PRIMARY KEY,
			user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			amount_cents BIGINT NOT NULL,
			channel TEXT NOT NULL DEFAULT 'manual',
			status TEXT NOT NULL DEFAULT 'paid',
			note TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS model_prices (
			id BIGSERIAL PRIMARY KEY,
			model TEXT NOT NULL UNIQUE,
			input_per_million_cents BIGINT NOT NULL DEFAULT 0,
			output_per_million_cents BIGINT NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS optimizations (
			id BIGSERIAL PRIMARY KEY,
			version TEXT NOT NULL,
			category TEXT NOT NULL,
			detail TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT 'manual',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_materials_parent ON material_projects(parent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_recharges_user ON recharges(user_id, created_at DESC)`,
	}
	for _, statement := range statements {
		if _, err := a.db.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

// ---------- 资料项目 ----------

type materialProject struct {
	ID        int64  `json:"id"`
	ParentID  *int64 `json:"parentId"`
	Title     string `json:"title"`
	Keywords  string `json:"keywords"`
	Sites     string `json:"sites"`
	Schedule  string `json:"schedule"`
	Review    string `json:"review"`
	Note      string `json:"note"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	Runs      int    `json:"runs"`
}

func (a *App) listMaterialProjects(ctx context.Context) ([]materialProject, error) {
	rows, err := a.db.Query(ctx, `
		SELECT p.id, p.parent_id, p.title, p.keywords, p.sites, p.schedule, p.review, p.note,
		       p.created_at, p.updated_at,
		       (SELECT COUNT(*) FROM material_runs r WHERE r.project_id = p.id)
		  FROM material_projects p
		 ORDER BY COALESCE(p.parent_id, p.id), p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []materialProject{}
	for rows.Next() {
		var item materialProject
		if err := rows.Scan(&item.ID, &item.ParentID, &item.Title, &item.Keywords, &item.Sites,
			&item.Schedule, &item.Review, &item.Note, &item.CreatedAt, &item.UpdatedAt, &item.Runs); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (a *App) handleMaterialList(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireCatalogEditor(w, r, "admin.materials.list"); !ok {
		return
	}
	items, err := a.listMaterialProjects(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取资料项目失败。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) handleMaterialSave(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireCatalogEditor(w, r, "admin.materials.save")
	if !ok {
		return
	}
	var body struct {
		ID       int64  `json:"id"`
		ParentID *int64 `json:"parentId"`
		Title    string `json:"title"`
		Keywords string `json:"keywords"`
		Sites    string `json:"sites"`
		Schedule string `json:"schedule"`
		Review   string `json:"review"`
		Note     string `json:"note"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确。")
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		fail(w, http.StatusBadRequest, "请填写资料项目名称。")
		return
	}
	review := strings.TrimSpace(body.Review)
	switch review {
	case "", "pending", "approved", "rejected":
	default:
		fail(w, http.StatusBadRequest, "评审状态只能是 pending / approved / rejected。")
		return
	}
	if review == "" {
		review = "pending"
	}

	now := nowText()
	ctx := r.Context()
	if body.ID == 0 {
		_, err := a.db.Exec(ctx, `
			INSERT INTO material_projects(parent_id, title, keywords, sites, schedule, review, note, created_at, updated_at)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8, $8)`,
			body.ParentID, title, strings.TrimSpace(body.Keywords), strings.TrimSpace(body.Sites),
			strings.TrimSpace(body.Schedule), review, strings.TrimSpace(body.Note), now)
		if err != nil {
			fail(w, http.StatusInternalServerError, "新增资料项目失败。")
			return
		}
		a.audit(ctx, actor.Account, "material_create", title)
	} else {
		_, err := a.db.Exec(ctx, `
			UPDATE material_projects
			   SET parent_id = $2, title = $3, keywords = $4, sites = $5, schedule = $6,
			       review = $7, note = $8, updated_at = $9
			 WHERE id = $1`,
			body.ID, body.ParentID, title, strings.TrimSpace(body.Keywords), strings.TrimSpace(body.Sites),
			strings.TrimSpace(body.Schedule), review, strings.TrimSpace(body.Note), now)
		if err != nil {
			fail(w, http.StatusInternalServerError, "保存资料项目失败。")
			return
		}
		a.audit(ctx, actor.Account, "material_update", title)
	}
	writeMessage(w, http.StatusOK, "已保存。")
}

func (a *App) handleMaterialDelete(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireCatalogEditor(w, r, "admin.materials.delete")
	if !ok {
		return
	}
	id, valid := parseInt64(r.PathValue("id"))
	if !valid {
		fail(w, http.StatusBadRequest, "项目不存在。")
		return
	}
	_, _ = a.db.Exec(r.Context(), `DELETE FROM material_projects WHERE id = $1`, id)
	a.audit(r.Context(), actor.Account, "material_delete", strconv.FormatInt(id, 10))
	writeMessage(w, http.StatusOK, "已删除（子项目一并删除）。")
}

// handleMaterialRun 触发一次资料检索。
// 这里只记录「什么时候、按什么关键词、搜哪些站点」，真正的抓取由外部完成；
// 抓回来的结果通过评审状态进入目录，不在这里伪造条目。
func (a *App) handleMaterialRun(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireCatalogEditor(w, r, "admin.materials.run")
	if !ok {
		return
	}
	id, valid := parseInt64(r.PathValue("id"))
	if !valid {
		fail(w, http.StatusBadRequest, "项目不存在。")
		return
	}
	var title, keywords, sites string
	err := a.db.QueryRow(r.Context(),
		`SELECT title, keywords, sites FROM material_projects WHERE id = $1`, id).
		Scan(&title, &keywords, &sites)
	if err != nil {
		fail(w, http.StatusNotFound, "项目不存在。")
		return
	}
	detail := fmt.Sprintf("关键词：%s；站点：%s", defaultText(keywords, "未设置"), defaultText(sites, "未设置"))
	if _, err := a.db.Exec(r.Context(),
		`INSERT INTO material_runs(project_id, triggered_by, found, detail, created_at)
		 VALUES($1, 'manual', 0, $2, $3)`, id, detail, nowText()); err != nil {
		fail(w, http.StatusInternalServerError, "记录检索任务失败。")
		return
	}
	a.audit(r.Context(), actor.Account, "material_run", title)
	writeMessage(w, http.StatusOK, "已记录一次检索任务；抓取结果由外部写入后可进入评审。")
}

func defaultText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// ---------- 充值 ----------

type rechargeRecord struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"userId"`
	Account     string `json:"account"`
	AmountCents int64  `json:"amountCents"`
	Channel     string `json:"channel"`
	Status      string `json:"status"`
	Note        string `json:"note"`
	CreatedAt   string `json:"createdAt"`
}

func (a *App) handleRechargeList(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT r.id, r.user_id, COALESCE(u.account, ''), r.amount_cents, r.channel, r.status, r.note, r.created_at
		  FROM recharges r LEFT JOIN users u ON u.id = r.user_id
		 ORDER BY r.id DESC LIMIT 200`)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取充值记录失败。")
		return
	}
	defer rows.Close()

	items := []rechargeRecord{}
	for rows.Next() {
		var item rechargeRecord
		if err := rows.Scan(&item.ID, &item.UserID, &item.Account, &item.AmountCents,
			&item.Channel, &item.Status, &item.Note, &item.CreatedAt); err != nil {
			fail(w, http.StatusInternalServerError, "读取充值记录失败。")
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) handleRechargeCreate(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Account     string `json:"account"`
		AmountCents int64  `json:"amountCents"`
		Channel     string `json:"channel"`
		Note        string `json:"note"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确。")
		return
	}
	if strings.TrimSpace(body.Account) == "" || body.AmountCents <= 0 {
		fail(w, http.StatusBadRequest, "请填写账号与正整数金额（分）。")
		return
	}
	channel := strings.TrimSpace(body.Channel)
	switch channel {
	case "wechat", "alipay", "manual":
	default:
		fail(w, http.StatusBadRequest, "渠道只能是 wechat / alipay / manual。")
		return
	}

	ctx := r.Context()
	var userID int64
	if err := a.db.QueryRow(ctx, `SELECT id FROM users WHERE account = $1`, strings.TrimSpace(body.Account)).Scan(&userID); err != nil {
		fail(w, http.StatusNotFound, "账号不存在。")
		return
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		fail(w, http.StatusInternalServerError, "充值失败。")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`INSERT INTO recharges(user_id, amount_cents, channel, status, note, created_at)
		 VALUES($1, $2, $3, 'paid', $4, $5)`,
		userID, body.AmountCents, channel, strings.TrimSpace(body.Note), nowText()); err != nil {
		fail(w, http.StatusInternalServerError, "写入充值记录失败。")
		return
	}
	if _, err := tx.Exec(ctx,
		`UPDATE users SET balance_cents = balance_cents + $2, recharge_cents = recharge_cents + $2 WHERE id = $1`,
		userID, body.AmountCents); err != nil {
		fail(w, http.StatusInternalServerError, "更新余额失败。")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		fail(w, http.StatusInternalServerError, "充值失败。")
		return
	}
	a.audit(ctx, actor.Account, "recharge", fmt.Sprintf("%s +%d 分 (%s)", body.Account, body.AmountCents, channel))
	writeMessage(w, http.StatusOK, "充值已记录。")
}

// ---------- Token 价格 ----------

type modelPrice struct {
	Model                 string `json:"model"`
	InputPerMillionCents  int64  `json:"inputPerMillionCents"`
	OutputPerMillionCents int64  `json:"outputPerMillionCents"`
	UpdatedAt             string `json:"updatedAt"`
}

func (a *App) handlePriceList(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	rows, err := a.db.Query(r.Context(),
		`SELECT model, input_per_million_cents, output_per_million_cents, updated_at FROM model_prices ORDER BY model`)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取价格失败。")
		return
	}
	defer rows.Close()
	items := []modelPrice{}
	for rows.Next() {
		var item modelPrice
		if err := rows.Scan(&item.Model, &item.InputPerMillionCents, &item.OutputPerMillionCents, &item.UpdatedAt); err != nil {
			fail(w, http.StatusInternalServerError, "读取价格失败。")
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) handlePriceSave(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Model                 string `json:"model"`
		InputPerMillionCents  int64  `json:"inputPerMillionCents"`
		OutputPerMillionCents int64  `json:"outputPerMillionCents"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.Model) == "" {
		fail(w, http.StatusBadRequest, "请填写模型名称。")
		return
	}
	if body.InputPerMillionCents < 0 || body.OutputPerMillionCents < 0 {
		fail(w, http.StatusBadRequest, "价格不能为负。")
		return
	}
	_, err := a.db.Exec(r.Context(), `
		INSERT INTO model_prices(model, input_per_million_cents, output_per_million_cents, updated_at)
		VALUES($1, $2, $3, $4)
		ON CONFLICT (model) DO UPDATE SET
		  input_per_million_cents = excluded.input_per_million_cents,
		  output_per_million_cents = excluded.output_per_million_cents,
		  updated_at = excluded.updated_at`,
		strings.TrimSpace(body.Model), body.InputPerMillionCents, body.OutputPerMillionCents, nowText())
	if err != nil {
		fail(w, http.StatusInternalServerError, "保存价格失败。")
		return
	}
	a.audit(r.Context(), actor.Account, "price_save", body.Model)
	writeMessage(w, http.StatusOK, "已保存。")
}

// ---------- 自主优化记录 ----------

type optimization struct {
	ID        int64  `json:"id"`
	Version   string `json:"version"`
	Category  string `json:"category"`
	Detail    string `json:"detail"`
	Source    string `json:"source"`
	CreatedAt string `json:"createdAt"`
}

func (a *App) handleOptimizationList(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	rows, err := a.db.Query(r.Context(),
		`SELECT id, version, category, detail, source, created_at FROM optimizations ORDER BY id DESC LIMIT 200`)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取优化记录失败。")
		return
	}
	defer rows.Close()
	items := []optimization{}
	for rows.Next() {
		var item optimization
		if err := rows.Scan(&item.ID, &item.Version, &item.Category, &item.Detail, &item.Source, &item.CreatedAt); err != nil {
			fail(w, http.StatusInternalServerError, "读取优化记录失败。")
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) handleOptimizationCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version  string `json:"version"`
		Category string `json:"category"`
		Detail   string `json:"detail"`
		Source   string `json:"source"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确。")
		return
	}
	if strings.TrimSpace(body.Version) == "" || strings.TrimSpace(body.Detail) == "" {
		fail(w, http.StatusBadRequest, "请填写版本号与优化内容。")
		return
	}
	source := strings.TrimSpace(body.Source)
	if source == "" {
		source = "manual"
	}
	_, err := a.db.Exec(r.Context(),
		`INSERT INTO optimizations(version, category, detail, source, created_at) VALUES($1, $2, $3, $4, $5)`,
		strings.TrimSpace(body.Version), strings.TrimSpace(body.Category), strings.TrimSpace(body.Detail), source, nowText())
	if err != nil {
		fail(w, http.StatusInternalServerError, "记录优化失败。")
		return
	}
	a.audit(r.Context(), "system", "optimization", body.Version+" "+body.Category)
	writeMessage(w, http.StatusOK, "已记录。")
}

// 定时任务：每分钟检查是否有到点的资料项目，记录一条待检索任务。
// 抓取动作不在这里做（需要外部抓取能力），只保证提醒与留痕。
func (a *App) startMaterialScheduler(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.runDueMaterialPlans(ctx)
			}
		}
	}()
}

func (a *App) runDueMaterialPlans(ctx context.Context) {
	now := time.Now()
	stamp := fmt.Sprintf("%02d:%02d", now.Hour(), now.Minute())
	rows, err := a.db.Query(ctx,
		`SELECT id, title, keywords, sites FROM material_projects WHERE schedule <> ''`)
	if err != nil {
		return
	}
	defer rows.Close()

	type due struct {
		id       int64
		title    string
		keywords string
		sites    string
	}
	pending := []due{}
	for rows.Next() {
		var item due
		if err := rows.Scan(&item.id, &item.title, &item.keywords, &item.sites); err != nil {
			return
		}
		pending = append(pending, item)
	}
	rows.Close()

	for _, item := range pending {
		// 只认「HH:MM」形式的时刻，其他写法跳过
		rows2, err := a.db.Query(ctx, `SELECT schedule FROM material_projects WHERE id = $1`, item.id)
		if err != nil {
			continue
		}
		var schedule string
		if rows2.Next() {
			_ = rows2.Scan(&schedule)
		}
		rows2.Close()
		if !strings.Contains(schedule, stamp) {
			continue
		}
		// 同一分钟同一项目只记一次
		var count int
		_ = a.db.QueryRow(ctx,
			`SELECT COUNT(*) FROM material_runs WHERE project_id = $1 AND created_at LIKE $2`,
			item.id, now.Format("2006-01-02 15:04")+"%").Scan(&count)
		if count > 0 {
			continue
		}
		detail := fmt.Sprintf("关键词：%s；站点：%s", defaultText(item.keywords, "未设置"), defaultText(item.sites, "未设置"))
		_, _ = a.db.Exec(ctx,
			`INSERT INTO material_runs(project_id, triggered_by, found, detail, created_at)
			 VALUES($1, 'schedule', 0, $2, $3)`, item.id, detail, nowText())
	}
}

// handleOptimizationExternal 供第三方软件写入优化记录（CLD-07）。
// 需要一个共享密钥；没配置就明确拒绝，不开放匿名写入。
func (a *App) handleOptimizationExternal(w http.ResponseWriter, r *http.Request) {
	expected := strings.TrimSpace(os.Getenv("KAIXIN_OPTIMIZATION_KEY"))
	if expected == "" {
		fail(w, http.StatusServiceUnavailable, "未配置 KAIXIN_OPTIMIZATION_KEY，外部写入未开启。")
		return
	}
	if strings.TrimSpace(r.Header.Get("X-Optimization-Key")) != expected {
		a.denyWithAudit(w, r, nil, "optimizations.external", "", "bad_key")
		return
	}
	var body struct {
		Version  string `json:"version"`
		Category string `json:"category"`
		Detail   string `json:"detail"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确。")
		return
	}
	if strings.TrimSpace(body.Version) == "" || strings.TrimSpace(body.Detail) == "" {
		fail(w, http.StatusBadRequest, "请填写版本号与优化内容。")
		return
	}
	if _, err := a.db.Exec(r.Context(),
		`INSERT INTO optimizations(version, category, detail, source, created_at) VALUES($1, $2, $3, 'external', $4)`,
		strings.TrimSpace(body.Version), strings.TrimSpace(body.Category), strings.TrimSpace(body.Detail), nowText()); err != nil {
		fail(w, http.StatusInternalServerError, "记录优化失败。")
		return
	}
	a.audit(r.Context(), "external", "optimization", body.Version+" "+body.Category)
	writeMessage(w, http.StatusOK, "已记录。")
}
