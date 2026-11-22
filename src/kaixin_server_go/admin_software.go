package main

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type softwareInput struct {
	Name       string `json:"name"`
	WingetID   string `json:"wingetId"`
	Purpose    string `json:"purpose"`
	Summary    string `json:"summary"`
	AISoftware string `json:"aiSoftware"`
	Notes      string `json:"notes"`
	Required   bool   `json:"required"`
}

func (input softwareInput) normalized() Software {
	return Software{
		Name:       strings.TrimSpace(input.Name),
		WingetID:   strings.TrimSpace(input.WingetID),
		Purpose:    strings.TrimSpace(input.Purpose),
		Summary:    strings.TrimSpace(input.Summary),
		AISoftware: strings.TrimSpace(input.AISoftware),
		Notes:      strings.TrimSpace(input.Notes),
		Required:   input.Required,
	}
}

// buildSoftwareDTO 汇总当前版本与上一版本，供客户端与管理后台共用。
func (a *App) buildSoftwareDTO(r *http.Request, software Software) SoftwareDTO {
	versions, err := a.listVersions(r.Context(), software.ID)
	if err != nil {
		versions = nil
	}
	dto := SoftwareDTO{
		ID:         software.ID,
		Name:       software.Name,
		Purpose:    software.Purpose,
		Summary:    software.Summary,
		AISoftware: software.AISoftware,
		Required:   software.Required,
		WingetID:   software.WingetID,
		Notes:      software.Notes,
	}
	published := make([]Version, 0, len(versions))
	for _, version := range versions {
		if version.Status != "draft" {
			published = append(published, version)
		}
	}
	// 必须按 is_current 取「当前版本」：回滚后最新发布的版本已不是对外下载版本，
	// 若按列表第一条取，管理后台会显示错误的当前版本。
	current, previous := pickCurrentVersion(published)
	if current != nil {
		currentDTO := toVersionDTO(*current)
		dto.Current = &currentDTO
	}
	if previous != nil {
		previousDTO := toVersionDTO(*previous)
		dto.Previous = &previousDTO
	}
	return dto
}

// pickCurrentVersion 从已发布版本列表中挑出对外下载版本（is_current）与它的上一个版本（回滚目标）。
// 列表需按发布时间倒序（最新在前）；没有标记 is_current 时两个返回值都为 nil。
func pickCurrentVersion(published []Version) (current *Version, previous *Version) {
	for index := range published {
		if !published[index].IsCurrent {
			continue
		}
		current = &published[index]
		if index+1 < len(published) {
			previous = &published[index+1]
		}
		return current, previous
	}
	return nil, nil
}

func (a *App) handleSoftwareList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, err := a.listSoftware(ctx)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取技能失败。")
		return
	}
	result := make([]SoftwareDTO, 0, len(items))
	for _, item := range items {
		dto := a.buildSoftwareDTO(r, item)
		// CLD-06：对外目录必须带 SHA-256。没有校验值的条目不下发，
		// 避免客户端拿到无法验证的安装包。
		if dto.Current != nil && dto.Current.SHA256 == "" {
			continue
		}
		result = append(result, dto)
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *App) handleSkillPackages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, err := a.listSoftware(ctx)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取技能失败。")
		return
	}
	type packageDTO struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		WingetID   string `json:"wingetId"`
		Purpose    string `json:"purpose"`
		Summary    string `json:"summary"`
		AISoftware string `json:"aiSoftware"`
		Notes      string `json:"notes"`
		Required   bool   `json:"required"`
	}
	result := make([]packageDTO, 0, len(items))
	for _, item := range items {
		result = append(result, packageDTO{
			ID: item.ID, Name: item.Name, WingetID: item.WingetID, Purpose: item.Purpose,
			Summary: item.Summary, AISoftware: item.AISoftware, Notes: item.Notes, Required: item.Required,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *App) handleSoftwareCreate(w http.ResponseWriter, r *http.Request) {
	admin, ok := a.requireCatalogEditor(w, r, "admin.software.write")
	if !ok {
		return
	}
	var body softwareInput
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确。")
		return
	}
	input := body.normalized()
	if input.Name == "" {
		fail(w, http.StatusBadRequest, "请填写技能名称。")
		return
	}

	ctx := r.Context()
	existing, err := a.findSoftwareByName(ctx, input.Name)
	if err != nil {
		fail(w, http.StatusInternalServerError, "保存失败。")
		return
	}
	if existing != nil {
		fail(w, http.StatusConflict, "同名技能已存在。")
		return
	}
	if _, err := a.createSoftware(ctx, input); err != nil {
		fail(w, http.StatusInternalServerError, "保存失败。")
		return
	}
	a.audit(ctx, admin.Account, "skill_create", input.Name)
	writeMessage(w, http.StatusOK, "已新增技能「"+input.Name+"」。")
}

func (a *App) handleSoftwareUpdate(w http.ResponseWriter, r *http.Request) {
	admin, ok := a.requireCatalogEditor(w, r, "admin.software.write")
	if !ok {
		return
	}
	id, valid := parseInt64(r.PathValue("id"))
	if !valid {
		fail(w, http.StatusBadRequest, "技能不存在。")
		return
	}
	var body softwareInput
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确。")
		return
	}
	input := body.normalized()
	if input.Name == "" {
		fail(w, http.StatusBadRequest, "请填写技能名称。")
		return
	}

	ctx := r.Context()
	existing, err := a.findSoftware(ctx, id)
	if err != nil || existing == nil {
		fail(w, http.StatusNotFound, "技能不存在。")
		return
	}
	duplicated, err := a.findSoftwareByName(ctx, input.Name)
	if err != nil {
		fail(w, http.StatusInternalServerError, "保存失败。")
		return
	}
	if duplicated != nil && duplicated.ID != id {
		fail(w, http.StatusConflict, "同名技能已存在。")
		return
	}
	if err := a.updateSoftware(ctx, id, input); err != nil {
		fail(w, http.StatusInternalServerError, "保存失败。")
		return
	}
	a.audit(ctx, admin.Account, "skill_update", input.Name)
	writeMessage(w, http.StatusOK, "已更新技能「"+input.Name+"」。")
}

func (a *App) handleSoftwareDelete(w http.ResponseWriter, r *http.Request) {
	admin, ok := a.requireCatalogEditor(w, r, "admin.software.write")
	if !ok {
		return
	}
	id, valid := parseInt64(r.PathValue("id"))
	if !valid {
		fail(w, http.StatusBadRequest, "技能不存在。")
		return
	}
	ctx := r.Context()
	existing, err := a.findSoftware(ctx, id)
	if err != nil || existing == nil {
		fail(w, http.StatusNotFound, "技能不存在。")
		return
	}
	if err := a.deleteSoftware(ctx, id); err != nil {
		fail(w, http.StatusInternalServerError, "删除失败。")
		return
	}
	a.audit(ctx, admin.Account, "skill_delete", existing.Name)
	writeMessage(w, http.StatusOK, "已删除技能「"+existing.Name+"」。")
}

var versionPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)

func bumpVersion(current string) string {
	match := versionPattern.FindStringSubmatch(strings.TrimSpace(current))
	if match == nil {
		return "0.1.0"
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	return fmt.Sprintf("%d.%d.%d", major, minor, patch+1)
}

func (a *App) handleNextVersion(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireCatalogEditor(w, r, "admin.software.list"); !ok {
		return
	}
	id, valid := parseInt64(r.PathValue("id"))
	if !valid {
		fail(w, http.StatusBadRequest, "技能不存在。")
		return
	}
	ctx := r.Context()
	versions, err := a.listVersions(ctx, id)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取版本失败。")
		return
	}
	next := "0.1.0"
	if len(versions) > 0 {
		next = bumpVersion(versions[0].Version)
	}
	writeJSON(w, http.StatusOK, map[string]string{"version": next})
}

// handleVersionDelete 删除误发布的版本。
// 当前版本被删后不再有 current，客户端会看到「尚未发布」，不会拿到坏包。
func (a *App) handleVersionDelete(w http.ResponseWriter, r *http.Request) {
	admin, ok := a.requireLogin(w, r, "admin.software.version.delete")
	if !ok {
		return
	}
	if !canEditCatalog(admin) {
		a.denyWithAudit(w, r, admin, "admin.software.version.delete", "", reasonNotAdmin)
		return
	}
	softwareID, valid := parseInt64(r.PathValue("id"))
	if !valid {
		fail(w, http.StatusBadRequest, "技能不存在。")
		return
	}
	version := strings.TrimSpace(r.PathValue("version"))
	if version == "" {
		fail(w, http.StatusBadRequest, "缺少版本号。")
		return
	}

	ctx := r.Context()
	var versionID int64
	var filePath string
	err := a.db.QueryRow(ctx,
		`SELECT id, file_path FROM software_versions WHERE software_id = $1 AND version = $2`,
		softwareID, version).Scan(&versionID, &filePath)
	if err != nil {
		fail(w, http.StatusNotFound, "找不到该版本。")
		return
	}

	if _, err := a.db.Exec(ctx, `DELETE FROM software_versions WHERE id = $1`, versionID); err != nil {
		fail(w, http.StatusInternalServerError, "删除版本失败。")
		return
	}
	if filePath != "" {
		_ = os.Remove(filePath)
	}
	a.audit(ctx, admin.Account, "version_delete", fmt.Sprintf("software=%d version=%s", softwareID, version))
	writeMessage(w, http.StatusOK, "已删除版本 "+version+"。")
}
