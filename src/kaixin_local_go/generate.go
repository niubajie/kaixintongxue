package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// 用模型生成「澄清提问」与「三阶段方案」（FR-GOAL-02/03、FR-LRN-02/03）。
// 模型不可用或输出不合格式时返回错误，由前端回退到内置模板，保证功能不瘫。

const questionPromptGoal = `你是目标拆解助手。围绕主题「%s」，提出 5 个用于澄清目标的问题，分别覆盖：成功指标、现状差距、可投入时间、可用资源、中断风险。` +
	`只输出一个 JSON 数组，包含 5 个中文字符串，不要输出任何解释或额外文字。`

const planPromptGoal = `你是目标规划助手。把主题「%s」拆成三个阶段，阶段名依次为：明确现状、稳定执行、复盘冲刺。` +
	`每个阶段给出 3 个具体可执行的步骤（每条一句话，可勾选完成）。` +
	`只输出 JSON，格式为 {"stages":[{"name":"阶段名","steps":["步骤1","步骤2","步骤3"]}]}，不要输出任何解释。`

const questionPromptLearning = `你是学习规划助手。围绕学习主题「%s」，提出 5 个用于澄清学习计划的问题，分别覆盖：当前基础水平、每周频次与时长、可检验的掌握目标、检验方式、遇到卡点时的求助方式。` +
	`只输出一个 JSON 数组，包含 5 个中文字符串，不要输出任何解释或额外文字。`

const planPromptLearning = `你是学习规划助手。把学习主题「%s」拆成三个阶段，阶段名依次为：基础打底、专项训练、综合检验。` +
	`每个阶段给出 3 个具体可执行的步骤（每条一句话，可勾选完成）。` +
	`只输出 JSON，格式为 {"stages":[{"name":"阶段名","steps":["步骤1","步骤2","步骤3"]}]}，不要输出任何解释。`

// extractJSON 从模型输出里抠出 JSON（模型常带 ```json 围栏或前后废话）。
func extractJSON(text string, wantArray bool) string {
	cleaned := strings.ReplaceAll(text, "```json", "")
	cleaned = strings.ReplaceAll(cleaned, "```", "")

	open, closing := byte('{'), byte('}')
	if wantArray {
		open, closing = '[', ']'
	}
	start := strings.IndexByte(cleaned, open)
	last := strings.LastIndexByte(cleaned, closing)
	if start < 0 || last <= start {
		return ""
	}
	return strings.TrimSpace(cleaned[start : last+1])
}

func (a *App) handleGenerateQuestions(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Topic string `json:"topic"`
		Kind  string `json:"kind"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	topic := strings.TrimSpace(body.Topic)
	if topic == "" {
		fail(w, http.StatusBadRequest, "缺少主题")
		return
	}
	prompt := questionPromptGoal
	if body.Kind == "learning" {
		prompt = questionPromptLearning
	}

	// 本机小模型生成 5 个问题可能要一分钟，给足时间
	ctx, cancel := context.WithTimeout(r.Context(), 210*time.Second)
	defer cancel()

	result, err := a.chatOnce(ctx, []ChatMessage{{Role: "user", Content: fmt.Sprintf(prompt, topic)}})
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}

	raw := extractJSON(result.Text, true)
	var questions []string
	if raw == "" || json.Unmarshal([]byte(raw), &questions) != nil {
		fail(w, http.StatusBadGateway, "模型返回的不是有效 JSON，已回退到模板")
		return
	}
	cleaned := make([]string, 0, len(questions))
	for _, item := range questions {
		if text := strings.TrimSpace(item); text != "" {
			cleaned = append(cleaned, text)
		}
	}
	if len(cleaned) < 3 {
		fail(w, http.StatusBadGateway, "模型返回的问题太少，已回退到模板")
		return
	}
	if len(cleaned) > 6 {
		cleaned = cleaned[:6]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"questions": cleaned,
		"model":     result.Model,
		"elapsedMs": result.ElapsedMs,
	})
}

type generatedStage struct {
	Name  string   `json:"name"`
	Steps []string `json:"steps"`
}

func (a *App) handleGeneratePlan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Topic string `json:"topic"`
		Kind  string `json:"kind"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	topic := strings.TrimSpace(body.Topic)
	if topic == "" {
		fail(w, http.StatusBadRequest, "缺少主题")
		return
	}
	prompt := planPromptGoal
	if body.Kind == "learning" {
		prompt = planPromptLearning
	}

	ctx, cancel := context.WithTimeout(r.Context(), 240*time.Second)
	defer cancel()

	result, err := a.chatOnce(ctx, []ChatMessage{{Role: "user", Content: fmt.Sprintf(prompt, topic)}})
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}

	raw := extractJSON(result.Text, false)
	var payload struct {
		Stages []generatedStage `json:"stages"`
	}
	if raw == "" || json.Unmarshal([]byte(raw), &payload) != nil {
		fail(w, http.StatusBadGateway, "模型返回的不是有效 JSON，已回退到模板")
		return
	}

	stages := make([]generatedStage, 0, len(payload.Stages))
	for _, stage := range payload.Stages {
		name := strings.TrimSpace(stage.Name)
		steps := make([]string, 0, len(stage.Steps))
		for _, step := range stage.Steps {
			if text := strings.TrimSpace(step); text != "" {
				steps = append(steps, text)
			}
		}
		if name == "" || len(steps) == 0 {
			continue
		}
		stages = append(stages, generatedStage{Name: name, Steps: steps})
	}
	if len(stages) < 2 {
		fail(w, http.StatusBadGateway, "模型返回的阶段不完整，已回退到模板")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"stages":    stages,
		"model":     result.Model,
		"elapsedMs": result.ElapsedMs,
	})
}
