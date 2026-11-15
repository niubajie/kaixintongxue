package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const maxPackageSize = 512 << 20

func (a *App) handlePublish(w http.ResponseWriter, r *http.Request) {
	admin, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	softwareID, valid := parseInt64(r.PathValue("id"))
	if !valid {
		fail(w, http.StatusBadRequest, "技能不存在。")
		return
	}

	ctx := r.Context()
	software, err := a.findSoftware(ctx, softwareID)
	if err != nil || software == nil {
		fail(w, http.StatusNotFound, "技能不存在。")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPackageSize+8<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		fail(w, http.StatusBadRequest, "上传内容过大或格式不正确。")
		return
	}
	version := strings.TrimSpace(r.FormValue("version"))
	notes := strings.TrimSpace(r.FormValue("notes"))
	draft := strings.EqualFold(r.FormValue("draft"), "true")
	if !versionPattern.MatchString(version) {
		fail(w, http.StatusBadRequest, "版本号格式应为 x.y.z。")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		fail(w, http.StatusBadRequest, "请选择要上传的安装包。")
		return
	}
	defer file.Close()
	if header.Size > maxPackageSize {
		fail(w, http.StatusBadRequest, "安装包不能超过 512 MB。")
		return
	}

	targetDir := filepath.Join(a.cfg.DataRoot, "software", fmt.Sprintf("%d", softwareID), version)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		fail(w, http.StatusInternalServerError, "保存文件失败。")
		return
	}
	fileName := filepath.Base(header.Filename)
	targetPath := filepath.Join(targetDir, fileName)

	destination, err := os.Create(targetPath)
	if err != nil {
		fail(w, http.StatusInternalServerError, "保存文件失败。")
		return
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(destination, hasher), io.LimitReader(file, maxPackageSize))
	closeErr := destination.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(targetPath)
		fail(w, http.StatusInternalServerError, "保存文件失败。")
		return
	}
	digest := hex.EncodeToString(hasher.Sum(nil))

	scanStatus, scanDetail := a.scanPackage(targetPath)
	if scanStatus == "infected" && !draft {
		_ = os.Remove(targetPath)
		a.audit(ctx, admin.Account, "version_blocked", software.Name+" "+version)
		fail(w, http.StatusBadRequest, "安全检查发现风险，已阻止发布："+scanDetail)
		return
	}

	status := "published"
	if draft {
		status = "draft"
	}
	record := Version{
		SoftwareID:   softwareID,
		Version:      version,
		FileName:     fileName,
		FilePath:     targetPath,
		FileSize:     written,
		SHA256:       digest,
		ScanStatus:   scanStatus,
		ScanDetail:   scanDetail,
		ReleaseNotes: notes,
		Status:       status,
	}
	versionID, err := a.upsertVersion(ctx, record)
	if err != nil {
		fail(w, http.StatusInternalServerError, "写入版本记录失败。")
		return
	}
	if !draft {
		if err := a.setCurrentVersion(ctx, softwareID, versionID); err != nil {
			fail(w, http.StatusInternalServerError, "更新当前版本失败。")
			return
		}
	}

	action := "version_publish"
	message := "已发布 " + software.Name + " v" + version + "。"
	if draft {
		action = "version_draft"
		message = "已保存草稿 v" + version + "，确认后才会替换当前版本。"
	}
	a.audit(ctx, admin.Account, action, software.Name+" v"+version)
	writeJSON(w, http.StatusOK, map[string]any{
		"message":    message,
		"version":    version,
		"scanStatus": scanStatus,
		"scanDetail": scanDetail,
	})
}

func (a *App) handleConfirmVersion(w http.ResponseWriter, r *http.Request) {
	admin, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	softwareID, validID := parseInt64(r.PathValue("id"))
	versionText := strings.TrimSpace(r.PathValue("version"))
	if !validID || versionText == "" {
		fail(w, http.StatusBadRequest, "参数不正确。")
		return
	}

	ctx := r.Context()
	versions, err := a.listVersions(ctx, softwareID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取版本失败。")
		return
	}
	var target *Version
	for index := range versions {
		if versions[index].Version == versionText {
			target = &versions[index]
			break
		}
	}
	if target == nil {
		fail(w, http.StatusNotFound, "版本不存在。")
		return
	}
	if target.Status != "draft" {
		fail(w, http.StatusBadRequest, "只有草稿可以确认发布。")
		return
	}
	if target.ScanStatus == "infected" {
		fail(w, http.StatusBadRequest, "该版本安全检查未通过，不能发布。")
		return
	}
	if err := a.setVersionStatus(ctx, target.ID, "published"); err != nil {
		fail(w, http.StatusInternalServerError, "确认失败。")
		return
	}
	if err := a.setCurrentVersion(ctx, softwareID, target.ID); err != nil {
		fail(w, http.StatusInternalServerError, "确认失败。")
		return
	}
	a.audit(ctx, admin.Account, "version_confirm", fmt.Sprintf("software=%d v%s", softwareID, versionText))
	writeMessage(w, http.StatusOK, "已确认发布 v"+versionText+"。")
}

func (a *App) handleRollback(w http.ResponseWriter, r *http.Request) {
	admin, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	softwareID, valid := parseInt64(r.PathValue("id"))
	if !valid {
		fail(w, http.StatusBadRequest, "技能不存在。")
		return
	}

	ctx := r.Context()
	versions, err := a.listVersions(ctx, softwareID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取版本失败。")
		return
	}
	published := make([]Version, 0, len(versions))
	for _, version := range versions {
		if version.Status != "draft" {
			published = append(published, version)
		}
	}
	if len(published) < 2 {
		fail(w, http.StatusBadRequest, "没有可回滚的历史版本。")
		return
	}
	target := published[1]
	if err := a.setCurrentVersion(ctx, softwareID, target.ID); err != nil {
		fail(w, http.StatusInternalServerError, "回滚失败。")
		return
	}
	a.audit(ctx, admin.Account, "version_rollback", fmt.Sprintf("software=%d -> v%s", softwareID, target.Version))
	writeMessage(w, http.StatusOK, "已回滚到 v"+target.Version+"。")
}

func (a *App) handleRescan(w http.ResponseWriter, r *http.Request) {
	admin, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	softwareID, valid := parseInt64(r.PathValue("id"))
	if !valid {
		fail(w, http.StatusBadRequest, "技能不存在。")
		return
	}

	ctx := r.Context()
	current, err := a.currentVersion(ctx, softwareID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取版本失败。")
		return
	}
	if current == nil {
		fail(w, http.StatusBadRequest, "该技能还没有已发布版本。")
		return
	}
	status, detail := a.scanPackage(current.FilePath)
	if err := a.updateScan(ctx, current.ID, status, detail); err != nil {
		fail(w, http.StatusInternalServerError, "保存审查结果失败。")
		return
	}
	a.audit(ctx, admin.Account, "version_scan", fmt.Sprintf("software=%d v%s -> %s", softwareID, current.Version, status))
	message := "v" + current.Version + " 审查通过。"
	if status == "infected" {
		message = "v" + current.Version + " 发现风险：" + detail
	} else if status == "unavailable" {
		message = "本机未找到可用杀毒引擎，已记录为未完成审查。"
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": message, "scanStatus": status, "scanDetail": detail})
}

func (a *App) handleHistory(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	softwareID, valid := parseInt64(r.PathValue("id"))
	if !valid {
		fail(w, http.StatusBadRequest, "技能不存在。")
		return
	}
	versions, err := a.listVersions(r.Context(), softwareID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取版本失败。")
		return
	}
	items := make([]VersionDTO, 0, len(versions))
	for _, version := range versions {
		items = append(items, toVersionDTO(version))
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *App) handleNotesList(w http.ResponseWriter, r *http.Request) {
	notes, err := a.listNotes(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取说明失败。")
		return
	}
	writeJSON(w, http.StatusOK, notes)
}

func (a *App) handleNoteCreate(w http.ResponseWriter, r *http.Request) {
	admin, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Version string `json:"version"`
		Title   string `json:"title"`
		Detail  string `json:"detail"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确。")
		return
	}
	version := strings.TrimSpace(body.Version)
	title := strings.TrimSpace(body.Title)
	if version == "" || title == "" {
		fail(w, http.StatusBadRequest, "请填写版本号与标题。")
		return
	}
	ctx := r.Context()
	if err := a.addNote(ctx, version, title, strings.TrimSpace(body.Detail)); err != nil {
		fail(w, http.StatusInternalServerError, "发布说明失败。")
		return
	}
	a.audit(ctx, admin.Account, "note_publish", version+" "+title)
	writeMessage(w, http.StatusOK, "已发布 v"+version+" 的升级说明。")
}
