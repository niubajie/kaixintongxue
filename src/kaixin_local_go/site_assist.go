package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// SiteAssistSnapshot 是前端分步修复向导的唯一状态来源。
type SiteAssistSnapshot struct {
	SessionID     string `json:"sessionId"`
	SiteID        string `json:"siteId"`
	ScheduleID    string `json:"scheduleId,omitempty"`
	State         string `json:"state"`
	Message       string `json:"message"`
	Keyword       string `json:"keyword"`
	EntryURL      string `json:"entryUrl"`
	ResultCount   int    `json:"resultCount"`
	DownloadReady bool   `json:"downloadReady"`
	FailureCode   string `json:"failureCode,omitempty"`
}

type siteAssistSession struct {
	mu              sync.RWMutex
	actionMu        sync.Mutex
	browserMu       sync.Mutex
	id              string
	siteID          string
	scheduleID      string
	keyword         string
	domain          string
	entryURL        string
	state           string
	message         string
	failureCode     string
	results         []webResult
	ready           bool
	delivered       bool // 是否已把验证过的结果交给下载器
	giveUp          bool // 用户是否明确放弃本次修复（不再自动重开）
	ctx             context.Context
	cancel          context.CancelFunc
	closeOnce       sync.Once
	loginMonitoring bool
}

func (s *siteAssistSession) set(state, message string) {
	s.mu.Lock()
	s.state, s.message = state, message
	s.mu.Unlock()
}

func (s *siteAssistSession) setFailure(code string) {
	s.mu.Lock()
	s.failureCode = code
	s.mu.Unlock()
}

func (s *siteAssistSession) snapshot() SiteAssistSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return SiteAssistSnapshot{
		SessionID: s.id, SiteID: s.siteID, ScheduleID: s.scheduleID,
		State: s.state, Message: s.message, Keyword: s.keyword,
		EntryURL: s.entryURL, ResultCount: len(s.results), DownloadReady: s.ready,
		FailureCode: s.failureCode,
	}
}

func (s *siteAssistSession) browserContext() context.Context {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ctx
}

func (s *siteAssistSession) runBrowser(action func(context.Context) error) error {
	s.browserMu.Lock()
	defer s.browserMu.Unlock()
	ctx := s.browserContext()
	if ctx == nil || ctx.Err() != nil {
		return fmt.Errorf("浏览器窗口已关闭")
	}
	return action(ctx)
}

func (s *siteAssistSession) close(state, message string) {
	s.set(state, message)
	s.closeOnce.Do(func() {
		s.mu.RLock()
		cancel := s.cancel
		s.mu.RUnlock()
		if cancel != nil {
			cancel()
		}
	})
}

var assistSessions = struct {
	sync.RWMutex
	byID   map[string]*siteAssistSession
	active map[string]string
}{byID: map[string]*siteAssistSession{}, active: map[string]string{}}

func lookupAssist(id string) *siteAssistSession {
	assistSessions.RLock()
	defer assistSessions.RUnlock()
	return assistSessions.byID[id]
}

func activeAssist(siteID string) *siteAssistSession {
	assistSessions.RLock()
	id := assistSessions.active[siteID]
	assistSessions.RUnlock()
	if id == "" {
		return nil
	}
	session := lookupAssist(id)
	if session == nil {
		return nil
	}
	state := session.snapshot().State
	if state == "failed" || state == "closed" || state == "downloading" {
		return nil
	}
	return session
}

func storeAssist(session *siteAssistSession) {
	assistSessions.Lock()
	assistSessions.byID[session.id] = session
	assistSessions.active[session.siteID] = session.id
	assistSessions.Unlock()
}

func assistSiteEntry(domain string) string {
	if strings.HasPrefix(domain, "http://") || strings.HasPrefix(domain, "https://") {
		return domain
	}
	return "https://" + domain
}

// handleSiteAssistStart 打开一个有头浏览器，后续所有步骤都复用这个窗口。
func (a *App) handleSiteAssistStart(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	var request struct {
		SiteID     string `json:"siteId"`
		ScheduleID string `json:"scheduleId"`
		Keyword    string `json:"keyword"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	request.SiteID = strings.TrimSpace(request.SiteID)
	var domain string
	if err := a.store.db.QueryRowContext(r.Context(), `SELECT domain FROM search_sites WHERE id = ?`, request.SiteID).Scan(&domain); err != nil {
		fail(w, http.StatusNotFound, "网站不存在")
		return
	}
	domain = normalizeDomain(domain)
	if domain == "" || browserExecPath() == "" {
		fail(w, http.StatusFailedDependency, "网站地址无效，或本机没有 Edge/Chrome")
		return
	}
	if existing := activeAssist(request.SiteID); existing != nil {
		writeJSON(w, http.StatusOK, existing.snapshot())
		return
	}
	keyword := strings.TrimSpace(request.Keyword)
	if keyword == "" {
		keyword = "测试"
	}
	session := &siteAssistSession{
		id: newRandomID(), siteID: request.SiteID, scheduleID: strings.TrimSpace(request.ScheduleID),
		keyword: keyword, domain: domain, entryURL: assistSiteEntry(domain),
		state: "starting", message: "第一步：正在打开网站…",
	}
	storeAssist(session)
	go a.runSiteAssist(session)
	writeJSON(w, http.StatusOK, session.snapshot())
}

func (a *App) runSiteAssist(session *siteAssistSession) {
	profileName := strings.NewReplacer(":", "_", "/", "_", "\\", "_").Replace(strings.Split(session.domain, "/")[0])
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browserExecPath()),
		chromedp.UserDataDir(filepath.Join(a.root, "browser-profile", profileName)),
		chromedp.Flag("headless", false),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()
	session.mu.Lock()
	session.ctx, session.cancel = browserCtx, browserCancel
	session.mu.Unlock()
	if err := chromedp.Run(browserCtx, chromedp.Navigate(session.entryURL)); err != nil {
		session.set("failed", "网站打开失败："+err.Error())
		return
	}
	session.set("awaiting_open", "第一步：请查看弹出的浏览器。网站能正常打开吗？")
	<-browserCtx.Done()
	if state := session.snapshot().State; state != "closed" && state != "downloading" {
		session.set("closed", "浏览器窗口已关闭，修复已停止")
	}
	// 浏览器关掉时若还没交付真实结果，且用户没明确放弃，就再开一轮修复——异常随时重新进入修复。
	a.reopenAssist(session)
}

// reopenAssist 修复会话结束且未交付结果时，决定是否自动重开一轮修复。
// 达到连续重开上限或用户明确放弃时，存一个空结果唤醒下载循环，给出“多次修复失败”事项。
func (a *App) reopenAssist(session *siteAssistSession) {
	if session.scheduleID == "" {
		return // 没有关联任务的手动向导不自动重开
	}
	if _, running := a.scheduleRuns.Load(session.scheduleID); !running {
		return // 任务已暂停或结束，不再重开（用户可再次点「开始」触发新一轮）
	}
	session.mu.RLock()
	delivered := session.delivered
	giveUp := session.giveUp
	session.mu.RUnlock()
	if delivered {
		a.scheduleAssistReopens.Delete(session.scheduleID)
		return
	}
	if giveUp {
		a.storeAssistResults(session.scheduleID+"|"+session.siteID, []webResult{})
		return
	}
	count := 0
	if value, ok := a.scheduleAssistReopens.Load(session.scheduleID); ok {
		count, _ = value.(int)
	}
	const maxReopens = 4
	if count >= maxReopens {
		a.storeAssistResults(session.scheduleID+"|"+session.siteID, []webResult{})
		return
	}
	count++
	a.scheduleAssistReopens.Store(session.scheduleID, count)
	a.publishIssue(session.scheduleID, session.siteID, "failed",
		"修复未成功，正在重新打开修复向导；请在浏览器里处理弹窗、验证码或登录后继续",
		"", "retry", a.bumpRunNonce(session.scheduleID))
}

// handleSiteAssistStatus 返回当前步骤快照。
func (a *App) handleSiteAssistStatus(w http.ResponseWriter, r *http.Request) {
	session := lookupAssist(strings.TrimSpace(r.URL.Query().Get("sessionId")))
	if session == nil {
		fail(w, http.StatusNotFound, "修复会话不存在")
		return
	}
	writeJSON(w, http.StatusOK, session.snapshot())
}

// handleSiteAssistOpen 用户反馈网站能否打开。
func (a *App) handleSiteAssistOpen(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SessionID string `json:"sessionId"`
		Opened    bool   `json:"opened"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	session := lookupAssist(request.SessionID)
	if session == nil {
		fail(w, http.StatusNotFound, "修复会话不存在")
		return
	}
	if !request.Opened {
		session.set("failed", "网站在浏览器中也无法打开，请检查地址或网络")
		writeJSON(w, http.StatusOK, session.snapshot())
		return
	}
	session.set("detecting_search", "第二步：正在查找搜索入口…")
	go a.detectAndSearch(session, false)
	writeJSON(w, http.StatusOK, session.snapshot())
}

// handleSiteAssistEntry 用户确认有无搜索入口，或选择手动填写搜索页地址。
func (a *App) handleSiteAssistEntry(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SessionID string `json:"sessionId"`
		Decision  string `json:"decision"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	session := lookupAssist(request.SessionID)
	if session == nil {
		fail(w, http.StatusNotFound, "修复会话不存在")
		return
	}
	switch request.Decision {
	case "advanced":
		session.set("detecting_search", "正在深度识别搜索入口（动态元素、展开按钮和通用输入框）…")
		go a.detectAndSearch(session, true)
	case "provideUrl":
		session.set("awaiting_search_url", "请填写包含搜索入口的完整页面地址")
	case "unsupported":
		session.set("failed", "已确认此网站没有搜索入口，无法按关键词自动搜集资料")
	default:
		fail(w, http.StatusBadRequest, "未知操作")
		return
	}
	writeJSON(w, http.StatusOK, session.snapshot())
}

// handleSiteAssistSearchURL 保存用户提供的搜索页地址，并在同一窗口继续识别。
func (a *App) handleSiteAssistSearchURL(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SessionID string `json:"sessionId"`
		URL       string `json:"url"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	session := lookupAssist(request.SessionID)
	if session == nil || a.store == nil {
		fail(w, http.StatusNotFound, "修复会话不存在")
		return
	}
	domain := normalizeDomain(request.URL)
	if domain == "" {
		fail(w, http.StatusBadRequest, "搜索页地址格式不正确")
		return
	}
	if _, err := a.store.db.ExecContext(r.Context(), `UPDATE search_sites SET domain = ? WHERE id = ?`, domain, session.siteID); err != nil {
		fail(w, http.StatusInternalServerError, "保存搜索页地址失败："+err.Error())
		return
	}
	session.mu.Lock()
	session.domain, session.entryURL = domain, assistSiteEntry(domain)
	session.mu.Unlock()
	if err := session.runBrowser(func(ctx context.Context) error {
		return chromedp.Run(ctx, chromedp.Navigate(session.entryURL), chromedp.WaitReady("body"))
	}); err != nil {
		session.set("failed", "搜索页打开失败："+err.Error())
		writeJSON(w, http.StatusOK, session.snapshot())
		return
	}
	session.set("detecting_search", "搜索页已打开，正在重新查找搜索入口…")
	go a.detectAndSearch(session, false)
	writeJSON(w, http.StatusOK, session.snapshot())
}

// handleSiteAssistCancel 取消修复并关闭受控浏览器，不影响已下载资料。
// giveUp=true 表示用户明确放弃本站修复：不再自动重开，直接结束本站并给出事项。
func (a *App) handleSiteAssistCancel(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SessionID string `json:"sessionId"`
		GiveUp    bool   `json:"giveUp"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	if session := lookupAssist(request.SessionID); session != nil {
		if request.GiveUp {
			session.mu.Lock()
			session.giveUp = true
			session.mu.Unlock()
		}
		session.close("closed", "已取消网站修复")
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleSiteAssistRetry 失败后在同一个浏览器窗口里重新搜索（保留用户已完成的验证码/登录状态）。
func (a *App) handleSiteAssistRetry(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SessionID string `json:"sessionId"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	session := lookupAssist(request.SessionID)
	if session == nil {
		fail(w, http.StatusNotFound, "修复会话不存在")
		return
	}
	if !a.scheduleAssistAlive(session) {
		// 浏览器已关：触发重开，让前端弹出新窗口。
		a.reopenAssist(session)
		writeJSON(w, http.StatusOK, session.snapshot())
		return
	}
	session.setFailure("")
	session.set("detecting_search", "正在重新搜索（保留当前浏览器状态）…")
	go func() {
		_ = session.runBrowser(func(ctx context.Context) error {
			return chromedp.Run(ctx, chromedp.Navigate(session.entryURL), chromedp.WaitReady("body"))
		})
		a.detectAndSearch(session, true)
	}()
	writeJSON(w, http.StatusOK, session.snapshot())
}

// handleSiteAssistLogin 用户确认此站需要登录时，显式进入登录等待步骤。
func (a *App) handleSiteAssistLogin(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SessionID string `json:"sessionId"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	session := lookupAssist(request.SessionID)
	if session == nil {
		fail(w, http.StatusNotFound, "修复会话不存在")
		return
	}
	a.beginAssistLogin(session)
	writeJSON(w, http.StatusOK, session.snapshot())
}

// handleSiteAssistLoginConfirm 未知登录标记站的人工确认；保存后仍要重新搜索验证。
func (a *App) handleSiteAssistLoginConfirm(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SessionID string `json:"sessionId"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	session := lookupAssist(request.SessionID)
	if session == nil {
		fail(w, http.StatusNotFound, "修复会话不存在")
		return
	}
	if err := a.completeAssistLogin(session); err != nil {
		fail(w, http.StatusPreconditionFailed, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, session.snapshot())
}

func (a *App) completeAssistLogin(session *siteAssistSession) error {
	cookies, err := a.assistCookies(session)
	if err != nil || len(cookies) == 0 {
		return fmt.Errorf("还没有读到登录信息，请先在浏览器中完成登录")
	}
	markers := loginMarkerCookies(session.domain)
	if len(markers) > 0 && !hasCookieMarkers(cookies, markers) {
		return fmt.Errorf("仍未检测到登录成功，会继续等待")
	}
	if err := a.saveSiteCookie(session.siteID, session.domain, buildCookieHeader(cookies)); err != nil {
		return err
	}
	session.set("login_verifying", "登录信息已保存，正在返回原页面重新搜索…")
	go a.resumeAssistAfterLogin(session)
	return nil
}

func (a *App) resumeAssistAfterLogin(session *siteAssistSession) {
	_ = session.runBrowser(func(ctx context.Context) error {
		return chromedp.Run(ctx, chromedp.Navigate(session.entryURL), chromedp.WaitReady("body"))
	})
	a.detectAndSearch(session, true)
}

func (a *App) assistCookies(session *siteAssistSession) ([]*network.Cookie, error) {
	var cookies []*network.Cookie
	err := session.runBrowser(func(ctx context.Context) error {
		var err error
		cookies, err = collectSiteCookies(ctx, session.domain)
		return err
	})
	return cookies, err
}

// detectAndSearch 识别搜索框、填入关键词、提交并验证真实可下载结果。
// 增强：先关弹窗，提交后校验结果是否真的出现（没出现就换提交方式重试），
// 遇到验证码/安全验证/登录墙时给出明确 failureCode，不再静默判失败退出。
func (a *App) detectAndSearch(session *siteAssistSession, advanced bool) {
	session.actionMu.Lock()
	defer session.actionMu.Unlock()
	session.setFailure("")

	// 整体看门狗：任何一步异常卡顿，4 分钟后必然给出明确提示，绝不静默停在“正在搜索”。
	watchdog := make(chan struct{})
	defer close(watchdog)
	var browserDone <-chan struct{}
	if bctx := session.browserContext(); bctx != nil {
		browserDone = bctx.Done()
	}
	go func() {
		select {
		case <-watchdog:
		case <-browserDone:
		case <-time.After(4 * time.Minute):
			session.setFailure("timeout")
			session.set("failed", "修复流程超时（页面加载过慢或页面元素过多）；请在浏览器里关闭弹窗后点「重新修复」")
		}
	}()

	// 0. 先关掉常见弹窗（Cookie 同意、登录提示、遮罩），否则输入框可能被遮住点不到。
	// 关弹窗失败（脚本超时等）不视为浏览器关闭，继续往下走。
	session.set("detecting_search", "正在关闭页面弹窗并定位搜索入口…")
	_ = a.assistDismissPopups(session)
	if !a.scheduleAssistAlive(session) {
		session.set("closed", "浏览器窗口已关闭，修复停止")
		return
	}
	// 0b. 先看是不是一上来就被验证码/登录墙挡住。
	if blocker := a.assistDetectBlocker(session); blocker.captcha {
		session.setFailure("captcha")
		session.set("failed", "页面出现验证码或安全验证，请在弹出的浏览器窗口里完成验证，然后点击「重新修复」")
		return
	}

	// 1. 查找搜索入口。
	var found struct {
		Found bool `json:"found"`
		Login bool `json:"login"`
	}
	err := a.evalAssist(session, fmt.Sprintf("(%s)(%t)", assistFindSearchScript, advanced), &found, 12*time.Second)
	if err != nil && !a.scheduleAssistAlive(session) {
		session.set("closed", "浏览器窗口已关闭，修复停止")
		return
	}
	if !found.Found && advanced {
		// 高级模式先尝试展开“搜索”按钮，再给动态输入框一次出现机会。
		_ = session.runBrowser(func(ctx context.Context) error {
			runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			return chromedp.Run(runCtx, chromedp.Evaluate(assistExpandSearchScript, nil), chromedp.Sleep(1200*time.Millisecond),
				chromedp.Evaluate(fmt.Sprintf("(%s)(true)", assistFindSearchScript), &found))
		})
		_ = a.assistDismissPopups(session)
	}
	if found.Login {
		a.beginAssistLogin(session)
		return
	}
	if !found.Found {
		if blocker := a.assistDetectBlocker(session); blocker.captcha {
			session.setFailure("captcha")
			session.set("failed", "页面被验证码或安全验证挡住，找不到搜索框。请在浏览器里完成验证后点击「重新修复」")
			return
		}
		if advanced {
			session.setFailure("no_search_box")
			session.set("failed", "已进行深度识别，仍无法可靠定位搜索窗口；可能被弹窗或验证码挡住，可点击「重新修复」再试，或改填其他搜索页地址")
			return
		}
		session.set("awaiting_entry_choice", "未找到搜索窗口。请查验页面是否有搜索入口：有则进行深度识别，没有可填写正确的搜索页地址")
		return
	}

	// 2. 提交搜索，并校验结果是否真的加载出来（URL 变化或结果链接增多）。
	session.set("searching", "已找到搜索入口，正在填入关键词并检索…")
	if submitted, err := a.assistSubmitAndVerify(session, advanced); err != nil {
		session.set("failed", "搜索提交失败："+err.Error())
		return
	} else if !submitted {
		// 提交了但页面没反应：很可能被弹窗/验证码拦住。
		if blocker := a.assistDetectBlocker(session); blocker.captcha {
			session.setFailure("captcha")
			session.set("failed", "提交搜索后出现验证码或安全验证，结果没有加载。请在浏览器里完成验证后点击「重新修复」")
			return
		}
		session.setFailure("submit_blocked")
		session.set("failed", "已填入关键词并提交，但页面没有返回搜索结果（可能被弹窗、验证码或登录提示拦住）；请在浏览器里处理后点击「重新修复」")
		return
	}

	// 3. 提取并验证真实可下载结果。
	var results []webResult
	err = a.evalAssist(session, assistExtractResultsScript, &results, 12*time.Second)
	session.mu.Lock()
	session.results, session.ready = results, false
	session.mu.Unlock()
	if err != nil || len(results) == 0 {
		if blocker := a.assistDetectBlocker(session); blocker.captcha {
			session.setFailure("captcha")
			session.set("failed", "搜索后页面出现验证码或安全验证，没有拿到结果。请在浏览器里完成验证后点击「重新修复」")
			return
		}
		session.setFailure("no_results")
		session.set("failed", "搜索后没有找到相关资料，请更换关键词或检查搜索页地址；若有弹窗请先关闭再点「重新修复」")
		return
	}
	// 记住搜索列表页地址：验证完结果页后直接导航回来，比 NavigateBack 可靠（后者会被 beforeunload 挂住）。
	var searchURL string
	_ = a.evalAssist(session, "location.href", &searchURL, 8*time.Second)

	usable := make([]webResult, 0, 3)
	limit := len(results)
	if limit > 3 {
		limit = 3
	}
	for index := 0; index < limit; index++ {
		result := results[index]
		if len(usable) >= 1 {
			break
		}
		if !a.scheduleAssistAlive(session) {
			return
		}
		session.set("searching", fmt.Sprintf("已找到 %d 条结果，正在验证第 %d/%d 条是否可下载…", len(results), index+1, limit))
		probeCtx, probeCancel := context.WithTimeout(context.Background(), 8*time.Second)
		downloadable := probeDownloadable(probeCtx, result.URL)
		probeCancel()
		if downloadable {
			usable = append(usable, result)
			continue
		}
		// 动态网站必须在同一个已打开的浏览器里读取结果页，纯 HTTP 失败不等于需要登录。
		var html string
		err := session.runBrowser(func(ctx context.Context) error {
			runCtx, cancel := context.WithTimeout(ctx, 18*time.Second)
			defer cancel()
			return chromedp.Run(runCtx,
				chromedp.Navigate(result.URL), chromedp.WaitReady("body"), chromedp.Sleep(1000*time.Millisecond),
				chromedp.OuterHTML("html", &html),
			)
		})
		if err == nil {
			if title, markdown, ok := articleFromHTML(result.URL, []byte(html)); ok {
				result.ArticleTitle, result.ArticleMarkdown = title, markdown
				usable = append(usable, result)
			}
		}
		// 回搜索列表：优先直接导航回列表页，失败再尝试回退；两步都带硬超时。
		if searchURL != "" {
			_ = session.runBrowser(func(ctx context.Context) error {
				runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()
				return chromedp.Run(runCtx, chromedp.Navigate(searchURL), chromedp.WaitReady("body"))
			})
		} else {
			_ = session.runBrowser(func(ctx context.Context) error {
				runCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
				defer cancel()
				return chromedp.Run(runCtx, chromedp.NavigateBack())
			})
		}
	}
	if len(usable) == 0 {
		if blocker := a.assistDetectBlocker(session); blocker.captcha {
			session.setFailure("captcha")
			session.set("failed", "打开结果页时出现验证码或安全验证，无法读取正文。请在浏览器里完成验证后点击「重新修复」")
			return
		}
		var needLogin bool
		_ = a.evalAssist(session, assistLoginPageScript, &needLogin, 8*time.Second)
		if needLogin || (len(loginMarkerCookies(session.domain)) > 0 && memoryCookie(siteHostname(session.domain)) == "") {
			a.beginAssistLogin(session)
			return
		}
		session.setFailure("no_usable")
		session.set("failed", fmt.Sprintf("搜索到 %d 个链接，但前几条没有可下载文件或可保存正文；请更换关键词，或手动确认是否需要登录", len(results)))
		return
	}
	session.mu.Lock()
	session.results = usable
	session.ready = true
	session.delivered = true
	session.mu.Unlock()
	if session.scheduleID != "" {
		// 交付结果并唤醒等待中的下载循环；同时重置连续重开计数。
		a.scheduleAssistReopens.Delete(session.scheduleID)
		a.storeAssistResults(session.scheduleID+"|"+session.siteID, usable)
		session.set("ready", "已找到可下载资料，正在恢复原搜索任务…")
		return
	}
	session.set("ready", "已找到可下载资料，可以开始搜集任务")
}

// evalAssist 在受控浏览器里执行一段脚本，带硬超时，避免任何一步静默卡死。
func (a *App) evalAssist(session *siteAssistSession, script string, out any, timeout time.Duration) error {
	return session.runBrowser(func(ctx context.Context) error {
		runCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return chromedp.Run(runCtx, chromedp.Evaluate(script, out))
	})
}

// assistDismissPopups 关闭常见遮罩弹窗。脚本已做选择器限流，整体再兜 8 秒超时。
func (a *App) assistDismissPopups(session *siteAssistSession) error {
	return session.runBrowser(func(ctx context.Context) error {
		runCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		return chromedp.Run(runCtx,
			chromedp.Evaluate(assistDismissPopupsScript, nil),
			chromedp.Sleep(400*time.Millisecond),
		)
	})
}

type assistBlocker struct {
	captcha bool
	login   bool
}

// assistDetectBlocker 判断当前页是否被验证码 / 登录墙挡住。
func (a *App) assistDetectBlocker(session *siteAssistSession) assistBlocker {
	var out struct {
		Captcha bool `json:"captcha"`
		Login   bool `json:"login"`
	}
	_ = a.evalAssist(session, assistDetectBlockerScript, &out, 8*time.Second)
	return assistBlocker{captcha: out.Captcha, login: out.Login}
}

// scheduleAssistAlive 会话的浏览器是否还活着。
func (a *App) scheduleAssistAlive(session *siteAssistSession) bool {
	ctx := session.browserContext()
	return ctx != nil && ctx.Err() == nil
}

// assistSubmitAndVerify 填入关键词、提交，并确认结果页真的加载了；没加载就换方式重试。
func (a *App) assistSubmitAndVerify(session *siteAssistSession, advanced bool) (bool, error) {
	keywordJSON, _ := json.Marshal(session.keyword)
	capture := func() (string, int, error) {
		var state struct {
			URL   string `json:"url"`
			Count int    `json:"count"`
		}
		err := a.evalAssist(session, assistCaptureStateScript, &state, 10*time.Second)
		return state.URL, state.Count, err
	}
	beforeURL, beforeCount, err := capture()
	if err != nil {
		return false, err
	}
	if err := session.runBrowser(func(ctx context.Context) error {
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return chromedp.Run(runCtx,
			chromedp.Evaluate(fmt.Sprintf("(%s)(%s)", assistSubmitSearchScript, keywordJSON), nil),
			chromedp.Sleep(2500*time.Millisecond),
		)
	}); err != nil && !a.scheduleAssistAlive(session) {
		return false, err
	}
	_ = a.assistDismissPopups(session)
	afterURL, afterCount, err := capture()
	if err == nil && (afterURL != beforeURL || afterCount > beforeCount || afterCount > 0) {
		return true, nil
	}
	// 第一次提交没反应：再关一次弹窗，改用“点搜索按钮 / 回车”重试，并多等一会儿。
	_ = a.assistDismissPopups(session)
	if err := session.runBrowser(func(ctx context.Context) error {
		runCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
		defer cancel()
		return chromedp.Run(runCtx,
			chromedp.Evaluate(assistRetrySubmitScript, nil),
			chromedp.Sleep(3500*time.Millisecond),
		)
	}); err != nil && !a.scheduleAssistAlive(session) {
		return false, err
	}
	_ = a.assistDismissPopups(session)
	afterURL, afterCount, err = capture()
	if err != nil {
		return false, err
	}
	return afterURL != beforeURL || afterCount > beforeCount || afterCount > 0, nil
}

func (a *App) beginAssistLogin(session *siteAssistSession) {
	session.set("login_waiting", "下载资料需要登录。请在同一浏览器窗口完成登录；未成功时会继续等待")
	_ = session.runBrowser(func(ctx context.Context) error {
		return chromedp.Run(ctx, chromedp.Navigate(loginEntryURL(session.domain)))
	})
	session.mu.Lock()
	already := session.loginMonitoring
	session.loginMonitoring = true
	session.mu.Unlock()
	if !already {
		go a.monitorAssistLogin(session)
	}
}

func (a *App) monitorAssistLogin(session *siteAssistSession) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if session.snapshot().State != "login_waiting" {
			session.mu.Lock()
			session.loginMonitoring = false
			session.mu.Unlock()
			return
		}
		cookies, err := a.assistCookies(session)
		if err != nil {
			session.set("closed", "浏览器窗口已关闭，登录等待停止")
			continue
		}
		markers := loginMarkerCookies(session.domain)
		if len(markers) == 0 || !hasCookieMarkers(cookies, markers) {
			continue
		}
		if err := a.saveSiteCookie(session.siteID, session.domain, buildCookieHeader(cookies)); err == nil {
			session.set("login_verifying", "检测到登录成功，正在返回原页面验证下载…")
			go a.resumeAssistAfterLogin(session)
			return
		}
	}
}

const assistLoginPageScript = `(() => {
  const body = document.body ? document.body.innerText : '';
  return !!document.querySelector('input[type="password"]') || /登录后查看|请登录|登录后继续|登录后下载|账号密码登录/.test(body);
})()`

const assistFindSearchScript = `function(advanced) {
  const all = [];
  const collect = root => {
    try {
      for (const el of root.querySelectorAll('input,textarea,[contenteditable="true"]')) all.push(el);
      if (advanced) for (const el of root.querySelectorAll('*')) {
        if (el.shadowRoot) collect(el.shadowRoot);
      }
      if (advanced) for (const frame of root.querySelectorAll('iframe')) {
        try { if (frame.contentDocument) collect(frame.contentDocument); } catch (_) {}
      }
    } catch (_) {}
  };
  collect(document);
  const visible = el => {
    const s = getComputedStyle(el), r = el.getBoundingClientRect();
    return s.display !== 'none' && s.visibility !== 'hidden' && r.width > 40 && r.height > 12 && !el.disabled && !el.readOnly;
  };
  let best = null, bestScore = 0;
  for (const el of all) {
    const type = (el.type || '').toLowerCase();
    if (['password','hidden','checkbox','radio','button','submit','file','number','date'].includes(type) || !visible(el)) continue;
    const text = [el.type,el.name,el.id,el.placeholder,el.getAttribute('aria-label'),el.className].join(' ').toLowerCase();
    let score = advanced ? 1 : 0;
    if (type === 'search') score += 5;
    if (/search|keyword|query|\bq\b|wd|搜索|关键词/.test(text)) score += 4;
    if (el.closest('form') && /search|搜索|s\//.test((el.closest('form').action || '') + (el.closest('form').className || ''))) score += 2;
    if (score > bestScore) { best = el; bestScore = score; }
  }
  const body = document.body ? document.body.innerText : '';
  const login = !!document.querySelector('input[type="password"]') || /登录后查看|请登录|登录后继续|登录后下载/.test(body);
  if (!best) return {found:false,login};
  window.__kaixinAssistSearchInput = best;
  return {found:true,login:false};
}`

const assistExpandSearchScript = `(() => {
  const candidates = [...document.querySelectorAll('button,a,[role="button"]')];
  const target = candidates.find(el => /^(搜索|搜索一下|search|查找)$/i.test((el.innerText || el.getAttribute('aria-label') || el.title || '').trim()));
  if (target) target.click();
  return !!target;
})()`

const assistSubmitSearchScript = `function(keyword) {
  const el = window.__kaixinAssistSearchInput;
  if (!el) return false;
  el.focus();
  // React/Vue 受控输入：直接赋值不会触发 onChange，必须走原生 value setter。
  if (el.isContentEditable) {
    el.textContent = keyword;
  } else {
    const proto = Object.getPrototypeOf(el);
    const setter = proto ? Object.getOwnPropertyDescriptor(proto, 'value') : null;
    if (setter && setter.set) setter.set.call(el, keyword); else el.value = keyword;
  }
  el.dispatchEvent(new Event('input',{bubbles:true}));
  el.dispatchEvent(new Event('change',{bubbles:true}));
  const form = el.closest('form');
  if (form) { if (form.requestSubmit) form.requestSubmit(); else form.submit(); return true; }
  const root = el.parentElement && el.parentElement.parentElement ? el.parentElement.parentElement : document;
  const button = [...root.querySelectorAll('button,input[type="submit"],a,[role="button"]')].find(item => {
    const text = (item.innerText || item.value || item.getAttribute('aria-label') || item.title || '').trim();
    return /搜索|检索|查找|search/i.test(text);
  });
  if (button) { button.click(); return true; }
  for (const type of ['keydown','keypress','keyup']) el.dispatchEvent(new KeyboardEvent(type,{key:'Enter',code:'Enter',keyCode:13,which:13,bubbles:true}));
  return true;
}`

// assistRetrySubmitScript 第一次提交没反应时的兜底：再聚焦输入框，按回车；必要时点击附近的搜索按钮。
const assistRetrySubmitScript = `(() => {
  const el = window.__kaixinAssistSearchInput;
  if (!el) return false;
  el.focus();
  for (const type of ['keydown','keypress','keyup']) el.dispatchEvent(new KeyboardEvent(type,{key:'Enter',code:'Enter',keyCode:13,which:13,bubbles:true}));
  const form = el.closest('form');
  const root = form || (el.parentElement && el.parentElement.parentElement) || document;
  const button = [...root.querySelectorAll('button,input[type="submit"],a,[role="button"]')].find(item => {
    const text = (item.innerText || item.value || item.getAttribute('aria-label') || item.title || '').trim();
    return /^(搜索|检索|查找|search|搜一下)$/i.test(text);
  });
  if (button) button.click();
  return true;
})()`

// assistCaptureStateScript 采集当前 URL 与“结果链接数量”，用来判断提交后页面是否真的变化。
const assistCaptureStateScript = `(() => {
  const roots = [...document.querySelectorAll('main,[class*="result"],[class*="search-list"],[class*="video-list"],[id*="result"]')];
  const anchors = roots.length ? roots.flatMap(root => [...root.querySelectorAll('a[href]')]) : [...document.querySelectorAll('a[href]')];
  const count = anchors.filter(a => {
    const t = (a.innerText || a.textContent || '').trim();
    return t.length >= 4 && !/^(首页|登录|注册|搜索|下一页|上一页|设置|关于)/.test(t);
  }).length;
  return {url: location.href, count};
})()`

// assistDetectBlockerScript 识别验证码 / 安全验证 / 登录墙这类“挡住下载”的阻断页。
const assistDetectBlockerScript = `(() => {
  const body = document.body ? document.body.innerText : '';
  let captcha = /验证码|安全验证|人机验证|请完成验证|图形验证|滑块验证|拖动.*验证|行为验证|captcha|verify you are human|are you a robot/i.test(body);
  if (!captcha) {
    for (const f of document.querySelectorAll('iframe')) {
      const src = f.src || '';
      if (/recaptcha|geetest|captcha|hcaptcha|verification/i.test(src)) { captcha = true; break; }
    }
  }
  const login = !!document.querySelector('input[type="password"]') || /登录后查看|请登录|登录后继续|登录后下载|账号密码登录/.test(body);
  return {captcha, login};
})()`

// assistDismissPopupsScript 关闭常见遮罩弹窗：Cookie 同意、登录提示、活动推广、关闭按钮。
// 只点“关闭/同意”类按钮，且优先点在遮罩/弹窗容器内部，避免误触主页面的搜索按钮。
const assistDismissPopupsScript = `(() => {
  // 只按特征选择器找弹窗容器并限量，禁止遍历 body *：大页面逐元素取样式会卡死主线程。
  const sel = '[role="dialog"],[aria-modal="true"],[class*="modal"],[class*="dialog"],[class*="popup"],[class*="pop-up"],[class*="mask"],[class*="overlay"],[class*="cookie"],[class*="consent"],[class*="notice"]';
  const candidates = [...document.querySelectorAll(sel)].slice(0, 30);
  if (!candidates.length) return {dismissed: 0};
  const isVisible = el => {
    const s = getComputedStyle(el), r = el.getBoundingClientRect();
    return s.display !== 'none' && s.visibility !== 'hidden' && r.width > 0 && r.height > 0;
  };
  const closeText = /^(同意|接受|接受全部|我知道了|知道了|关闭|跳过|以后再说|稍后再说|不再提示|agree|accept|ok|close|dismiss|got it|×|x)$/i;
  let dismissed = 0;
  for (const c of candidates) {
    if (!isVisible(c)) continue;
    const nodes = [...c.querySelectorAll('button,a,[role="button"],[aria-label]')].slice(0, 25);
    for (const b of nodes) {
      const text = (b.innerText || b.getAttribute('aria-label') || b.title || '').trim();
      if (text && closeText.test(text)) { b.click(); dismissed++; break; }
    }
    if (!dismissed) {
      for (const b of nodes) {
        const hint = ((b.className || '') + ' ' + (b.id || '') + ' ' + (b.getAttribute('aria-label') || '')).toString().toLowerCase();
        if (/\b(close|dismiss|cancel|shut|guanbi)\b/.test(hint)) { b.click(); dismissed++; break; }
      }
    }
    if (dismissed) break;
  }
  return {dismissed};
})()`

const assistExtractResultsScript = `(() => {
  const seen = new Set(), out = [];
  const roots = [...document.querySelectorAll('main,[class*="result"],[class*="search-list"],[class*="video-list"],[id*="result"]')];
  const anchors = roots.length ? roots.flatMap(root => [...root.querySelectorAll('a[href]')]) : [...document.querySelectorAll('a[href]')];
  for (const a of anchors) {
    const title = (a.innerText || a.textContent || '').trim().replace(/\s+/g,' ');
    if (title.length < 4 || /^(首页|登录|注册|搜索|下一页|上一页|新闻|网页|视频|图片|贴吧|知道|地图|设置|关于)/.test(title)) continue;
    let url;
    try { url = new URL(a.href, location.href); } catch (_) { continue; }
    if (!/^https?:$/.test(url.protocol) || seen.has(url.href)) continue;
    seen.add(url.href);
    out.push({title:title.slice(0,160),url:url.href,snippet:'来自站内搜索',source:location.hostname});
    if (out.length >= 20) break;
  }
  return out;
})()`
