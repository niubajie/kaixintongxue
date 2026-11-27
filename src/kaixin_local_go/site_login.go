package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

// 反爬站点登录助手：
// 有些站点（知乎 403、CSDN 521、小红书、抖音）只放行已登录用户。
// 用户在验证时被拦下后，点「打开登录窗口」，我们启动一个真实的 Edge 窗口，
// 用户自己完成扫码/账号登录，程序轮询浏览器 Cookie，检测到登录态后自动保存到本地库，
// 之后正文抓取只需带上 Cookie，不用每次开浏览器。
//
// Cookie 存在本地库 search_sites.cookie（单用户本机库，明文可接受），内存里留一份供抓取直接用。

// memoryCookies 域名 → Cookie 字符串，进程内缓存，避免每次抓取都查库。
var memoryCookies = struct {
	sync.RWMutex
	items map[string]string
}{items: map[string]string{}}

func setMemoryCookie(domain, cookie string) {
	domain = siteHostname(domain)
	if domain == "" {
		return
	}
	memoryCookies.Lock()
	defer memoryCookies.Unlock()
	memoryCookies.items[domain] = strings.TrimSpace(cookie)
}

// memoryCookie 按主机名匹配已保存的站点 Cookie（含子域），取最长匹配。
func memoryCookie(host string) string {
	host = siteHostname(host)
	if host == "" {
		return ""
	}
	memoryCookies.RLock()
	defer memoryCookies.RUnlock()
	best, bestLen := "", 0
	for domain, cookie := range memoryCookies.items {
		if cookie == "" || domain == "" {
			continue
		}
		if host == domain || strings.HasSuffix(host, "."+domain) {
			if len(domain) > bestLen {
				best, bestLen = cookie, len(domain)
			}
		}
	}
	return best
}

// loadSiteCookies 启动时把库里已保存的 Cookie 读进内存。
func (a *App) loadSiteCookies(ctx context.Context) {
	if a.store == nil {
		return
	}
	rows, err := a.store.db.QueryContext(ctx, `SELECT domain, cookie FROM search_sites WHERE cookie <> ''`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var domain, cookie string
		if err := rows.Scan(&domain, &cookie); err != nil {
			continue
		}
		setMemoryCookie(domain, cookie)
	}
}

// loginMarkerCookies 各站点登录成功后才会出现的 Cookie 名。
// 没有对应标记的站点，由用户点「我已完成登录」手动确认。
func loginMarkerCookies(domain string) []string {
	lowered := strings.ToLower(domain)
	switch {
	case strings.Contains(lowered, "zhihu.com"):
		return []string{"z_c0"}
	case strings.Contains(lowered, "csdn.net"):
		return []string{"UserToken"}
	case strings.Contains(lowered, "xiaohongshu.com"):
		return []string{"web_session"}
	case strings.Contains(lowered, "douyin.com"):
		return []string{"sessionid", "sid_tt"}
	case strings.Contains(lowered, "weibo.com"):
		return []string{"SUB"}
	case strings.Contains(lowered, "mp.weixin.qq.com"):
		return []string{"slave_sid"}
	}
	return nil
}

// loginEntryURL 登录助手打开的起始页面。
func loginEntryURL(domain string) string {
	lowered := strings.ToLower(domain)
	switch {
	case strings.Contains(lowered, "zhihu.com"):
		return "https://www.zhihu.com/signin"
	case strings.Contains(lowered, "csdn.net"):
		return "https://passport.csdn.net/login"
	case strings.Contains(lowered, "xiaohongshu.com"):
		return "https://www.xiaohongshu.com"
	case strings.Contains(lowered, "douyin.com"):
		return "https://www.douyin.com"
	case strings.Contains(lowered, "weibo.com"):
		return "https://weibo.com/login.php"
	case strings.Contains(lowered, "mp.weixin.qq.com"):
		return "https://mp.weixin.qq.com"
	}
	return "https://" + domain
}

// browserExecPath 找一个可用的 Chromium 内核浏览器，优先本机 Edge。
func browserExecPath() string {
	if custom := strings.TrimSpace(os.Getenv("KAIXIN_BROWSER_PATH")); custom != "" {
		return custom
	}
	candidates := []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// siteLoginSession 一次登录引导的会话状态。
type siteLoginSession struct {
	mu      sync.Mutex
	state   string // starting | waiting | success | failed | closed
	message string
	ctx     context.Context // 浏览器上下文，供「我已完成登录」直接取 Cookie
	cancel  context.CancelFunc
}

func (s *siteLoginSession) set(state, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state, s.message = state, message
}

func (s *siteLoginSession) snapshot() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, s.message
}

func (s *siteLoginSession) finish(state, message string) {
	s.set(state, message)
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *siteLoginSession) browserContext() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ctx
}

// loginSessions 站点 ID → 会话。
var loginSessions = struct {
	sync.Mutex
	items map[string]*siteLoginSession
}{items: map[string]*siteLoginSession{}}

func lookupLoginSession(id string) *siteLoginSession {
	loginSessions.Lock()
	defer loginSessions.Unlock()
	return loginSessions.items[id]
}

func storeLoginSession(id string, session *siteLoginSession) {
	loginSessions.Lock()
	defer loginSessions.Unlock()
	loginSessions.items[id] = session
}

// handleSiteLogin 启动登录助手：开一个有头浏览器窗口让用户登录。
func (a *App) handleSiteLogin(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	var request struct {
		ID string `json:"id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	id := strings.TrimSpace(request.ID)
	if id == "" {
		fail(w, http.StatusBadRequest, "缺少网站 ID")
		return
	}
	var domain, name string
	if err := a.store.db.QueryRowContext(r.Context(),
		`SELECT name, domain FROM search_sites WHERE id = ?`, id).Scan(&name, &domain); err != nil {
		fail(w, http.StatusNotFound, "网站不存在")
		return
	}
	domain = normalizeDomain(domain)
	if domain == "" {
		fail(w, http.StatusBadRequest, "该网站还没有填写域名")
		return
	}
	if existing := lookupLoginSession(id); existing != nil {
		if state, _ := existing.snapshot(); state == "starting" || state == "waiting" {
			// 浏览器窗口还活着就复用；用户已经把窗口关掉则重新开一个。
			if ctx := existing.browserContext(); ctx != nil && ctx.Err() == nil {
				writeJSON(w, http.StatusOK, map[string]any{"state": state})
				return
			}
			existing.finish("closed", "登录窗口已关闭")
		}
	}
	browser := browserExecPath()
	if browser == "" {
		fail(w, http.StatusFailedDependency, "没有找到 Edge 或 Chrome，请先安装 Edge，或设置环境变量 KAIXIN_BROWSER_PATH 指向浏览器程序")
		return
	}

	session := &siteLoginSession{state: "starting", message: "正在打开登录窗口…"}
	storeLoginSession(id, session)
	go a.runSiteLogin(session, id, name, domain, browser)
	writeJSON(w, http.StatusOK, map[string]any{"state": "starting"})
}

// runSiteLogin 打开浏览器 → 轮询 Cookie → 检测到登录态后保存并关闭窗口。
func (a *App) runSiteLogin(session *siteLoginSession, id, name, domain, browser string) {
	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer timeoutCancel()

	// 独立用户数据目录：不干扰用户平时的浏览器，也便于复用登录态。
	profileName := strings.NewReplacer(":", "_", "/", "_", "\\", "_").Replace(domain)
	profile := filepath.Join(a.root, "browser-profile", profileName)
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browser),
		chromedp.UserDataDir(profile),
		chromedp.Flag("headless", false),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(timeoutCtx, opts...)
	defer allocCancel()
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	session.mu.Lock()
	session.ctx = browserCtx
	session.cancel = browserCancel
	session.mu.Unlock()

	if err := chromedp.Run(browserCtx, chromedp.Navigate(loginEntryURL(domain))); err != nil {
		session.finish("failed", "打开登录窗口失败："+err.Error())
		return
	}
	session.set("waiting", "请在弹出的浏览器窗口里完成登录（扫码或账号密码），检测到登录状态后会自动保存并关闭窗口")

	markers := loginMarkerCookies(domain)
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-timeoutCtx.Done():
			if state, _ := session.snapshot(); state == "waiting" {
				session.finish("closed", "登录等待超时（10 分钟），窗口已关闭，可重新点击「打开登录窗口」")
			}
			return
		case <-ticker.C:
			if state, _ := session.snapshot(); state != "waiting" {
				return
			}
			cookies, err := collectSiteCookies(browserCtx, domain)
			if err != nil {
				// 用户手动关掉了浏览器窗口。
				session.finish("closed", "登录窗口已关闭；如果已经登录，请重新点「打开登录窗口」再点「我已完成登录」")
				return
			}
			if len(markers) == 0 || len(cookies) == 0 {
				continue
			}
			if !hasCookieMarkers(cookies, markers) {
				continue
			}
			cookieString := buildCookieHeader(cookies)
			if cookieString == "" {
				continue
			}
			if err := a.saveSiteCookie(id, domain, cookieString); err != nil {
				session.finish("failed", "登录成功但保存失败："+err.Error())
				return
			}
			session.finish("success", fmt.Sprintf("已检测到「%s」登录状态，登录信息已保存", name))
			return
		}
	}
}

// collectSiteCookies 读取浏览器里属于该域名的 Cookie。
func collectSiteCookies(ctx context.Context, domain string) ([]*network.Cookie, error) {
	domain = siteHostname(domain)
	var all []*network.Cookie
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(runCtx context.Context) error {
		cookies, err := storage.GetCookies().Do(runCtx)
		if err != nil {
			return err
		}
		all = cookies
		return nil
	})); err != nil {
		return nil, err
	}
	matched := make([]*network.Cookie, 0, len(all))
	for _, cookie := range all {
		if cookie == nil {
			continue
		}
		cookieDomain := strings.TrimPrefix(strings.ToLower(cookie.Domain), ".")
		if cookieDomain == "" {
			continue
		}
		if cookieDomain == domain || strings.HasSuffix(domain, "."+cookieDomain) || strings.HasSuffix(cookieDomain, "."+domain) {
			matched = append(matched, cookie)
		}
	}
	return matched, nil
}

func hasCookieMarkers(cookies []*network.Cookie, markers []string) bool {
	for _, cookie := range cookies {
		if cookie == nil || strings.TrimSpace(cookie.Value) == "" {
			continue
		}
		for _, marker := range markers {
			if strings.EqualFold(cookie.Name, marker) {
				return true
			}
		}
	}
	return false
}

func buildCookieHeader(cookies []*network.Cookie) string {
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie == nil || cookie.Name == "" {
			continue
		}
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(parts, "; ")
}

// saveSiteCookie 写库 + 写内存缓存。
func (a *App) saveSiteCookie(id, domain, cookie string) error {
	if a.store == nil {
		return fmt.Errorf("本地库未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := a.store.db.ExecContext(ctx,
		`UPDATE search_sites SET cookie = ? WHERE id = ?`, cookie, id); err != nil {
		return err
	}
	setMemoryCookie(domain, cookie)
	return nil
}

// handleSiteLoginStatus 前端轮询登录进度。
func (a *App) handleSiteLoginStatus(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	session := lookupLoginSession(id)
	if session == nil {
		writeJSON(w, http.StatusOK, map[string]any{"state": "idle", "message": "尚未开始登录"})
		return
	}
	state, message := session.snapshot()
	writeJSON(w, http.StatusOK, map[string]any{"state": state, "message": message})
}

// handleSiteLoginFinish 兜底：用户手动确认已登录，直接取当前浏览器 Cookie 保存。
func (a *App) handleSiteLoginFinish(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID string `json:"id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	id := strings.TrimSpace(request.ID)
	session := lookupLoginSession(id)
	if session == nil {
		fail(w, http.StatusConflict, "登录窗口没有打开，请先点「打开登录窗口」")
		return
	}
	var domain, name string
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	if err := a.store.db.QueryRowContext(r.Context(),
		`SELECT name, domain FROM search_sites WHERE id = ?`, id).Scan(&name, &domain); err != nil {
		fail(w, http.StatusNotFound, "网站不存在")
		return
	}
	domain = normalizeDomain(domain)
	browserCtx := session.browserContext()
	if browserCtx == nil {
		fail(w, http.StatusConflict, "登录窗口已关闭，请重新点「打开登录窗口」")
		return
	}
	cookies, err := collectSiteCookies(browserCtx, domain)
	if err != nil || len(cookies) == 0 {
		fail(w, http.StatusPreconditionFailed, "没有读到该站的登录信息，请确认已在弹出的窗口里完成登录")
		return
	}
	// 已知登录标记的站点（知乎 z_c0、CSDN UserToken 等）：没登录时浏览器里也有匿名 Cookie，
	// 必须确认标记存在，否则会把匿名 Cookie 当成登录态存下来。
	if markers := loginMarkerCookies(domain); len(markers) > 0 && !hasCookieMarkers(cookies, markers) {
		fail(w, http.StatusPreconditionFailed, "还没有检测到该站的登录状态，请先在弹出的窗口里完成登录，再点「我已完成登录」")
		return
	}
	cookieString := buildCookieHeader(cookies)
	if cookieString == "" {
		fail(w, http.StatusPreconditionFailed, "浏览器里没有该站的 Cookie，请先完成登录")
		return
	}
	if err := a.saveSiteCookie(id, domain, cookieString); err != nil {
		fail(w, http.StatusInternalServerError, "保存登录信息失败："+err.Error())
		return
	}
	session.finish("success", fmt.Sprintf("已保存「%s」的登录信息", name))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "已保存登录信息"})
}
