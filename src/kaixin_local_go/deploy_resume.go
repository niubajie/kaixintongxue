package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// FR-SKL-18 失败重试与续装。
// 步骤快照每完成一步就落库（persistSteps），这里负责读回来继续做。
// 设计上借鉴 Rakazo 的「运行可恢复」：状态落在库里，恢复不依赖进程内存。

// reusableArchive 判断已下载的安装包能否直接复用。
// 有校验值时必须匹配，没有校验值只能看存在性——宁可多下一次，也不装错包。
func (a *App) reusableArchive(path, wantSHA string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return false
	}
	if wantSHA == "" {
		return true
	}
	sum, err := fileSHA256(path)
	if err != nil {
		return false
	}
	return strings.EqualFold(sum, wantSHA)
}

// loadRunSteps 从本地库读回某次部署的步骤快照。
func (a *App) loadRunSteps(ctx context.Context, runID string) (DeployRun, []*DeployStepStatus, error) {
	if a.store == nil {
		return DeployRun{}, nil, fmt.Errorf("本地库未初始化")
	}
	run, err := a.store.LoadDeployRun(ctx, runID)
	if err != nil {
		return DeployRun{}, nil, fmt.Errorf("找不到部署记录 %s", runID)
	}
	var steps []*DeployStepStatus
	if run.StepsJSON != "" {
		if err := json.Unmarshal([]byte(run.StepsJSON), &steps); err != nil {
			return run, nil, fmt.Errorf("步骤快照损坏：%v", err)
		}
	}
	return run, steps, nil
}

// claimDeploySlot 占住「同时只允许一个部署任务」的名额。
func (a *App) claimDeploySlot(job *DeployJob) bool {
	a.deploy.mu.Lock()
	defer a.deploy.mu.Unlock()
	if a.deploy.running {
		return false
	}
	a.deploy.job = job
	a.deploy.running = true
	ctx, cancel := context.WithCancel(context.Background())
	a.deploy.ctx = ctx
	a.deploy.cancel = cancel
	return true
}

// attachDeployLog 续跑时把日志接到原文件末尾，保持「一次任务一个日志」。
func (a *App) attachDeployLog(job *DeployJob) {
	if job.LogPath == "" {
		return
	}
	file, err := os.OpenFile(job.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	a.deploy.logFile = file
}

func (a *App) finishJob(job *DeployJob) {
	state := "done"
	for _, step := range job.Steps {
		if step.Status == "failed" {
			state = "failed"
			break
		}
		if step.Status == "skipped" && state == "done" {
			// 跳过不算失败
			continue
		}
	}

	a.deploy.mu.Lock()
	a.deploy.running = false
	job.State = state
	job.FinishedAt = time.Now().Format(time.RFC3339)
	if a.deploy.cancel != nil {
		a.deploy.cancel()
		a.deploy.cancel = nil
	}
	if a.deploy.logFile != nil {
		_ = a.deploy.logFile.Close()
		a.deploy.logFile = nil
	}
	a.deploy.mu.Unlock()

	if a.store != nil {
		payload, _ := json.Marshal(job.Steps)
		total := 0
		for _, step := range job.Steps {
			total += step.SizeMb
		}
		reason := ""
		if state == "failed" {
			for _, step := range job.Steps {
				if step.Status == "failed" {
					reason = step.Name + "：" + step.Message
					break
				}
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = a.store.RecordDeployRun(ctx, DeployRun{
			ID: job.ID, State: state, StartedAt: job.StartedAt, FinishedAt: job.FinishedAt,
			TotalSizeMb: total, LogPath: job.LogPath, StepsJSON: string(payload), Error: reason,
		})
		cancel()
	}
	a.deployLog("任务结束：%s", state)
}

// handleDeployResume 续装：跳过已完成步骤，只跑剩下的。
func (a *App) handleDeployResume(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RunID string `json:"runId"`
		Force bool   `json:"force"`
	}
	if err := decodeJSON(r, &request); err != nil || strings.TrimSpace(request.RunID) == "" {
		fail(w, http.StatusBadRequest, "缺少 runId")
		return
	}
	run, steps, err := a.loadRunSteps(r.Context(), request.RunID)
	if err != nil {
		fail(w, http.StatusNotFound, err.Error())
		return
	}
	if len(steps) == 0 {
		fail(w, http.StatusBadRequest, "该记录没有步骤快照，无法续装")
		return
	}
	pending := 0
	for _, step := range steps {
		if step.Status != "done" {
			pending++
		}
	}
	if pending == 0 {
		fail(w, http.StatusBadRequest, "该任务的所有步骤都已完成，无需续装")
		return
	}

	job := &DeployJob{
		ID: run.ID, State: "running", StartedAt: run.StartedAt,
		LogPath: run.LogPath, Steps: steps,
	}
	if !a.claimDeploySlot(job) {
		fail(w, http.StatusConflict, "已有部署任务正在进行，请等待完成。")
		return
	}
	a.attachDeployLog(job)
	a.deployLog("续装任务 %s：剩余 %d 步", job.ID, pending)

	go func() {
		defer a.finishJob(job)
		a.runDeployResume(job, request.Force)
	}()

	writeJSON(w, http.StatusOK, job)
}

// handleDeployRetryStep 单步重试：只重跑指定的一步，不动其他步骤的结果。
func (a *App) handleDeployRetryStep(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RunID string `json:"runId"`
		Order int    `json:"order"`
		Force bool   `json:"force"`
	}
	if err := decodeJSON(r, &request); err != nil || strings.TrimSpace(request.RunID) == "" {
		fail(w, http.StatusBadRequest, "缺少 runId")
		return
	}
	run, steps, err := a.loadRunSteps(r.Context(), request.RunID)
	if err != nil {
		fail(w, http.StatusNotFound, err.Error())
		return
	}

	var target *DeployStepStatus
	for _, step := range steps {
		if step.Order == request.Order {
			target = step
			break
		}
	}
	if target == nil {
		fail(w, http.StatusNotFound, fmt.Sprintf("找不到第 %d 步", request.Order))
		return
	}

	job := &DeployJob{
		ID: run.ID, State: "running", StartedAt: run.StartedAt,
		LogPath: run.LogPath, Steps: steps,
	}
	if !a.claimDeploySlot(job) {
		fail(w, http.StatusConflict, "已有部署任务正在进行，请等待完成。")
		return
	}
	a.attachDeployLog(job)

	entry := findSoftware(a.catalog, target.SoftwareID)
	go func() {
		defer a.finishJob(job)
		a.deployLog("单步重试：%s", target.Name)
		if a.store != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = a.store.StartDeployRun(ctx, DeployRun{ID: job.ID, StartedAt: job.StartedAt, LogPath: job.LogPath})
			cancel()
		}
		stop := a.startDeployHeartbeat(job.ID)
		defer stop()

		if entry == nil {
			a.setStep(target, "failed", "技能目录里找不到该软件，无法重试")
		} else {
			a.setStep(target, "pending", "单步重试中…")
			if err := a.executeStep(target, entry, request.Force); err != nil {
				a.setStep(target, "failed", err.Error())
			}
		}
		a.persistSteps(job)
	}()

	writeJSON(w, http.StatusOK, job)
}
