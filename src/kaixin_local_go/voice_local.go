package main

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	sherpa "github.com/k2-fsa/sherpa-onnx-go-windows"
)

// 本地实时语音：sherpa-onnx 负责中文流式识别与中文语音合成，对话复用本机模型网关。
//
// 全链路不依赖任何云端服务与密钥：
//   麦克风 16kHz PCM → VAD（打断检测）→ 流式 ASR → 模型网关（Ollama）→ 逐句 TTS → 16kHz PCM 回放
//
// 模型目录约定：KAIXIN_MODELS > <运行根目录>/models > <exe 所在目录>/models。

const (
	voiceSampleRate      = 16000
	voiceChunkWindows    = 512                 // VAD 每次至少吃 512 个采样点
	voiceHistoryLimit    = 12                  // 保留最近 6 轮对话，避免 prompt 无限增长
	voiceBargeWindows    = 3                   // 判定用户插话所需的连续语音窗口数（约 96ms）
	voicePreRollSamples  = voiceSampleRate / 2 // 插话时回补的音频长度：0.5 秒
	voiceMaxFrameSize    = 4 << 20
	voiceSpeechSpeed     = 1.1 // 语速：本机 VITS 默认偏慢，略微加速更像对话
	voiceMinSegmentRunes = 6   // 逗号切句的最小字数
)

const voiceSystemPrompt = "你是开心同学 AI 助手，用自然口语化的中文回答，简洁直接，一般不超过三句话；用户打断时立即停止并倾听。"

// VoiceLocalStatus 是前端判断能否开始对话的唯一依据。
type VoiceLocalStatus struct {
	Ready      bool     `json:"ready"`
	Loading    bool     `json:"loading"`
	ModelsRoot string   `json:"modelsRoot"`
	ASRModel   string   `json:"asrModel"`
	TTSModel   string   `json:"ttsModel"`
	SampleRate int      `json:"sampleRate"`
	Missing    []string `json:"missing,omitempty"`
	Message    string   `json:"message"`
}

type voiceEngine struct {
	mu     sync.Mutex
	state  string // "" / "loading" / "ready" / "error"
	errMsg string
	wait   chan struct{}

	rec *sherpa.OnlineRecognizer
	tts *sherpa.OfflineTts

	ttsMu  sync.Mutex
	warmAt time.Time
}

// shouldWarm 限制预热频率：本机模型闲置 5 分钟会被卸载，
// 页面停留期间每 3 分钟补一次即可，避免反复抢占 CPU。
func (e *voiceEngine) shouldWarm() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if time.Since(e.warmAt) < 3*time.Minute {
		return false
	}
	e.warmAt = time.Now()
	return true
}

var voiceEng = &voiceEngine{}

// modelsRoot 按优先级找模型目录；找不到时返回默认位置，供状态接口提示缺什么。
func (a *App) modelsRoot() string {
	if custom := strings.TrimSpace(os.Getenv("KAIXIN_MODELS")); custom != "" {
		return custom
	}
	candidates := []string{filepath.Join(a.root, "models")}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(dir, "models"), filepath.Join(dir, "..", "models"))
	}
	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return candidates[0]
}

func voiceASRDir(root string) string { return filepath.Join(root, "asr-zh") }
func voiceTTSDir(root string) string { return filepath.Join(root, "tts-zh") }
func voiceVadModel(root string) string {
	return filepath.Join(root, "vad", "silero_vad.onnx")
}

// voiceMissingFiles 列出缺失的模型文件；返回空表示模型齐备。
func voiceMissingFiles(root string) []string {
	files := []string{
		filepath.Join(voiceASRDir(root), "encoder.int8.onnx"),
		filepath.Join(voiceASRDir(root), "decoder.int8.onnx"),
		filepath.Join(voiceASRDir(root), "joiner.int8.onnx"),
		filepath.Join(voiceASRDir(root), "tokens.txt"),
		filepath.Join(voiceTTSDir(root), "model.onnx"),
		filepath.Join(voiceTTSDir(root), "lexicon.txt"),
		filepath.Join(voiceTTSDir(root), "tokens.txt"),
		voiceVadModel(root),
	}
	missing := make([]string, 0, len(files))
	for _, file := range files {
		if info, err := os.Stat(file); err != nil || info.IsDir() {
			rel, relErr := filepath.Rel(root, file)
			if relErr != nil {
				rel = filepath.Base(file)
			}
			missing = append(missing, rel)
		}
	}
	return missing
}

func (e *voiceEngine) snapshot(root string) VoiceLocalStatus {
	missing := voiceMissingFiles(root)
	status := VoiceLocalStatus{
		ModelsRoot: root,
		ASRModel:   "sherpa-onnx streaming zipformer 中文（int8）",
		TTSModel:   "sherpa-onnx VITS 中文（fanchen-C）",
		SampleRate: voiceSampleRate,
		Missing:    missing,
	}
	if len(missing) > 0 {
		status.Message = "缺少本地语音模型文件，请安装完整版本或把模型放到 " + root
		return status
	}
	e.mu.Lock()
	state := e.state
	e.mu.Unlock()
	switch state {
	case "ready":
		status.Ready = true
		status.Message = "本地语音已就绪"
	case "loading":
		status.Loading = true
		status.Message = "正在加载本地语音模型…"
	case "error":
		e.mu.Lock()
		errMsg := e.errMsg
		e.mu.Unlock()
		status.Message = errMsg
	default:
		status.Message = "本地语音模型尚未加载"
	}
	return status
}

// ensure 完成一次加载；并发调用会排队等同一个加载过程。
func (e *voiceEngine) ensure(root string) error {
	e.mu.Lock()
	switch e.state {
	case "ready":
		e.mu.Unlock()
		return nil
	case "loading":
		wait := e.wait
		e.mu.Unlock()
		<-wait
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.state == "ready" {
			return nil
		}
		return fmt.Errorf("%s", e.errMsg)
	case "error":
		// 允许重试：用户可能刚补上模型文件
	}
	e.state = "loading"
	e.errMsg = ""
	e.wait = make(chan struct{})
	wait := e.wait
	e.mu.Unlock()

	err := e.load(root)

	e.mu.Lock()
	if err != nil {
		e.state = "error"
		e.errMsg = err.Error()
	} else {
		e.state = "ready"
	}
	close(wait)
	e.mu.Unlock()
	return err
}

func (e *voiceEngine) load(root string) error {
	asrDir := voiceASRDir(root)
	recognizer := sherpa.NewOnlineRecognizer(&sherpa.OnlineRecognizerConfig{
		FeatConfig:              sherpa.FeatureConfig{SampleRate: voiceSampleRate, FeatureDim: 80},
		DecodingMethod:          "greedy_search",
		MaxActivePaths:          4,
		EnableEndpoint:          1,
		Rule1MinTrailingSilence: 2.4,
		Rule2MinTrailingSilence: 1.2,
		Rule3MinUtteranceLength: 20,
		ModelConfig: sherpa.OnlineModelConfig{
			Transducer: sherpa.OnlineTransducerModelConfig{
				Encoder: filepath.Join(asrDir, "encoder.int8.onnx"),
				Decoder: filepath.Join(asrDir, "decoder.int8.onnx"),
				Joiner:  filepath.Join(asrDir, "joiner.int8.onnx"),
			},
			Tokens:     filepath.Join(asrDir, "tokens.txt"),
			NumThreads: 2,
			Provider:   "cpu",
			ModelType:  "zipformer",
		},
	})
	if recognizer == nil {
		return fmt.Errorf("中文语音识别模型加载失败，请检查 %s 下的模型文件", asrDir)
	}

	ttsDir := voiceTTSDir(root)
	tts := sherpa.NewOfflineTts(&sherpa.OfflineTtsConfig{
		Model: sherpa.OfflineTtsModelConfig{
			Vits: sherpa.OfflineTtsVitsModelConfig{
				Model:       filepath.Join(ttsDir, "model.onnx"),
				Lexicon:     filepath.Join(ttsDir, "lexicon.txt"),
				Tokens:      filepath.Join(ttsDir, "tokens.txt"),
				NoiseScale:  0.667,
				NoiseScaleW: 0.8,
				LengthScale: 1.0,
			},
			NumThreads: 4,
			Provider:   "cpu",
		},
		MaxNumSentences: 1,
	})
	if tts == nil {
		sherpa.DeleteOnlineRecognizer(recognizer)
		return fmt.Errorf("中文语音合成模型加载失败，请检查 %s 下的模型文件", ttsDir)
	}

	e.mu.Lock()
	e.rec = recognizer
	e.tts = tts
	e.mu.Unlock()
	return nil
}

// synthesizeStream 边合成边回调，让首段音频尽早到达前端。
// ttsMu 保证 4 线程推理不被并发调用；ctx 取消时回调返回 false 提前中断合成。
func (e *voiceEngine) synthesizeStream(ctx context.Context, text string, emit func(samples []float32, sampleRate int) bool) {
	e.mu.Lock()
	tts := e.tts
	e.mu.Unlock()
	if tts == nil {
		return
	}
	e.ttsMu.Lock()
	defer e.ttsMu.Unlock()
	rate := tts.SampleRate()
	tts.GenerateWithCallback(text, 0, voiceSpeechSpeed, func(samples []float32) bool {
		if len(samples) == 0 {
			return ctx.Err() == nil
		}
		return emit(samples, rate) && ctx.Err() == nil
	})
}

// isIdle 表示还没尝试过加载；只有这种情况才自动预热，避免失败后无限重试。
func (e *voiceEngine) isIdle() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state == ""
}

func (a *App) voiceLocalStatus() VoiceLocalStatus {
	root := a.modelsRoot()
	status := voiceEng.snapshot(root)
	if len(status.Missing) == 0 {
		// 首次进入页面时后台加载语音模型，用户点「开始对话」就不用再等。
		if voiceEng.isIdle() {
			go func() {
				if err := voiceEng.ensure(root); err != nil {
					writeVoiceLog("本地语音预热失败: %v", err)
				}
			}()
		}
		// 同时预热本机大模型：冷启动要几十秒，放在页面停留期间完成，
		// 不占用真正对话时的 CPU。
		if voiceEng.shouldWarm() {
			go a.warmupVoiceModel(context.Background())
		}
	}
	return status
}

func (a *App) handleVoiceLocalStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.voiceLocalStatus())
}

// ensureVoiceEngine 在会话建立时等待模型就绪；失败时把原因原样返回给前端。
func (a *App) ensureVoiceEngine(ctx context.Context, root string) error {
	if missing := voiceMissingFiles(root); len(missing) > 0 {
		return fmt.Errorf("缺少本地语音模型：%s", strings.Join(missing, "、"))
	}
	done := make(chan error, 1)
	go func() { done <- voiceEng.ensure(root) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// 以下为 WebSocket 会话协议，字段全部显式声明，前端按 type 分发。
type voiceInbound struct {
	Type  string `json:"type"`
	Audio string `json:"audio"`
}

type voiceEvent struct {
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	Audio      string `json:"audio,omitempty"`
	SampleRate int    `json:"sampleRate,omitempty"`
	Message    string `json:"message,omitempty"`
	Model      string `json:"model,omitempty"`
}

type voiceSession struct {
	app  *App
	conn *websocket.Conn

	stream *sherpa.OnlineStream
	rec    *sherpa.OnlineRecognizer
	vad    *sherpa.VoiceActivityDetector

	ctx    context.Context
	cancel context.CancelFunc

	writeMu sync.Mutex

	mu         sync.Mutex
	respSeq    int64
	respCancel context.CancelFunc
	speaking   bool
	history    []ChatMessage

	lastPartial  string
	speechStreak int
	// preRoll 缓存 AI 说话期间的最近半秒音频：用户插话时补喂给识别，
	// 避免把「等一下」这类短句的开头吃掉。
	preRoll []float32
}

func (s *voiceSession) send(event voiceEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	writeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = s.conn.Write(writeCtx, websocket.MessageText, payload)
}

func (s *voiceSession) isSpeaking() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.speaking
}

// feedAudio 是音频主链路：先做 VAD 插话判断，再交给流式识别。
func (s *voiceSession) feedAudio(samples []float32) {
	barged := false
	if s.vad != nil {
		for i := 0; i < len(samples); i += voiceChunkWindows {
			end := i + voiceChunkWindows
			if end > len(samples) {
				end = len(samples)
			}
			s.vad.AcceptWaveform(samples[i:end])
			speech := s.vad.IsSpeech()
			for !s.vad.IsEmpty() {
				s.vad.Pop()
			}
			if speech {
				s.speechStreak++
				if s.speechStreak >= voiceBargeWindows && s.isSpeaking() {
					barged = true
					break
				}
			} else {
				s.speechStreak = 0
			}
		}
	}
	if barged {
		s.appendPreRoll(samples)
		s.interrupt("barge_in")
		return
	}
	if s.isSpeaking() {
		// AI 说话期间不做识别，只留最近半秒音频，防止扬声器回声触发自问自答。
		s.appendPreRoll(samples)
		return
	}
	s.preRoll = s.preRoll[:0]

	s.stream.AcceptWaveform(voiceSampleRate, samples)
	for s.rec.IsReady(s.stream) {
		s.rec.Decode(s.stream)
	}

	text := strings.TrimSpace(s.rec.GetResult(s.stream).Text)
	endpoint := s.rec.IsEndpoint(s.stream)
	if text != "" && !endpoint && text != s.lastPartial {
		s.lastPartial = text
		s.send(voiceEvent{Type: "partial", Text: text})
	}
	if !endpoint {
		return
	}
	s.rec.Reset(s.stream)
	s.lastPartial = ""
	if text == "" {
		return
	}
	s.send(voiceEvent{Type: "final", Text: text})
	s.startResponse(text)
}

// interrupt 停止当前回答与播放；前端收到事件后立即清空播放队列。
func (s *voiceSession) interrupt(reason string) {
	s.mu.Lock()
	cancel := s.respCancel
	s.respCancel = nil
	s.speaking = false
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if len(s.preRoll) > 0 {
		// 用缓存的插话片段重新开始识别，别把用户第一句话的开头丢掉。
		s.rec.Reset(s.stream)
		s.lastPartial = ""
		s.stream.AcceptWaveform(voiceSampleRate, s.preRoll)
		s.preRoll = s.preRoll[:0]
	}
	s.send(voiceEvent{Type: "interrupt", Message: reason})
}

// appendPreRoll 维护最近半秒的音频缓存（8kHz 采样点 * 0.5s = 4000，16k 下 8000）。
func (s *voiceSession) appendPreRoll(samples []float32) {
	s.preRoll = append(s.preRoll, samples...)
	if excess := len(s.preRoll) - voicePreRollSamples; excess > 0 {
		s.preRoll = s.preRoll[excess:]
	}
}

func (s *voiceSession) startResponse(userText string) {
	s.mu.Lock()
	s.respSeq++
	seq := s.respSeq
	prev := s.respCancel
	ctx, cancel := context.WithCancel(s.ctx)
	s.respCancel = cancel
	s.speaking = true
	s.history = append(s.history, ChatMessage{Role: "user", Content: userText})
	if len(s.history) > voiceHistoryLimit {
		s.history = s.history[len(s.history)-voiceHistoryLimit:]
	}
	messages := make([]ChatMessage, 0, len(s.history)+1)
	messages = append(messages, ChatMessage{Role: "system", Content: voiceSystemPrompt})
	messages = append(messages, s.history...)
	s.mu.Unlock()

	if prev != nil {
		prev()
	}
	go s.respond(ctx, seq, cancel, messages)
}

func (s *voiceSession) respond(ctx context.Context, seq int64, cancel context.CancelFunc, messages []ChatMessage) {
	defer func() {
		interrupted := ctx.Err() != nil
		cancel()
		s.mu.Lock()
		current := s.respSeq == seq
		if current {
			s.speaking = false
			s.respCancel = nil
		}
		s.mu.Unlock()
		if current && !interrupted {
			s.send(voiceEvent{Type: "end"})
		}
	}()

	s.send(voiceEvent{Type: "response.start"})
	started := time.Now()

	var full strings.Builder
	var pending strings.Builder

	speak := func(text string) {
		text = cleanForSpeech(text)
		if text == "" || ctx.Err() != nil {
			return
		}
		ttsStart := time.Now()
		samplesTotal := 0
		voiceEng.synthesizeStream(ctx, text, func(samples []float32, sampleRate int) bool {
			if ctx.Err() != nil {
				// 已经被打断：剩余分片直接丢弃，避免打断后还在播
				return false
			}
			samplesTotal += len(samples)
			s.send(voiceEvent{
				Type:       "tts",
				Audio:      encodeFloat32(samples),
				SampleRate: sampleRate,
			})
			return true
		})
		if samplesTotal > 0 {
			writeVoiceLog("TTS %d 字 -> %.2f 秒音频，用时 %d ms",
				utf8.RuneCountInString(text),
				float64(samplesTotal)/float64(voiceSampleRate),
				time.Since(ttsStart).Milliseconds())
		}
	}

	err := s.app.streamChat(ctx, messages, func(chunk streamChunk) {
		if chunk.Delta == "" {
			return
		}
		full.WriteString(chunk.Delta)
		s.send(voiceEvent{Type: "delta", Text: chunk.Delta})
		pending.WriteString(chunk.Delta)
		for {
			sentence, rest, ok := splitSentence(pending.String())
			if !ok {
				return
			}
			pending.Reset()
			pending.WriteString(rest)
			if sentence != "" {
				speak(sentence)
			}
			if ctx.Err() != nil {
				return
			}
		}
	})
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		s.send(voiceEvent{Type: "error", Message: err.Error()})
		return
	}
	speak(pending.String())

	answer := strings.TrimSpace(full.String())
	if answer == "" {
		return
	}
	s.mu.Lock()
	s.history = append(s.history, ChatMessage{Role: "assistant", Content: answer})
	if len(s.history) > voiceHistoryLimit {
		s.history = s.history[len(s.history)-voiceHistoryLimit:]
	}
	s.mu.Unlock()
	writeVoiceLog("回答完成：%d 字，总用时 %d ms", utf8.RuneCountInString(answer), time.Since(started).Milliseconds())
}

// handleVoiceLocal 建立一次本地语音会话。
func (a *App) handleVoiceLocal(w http.ResponseWriter, r *http.Request) {
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Time{})
	_ = controller.SetWriteDeadline(time.Time{})

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost", "127.0.0.1", "::1", "tauri.localhost"},
	})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(voiceMaxFrameSize)

	root := a.modelsRoot()
	session := &voiceSession{app: a, conn: conn}
	session.ctx, session.cancel = context.WithCancel(r.Context())
	defer session.cancel()

	session.send(voiceEvent{Type: "loading", Message: "正在加载本地语音模型…"})
	if err := a.ensureVoiceEngine(r.Context(), root); err != nil {
		session.send(voiceEvent{Type: "error", Message: err.Error()})
		return
	}
	voiceEng.mu.Lock()
	rec, ttsEngine := voiceEng.rec, voiceEng.tts
	voiceEng.mu.Unlock()
	if rec == nil || ttsEngine == nil {
		session.send(voiceEvent{Type: "error", Message: "本地语音模型尚未就绪"})
		return
	}

	stream := sherpa.NewOnlineStream(rec)
	if stream == nil {
		session.send(voiceEvent{Type: "error", Message: "创建语音识别流失败"})
		return
	}
	defer sherpa.DeleteOnlineStream(stream)
	session.rec = rec
	session.stream = stream

	if vadModel := voiceVadModel(root); fileExists(vadModel) {
		vad := sherpa.NewVoiceActivityDetector(&sherpa.VadModelConfig{
			SileroVad: sherpa.SileroVadModelConfig{
				Model:              vadModel,
				Threshold:          0.5,
				MinSilenceDuration: 0.5,
				MinSpeechDuration:  0.25,
				WindowSize:         voiceChunkWindows,
				MaxSpeechDuration:  20,
			},
			SampleRate: voiceSampleRate,
			NumThreads: 1,
			Provider:   "cpu",
		}, 30)
		if vad != nil {
			defer sherpa.DeleteVoiceActivityDetector(vad)
			session.vad = vad
		}
	}

	session.send(voiceEvent{
		Type:       "ready",
		SampleRate: voiceSampleRate,
		Model:      "sherpa-onnx + 本机大模型",
	})

	for {
		messageType, payload, readErr := conn.Read(session.ctx)
		if readErr != nil {
			break
		}
		if messageType != websocket.MessageText {
			continue
		}
		var frame voiceInbound
		if err := json.Unmarshal(payload, &frame); err != nil {
			continue
		}
		if frame.Type == "close" {
			break
		}
		switch frame.Type {
		case "audio":
			if samples := decodeFloat32(frame.Audio); len(samples) > 0 {
				session.feedAudio(samples)
			}
		case "cancel":
			session.interrupt("client_cancel")
		}
	}
}

// handleVoiceLocalTranscribe 一次性转写：前端录完整段 16k 单声道 float32 PCM 再送上来，
// 复用同一套 sherpa-onnx 中文模型，不依赖 whisper / ffmpeg。
func (a *App) handleVoiceLocalTranscribe(w http.ResponseWriter, r *http.Request) {
	var request struct {
		AudioBase64 string  `json:"audioBase64"`
		SampleRate  int     `json:"sampleRate"`
		Seconds     float64 `json:"seconds"`
		Peak        float64 `json:"peak"`
	}
	// 转写请求体远大于普通接口（59 秒录音的 base64 就超过 5 MB），
	// 不能复用带 4 MB 上限的 decodeJSON。
	body := http.MaxBytesReader(w, r.Body, asrMaxBytes+(1<<20))
	if err := json.NewDecoder(body).Decode(&request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if request.AudioBase64 == "" {
		fail(w, http.StatusBadRequest, "音频为空")
		return
	}
	if len(request.AudioBase64) > asrMaxBytes {
		fail(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("音频过大（约 %.1f MB），单次上限 25 MB（约 %d 秒）",
				float64(len(request.AudioBase64))/(1<<20)*0.75, asrMaxSeconds))
		return
	}

	samples := decodeFloat32(request.AudioBase64)
	if len(samples) == 0 {
		fail(w, http.StatusBadRequest, "没有采集到音频数据")
		return
	}
	if len(samples) > voiceSampleRate*asrMaxSeconds {
		fail(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("录音过长（约 %d 秒），单次上限 %d 秒", len(samples)/voiceSampleRate, asrMaxSeconds))
		return
	}

	// 排障信息落文件：前端上报的采样率/时长/峰值，以及服务端实测峰值。
	// 识别为空时靠这行区分「采集到静音」和「模型没识别出来」。
	peak := 0.0
	for _, sample := range samples {
		if value := math.Abs(float64(sample)); value > peak {
			peak = value
		}
	}
	a.logTranscribeDiag("request clientRate=%d clientSeconds=%.1f clientPeak=%.3f samples=%d seconds=%.1f serverPeak=%.3f",
		request.SampleRate, request.Seconds, request.Peak, len(samples),
		float64(len(samples))/voiceSampleRate, peak)

	root := a.modelsRoot()
	if missing := voiceMissingFiles(root); len(missing) > 0 {
		fail(w, http.StatusServiceUnavailable, "缺少本地语音模型："+strings.Join(missing, "、"))
		return
	}
	if err := a.ensureVoiceEngine(r.Context(), root); err != nil {
		fail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	voiceEng.mu.Lock()
	recognizer := voiceEng.rec
	voiceEng.mu.Unlock()
	if recognizer == nil {
		fail(w, http.StatusServiceUnavailable, "本地语音模型尚未就绪")
		return
	}

	started := time.Now()
	stream := sherpa.NewOnlineStream(recognizer)
	if stream == nil {
		fail(w, http.StatusInternalServerError, "创建语音识别流失败")
		return
	}
	defer sherpa.DeleteOnlineStream(stream)

	// 分段喂入并即时解码：既控制内存，也避免最后一次性解码太久。
	const step = voiceSampleRate * 10
	for offset := 0; offset < len(samples); offset += step {
		end := offset + step
		if end > len(samples) {
			end = len(samples)
		}
		stream.AcceptWaveform(voiceSampleRate, samples[offset:end])
		for recognizer.IsReady(stream) {
			recognizer.Decode(stream)
		}
	}
	stream.InputFinished()
	for recognizer.IsReady(stream) {
		recognizer.Decode(stream)
	}

	text := strings.TrimSpace(recognizer.GetResult(stream).Text)
	a.logTranscribeDiag("result chars=%d elapsed=%dms", utf8.RuneCountInString(text), time.Since(started).Milliseconds())
	writeJSON(w, http.StatusOK, ASRResult{
		Text:      text,
		ElapsedMs: time.Since(started).Milliseconds(),
		Language:  "zh",
	})
}

// warmupVoiceModel 触发一次极短的对话，让本机模型提前完成冷启动。
func (a *App) warmupVoiceModel(ctx context.Context) {
	started := time.Now()
	if _, err := a.chatOnce(ctx, []ChatMessage{{Role: "user", Content: "你好"}}); err != nil {
		writeVoiceLog("模型预热失败: %v", err)
		return
	}
	writeVoiceLog("模型预热完成，用时 %d ms", time.Since(started).Milliseconds())
}

// ---------------------------------------------------------------------------
// 音频编解码与文本切句
// ---------------------------------------------------------------------------

func encodeFloat32(samples []float32) string {
	buf := make([]byte, len(samples)*4)
	for i, sample := range samples {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(sample))
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func decodeFloat32(raw string) []float32 {
	if raw == "" {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(data) < 4 {
		return nil
	}
	count := len(data) / 4
	samples := make([]float32, count)
	for i := 0; i < count; i++ {
		samples[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return samples
}

// splitSentence 按标点切句：逗号也要切，保证首句尽快出声；
// 太短的片段继续累积，避免把「好，」这种碎句单独合成。
func splitSentence(text string) (sentence, rest string, ok bool) {
	if idx := strings.IndexAny(text, "。！？!?；;\n"); idx >= 0 {
		_, size := utf8.DecodeRuneInString(text[idx:])
		return text[:idx+size], text[idx+size:], true
	}
	if idx := strings.IndexAny(text, "，,"); idx >= 0 {
		_, size := utf8.DecodeRuneInString(text[idx:])
		if utf8.RuneCountInString(text[:idx]) >= voiceMinSegmentRunes {
			return text[:idx+size], text[idx+size:], true
		}
	}
	if utf8.RuneCountInString(text) >= 60 {
		return text, "", true
	}
	return "", text, false
}

// cleanForSpeech 去掉不适合朗读的 Markdown 符号与空白。
func cleanForSpeech(text string) string {
	replacer := strings.NewReplacer("*", "", "#", "", "`", "", ">", "", "~", "", "|", "")
	cleaned := strings.TrimSpace(replacer.Replace(text))
	trimmed := strings.Trim(cleaned, "，,、。.！!？?；;：: \t\r\n")
	if trimmed == "" {
		return ""
	}
	return cleaned
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// 日志只写状态，不打印音频与对话内容。
func writeVoiceLog(format string, args ...any) {
	log.Printf("[VOICE] "+format, args...)
}

// logTranscribeDiag 把一次性转写的关键指标追加到 <root>/logs/voice-transcribe.log，
// 便于在用户机器上直接排查「识别为空」这类问题；不记录音频内容。
func (a *App) logTranscribeDiag(format string, args ...any) {
	dir := filepath.Join(a.root, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	file, err := os.OpenFile(filepath.Join(dir, "voice-transcribe.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = fmt.Fprintf(file, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}
