package resource

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"gocircle/server/internal/audit"
	"gocircle/server/internal/config"
	"gocircle/server/internal/httpapi"
	"gocircle/server/internal/securebox"
)

type Service struct {
	cfg   *config.Config
	pool  *pgxpool.Pool
	repo  *Repository
	audit *audit.Service
}

func NewService(cfg *config.Config, pool *pgxpool.Pool, repo *Repository, auditSvc *audit.Service) *Service {
	return &Service{cfg: cfg, pool: pool, repo: repo, audit: auditSvc}
}

// ---- 查询 ----

func (s *Service) ListPublic(ctx context.Context, f ListFilter) ([]PublicItem, error) {
	return s.repo.ListPublic(ctx, f)
}

func (s *Service) Detail(ctx context.Context, uuid string) (*PublicDetail, error) {
	d, err := s.repo.GetPublicDetail(ctx, uuid)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, httpapi.NewDomainError(httpapi.CodeResourceNotFound, "资源不存在或已下架", http.StatusNotFound)
		}
		return nil, err
	}
	return d, nil
}

func (s *Service) ListMine(ctx context.Context, userID int64, page, size int) ([]Resource, int, error) {
	return s.repo.ListByOwner(ctx, userID, page, size)
}

func (s *Service) ListAdmin(ctx context.Context, f ListFilter) ([]Resource, int, error) {
	return s.repo.ListAdmin(ctx, f)
}

// ---- 提交 ----

func (s *Service) Create(ctx context.Context, userID int64, traceID string, req *CreateRequest) (*CreateResult, error) {
	if err := validateCreate(req); err != nil {
		return nil, err
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Summary = strings.TrimSpace(req.Summary)
	req.DetailMD = strings.TrimSpace(req.DetailMD)
	req.RegionCode = strings.TrimSpace(req.RegionCode)
	if req.ProviderType == "" {
		req.ProviderType = ProviderEnterprise
	}

	// 分类必须存在且启用（停用后不可提交/检索）。
	_, enabled, _, err := s.repo.GetCategory(ctx, req.CategoryCode)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, httpapi.NewDomainError(httpapi.CodeInvalidArg, "分类不存在", http.StatusBadRequest)
		}
		return nil, err
	}
	if !enabled {
		return nil, httpapi.NewDomainError(httpapi.CodeCategoryDisabled, "该分类已停用", http.StatusBadRequest)
	}

	secret := s.dedupSecret()
	dedupHMAC := DedupKey(secret, req.CategoryCode, req.RegionCode, req.Title)

	// 强去重
	if dup, err := s.repo.FindDuplicate(ctx, dedupHMAC); err != nil {
		return nil, err
	} else if dup != "" && !req.Force {
		return nil, httpapi.NewDomainError(httpapi.CodeDuplicateResource,
			"疑似重复：已存在相同资源（uuid="+dup+"），确认继续请设置 force=true", http.StatusConflict)
	}

	// 弱去重
	if dup, err := s.weakDuplicate(ctx, req); err != nil {
		return nil, err
	} else if dup != "" && !req.Force {
		return nil, httpapi.NewDomainError(httpapi.CodeDuplicateResource,
			"疑似重复：内容相近（uuid="+dup+"），确认继续请设置 force=true", http.StatusConflict)
	}

	quality := QualityScore(req.Title, req.Summary, req.DetailMD, req.RegionCode,
		req.Contact.Phone != "", req.Contact.Wechat != "")

	days := req.ExpireDays
	if days == 0 {
		days = s.cfg.ResourceDefaultExpireDays
	}
	if days < 7 {
		days = 7
	}
	if days > 365 {
		days = 365
	}
	expiresAt := time.Now().UTC().AddDate(0, 0, days)

	params := &CreateParams{
		OwnerUserID:    userID,
		CategoryCode:   req.CategoryCode,
		Title:          req.Title,
		Summary:        req.Summary,
		DetailMD:       req.DetailMD,
		RegionCode:     req.RegionCode,
		ProviderType:   req.ProviderType,
		Status:         StatusPending,
		QualityScore:   quality,
		DedupHMAC:      dedupHMAC,
		ContentSimhash: int64(simhash(req.Title + req.Summary + req.DetailMD)),
		ExpiresAt:      &expiresAt,
	}

	// 事务只在 service 层开启。
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id, err := s.repo.InsertResource(ctx, tx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, httpapi.NewDomainError(httpapi.CodeDuplicateResource, "疑似重复：并发提交了相同资源", http.StatusConflict)
		}
		return nil, fmt.Errorf("insert resource: %w", err)
	}

	contact, err := s.encryptContact(id, &req.Contact)
	if err != nil {
		return nil, err
	}
	if err := s.repo.InsertContact(ctx, tx, id, contact); err != nil {
		return nil, fmt.Errorf("insert contact: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// 重新取一次 uuid。
	created, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	_ = s.audit.Write(ctx, "user", fmt.Sprintf("%d", userID), "RESOURCE_SUBMIT",
		"resource", created.UUID, traceID, map[string]any{"category": req.CategoryCode, "quality": quality})

	return &CreateResult{UUID: created.UUID, Status: created.Status, QualityScore: quality, NeedReview: true}, nil
}

// ---- 审核 ----

func (s *Service) Review(ctx context.Context, uuid, adminID, traceID string, req *ReviewRequest) (*Resource, error) {
	res, err := s.repo.GetByUUID(ctx, uuid)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, httpapi.NewDomainError(httpapi.CodeResourceNotFound, "资源不存在", http.StatusNotFound)
		}
		return nil, err
	}

	var from []string
	var to string
	switch req.Action {
	case "approve":
		from, to = []string{StatusPending}, StatusOnline
	case "reject":
		if strings.TrimSpace(req.Reason) == "" {
			return nil, httpapi.NewDomainError(httpapi.CodeInvalidArg, "拒绝必须填写原因", http.StatusBadRequest)
		}
		from, to = []string{StatusPending}, StatusRejected
	case "offline":
		from, to = []string{StatusOnline}, StatusOffline
	default:
		return nil, httpapi.NewDomainError(httpapi.CodeInvalidArg, "未知审核动作", http.StatusBadRequest)
	}

	ok, err := s.repo.Transition(ctx, res.ID, from, to, req.Reason, adminID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, httpapi.NewDomainError(httpapi.CodeStatusConflict,
			"当前状态不允许该操作（需要状态："+strings.Join(from, "/")+"）", http.StatusConflict)
	}

	_ = s.audit.Write(ctx, "admin", adminID, "RESOURCE_REVIEW", "resource", uuid, traceID,
		map[string]any{"action": req.Action, "from": from, "to": to, "reason": req.Reason})

	return s.repo.GetByUUID(ctx, uuid)
}

// ---- 本人操作 ----

func (s *Service) OfflineOwn(ctx context.Context, uuid string, userID int64, traceID string) error {
	res, err := s.repo.GetByUUID(ctx, uuid)
	if err != nil || res.OwnerUserID != userID {
		// 不暴露他人资源是否存在，统一 404。
		return httpapi.NewDomainError(httpapi.CodeResourceNotFound, "资源不存在", http.StatusNotFound)
	}
	ok, err := s.repo.Transition(ctx, res.ID, []string{StatusOnline}, StatusOffline, "", "")
	if err != nil {
		return err
	}
	if !ok {
		return httpapi.NewDomainError(httpapi.CodeStatusConflict, "只有已上线资源可下架", http.StatusConflict)
	}
	_ = s.audit.Write(ctx, "user", fmt.Sprintf("%d", userID), "RESOURCE_OFFLINE", "resource", uuid, traceID, nil)
	return nil
}

func (s *Service) DeleteOwn(ctx context.Context, uuid string, userID int64, traceID string) error {
	ok, err := s.repo.SoftDelete(ctx, uuid, userID)
	if err != nil {
		return err
	}
	if !ok {
		return httpapi.NewDomainError(httpapi.CodeStatusConflict, "资源不存在或当前状态不可删除", http.StatusConflict)
	}
	_ = s.audit.Write(ctx, "user", fmt.Sprintf("%d", userID), "RESOURCE_DELETE", "resource", uuid, traceID, nil)
	return nil
}

// ---- 定时任务 ----

// ExpireDue 把到期资源置为 expired，返回处理条数。由 main 的定时器调用。
func (s *Service) ExpireDue(ctx context.Context) (int64, error) {
	return s.repo.ExpireDue(ctx)
}

// ---- 内部 ----

func (s *Service) weakDuplicate(ctx context.Context, req *CreateRequest) (string, error) {
	target := simhash(req.Title + req.Summary + req.DetailMD)
	cands, err := s.repo.SimhashCandidates(ctx, req.CategoryCode, 500)
	if err != nil {
		return "", err
	}
	threshold := s.cfg.SimhashThreshold
	for _, c := range cands {
		if Hamming(uint64(c.Simhash), target) <= threshold {
			return c.UUID, nil
		}
	}
	return "", nil
}

func (s *Service) encryptContact(resourceID int64, in *ContactInput) (*ContactCipher, error) {
	key := []byte(s.cfg.MasterKeyEnv)
	if len(key) != 32 {
		return nil, fmt.Errorf("master key 未配置或长度不为 32 字节")
	}
	out := &ContactCipher{ContactName: strings.TrimSpace(in.ContactName), KeyID: "env_s1"}

	if p := normalizePhone(in.Phone); p != "" {
		ct, nonce, err := securebox.EncryptGCM(key, []byte(p),
			fmt.Sprintf("resource_contacts:phone_cipher:%d:env_s1", resourceID))
		if err != nil {
			return nil, err
		}
		out.PhoneCipher, out.PhoneNonce = ct, nonce
		out.PhoneHMAC = securebox.HMACHex(key, "phone", p)
	}
	if w := strings.TrimSpace(in.Wechat); w != "" {
		ct, nonce, err := securebox.EncryptGCM(key, []byte(w),
			fmt.Sprintf("resource_contacts:wechat_cipher:%d:env_s1", resourceID))
		if err != nil {
			return nil, err
		}
		out.WechatCipher, out.WechatNonce = ct, nonce
		out.WechatHMAC = securebox.HMACHex(key, "wechat", w)
	}
	return out, nil
}

func (s *Service) dedupSecret() []byte {
	if s.cfg.DedupSecret != "" {
		return []byte(s.cfg.DedupSecret)
	}
	return []byte(s.cfg.MasterKeyEnv)
}

func validateCreate(req *CreateRequest) error {
	title := strings.TrimSpace(req.Title)
	if n := utf8.RuneCountInString(title); n < 4 || n > 60 {
		return httpapi.NewDomainError(httpapi.CodeInvalidArg, "标题长度需在 4–60 字之间", http.StatusBadRequest)
	}
	if utf8.RuneCountInString(req.Summary) > 200 {
		return httpapi.NewDomainError(httpapi.CodeInvalidArg, "摘要不能超过 200 字", http.StatusBadRequest)
	}
	if utf8.RuneCountInString(req.DetailMD) > 5000 {
		return httpapi.NewDomainError(httpapi.CodeInvalidArg, "详情不能超过 5000 字", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.RegionCode) == "" {
		return httpapi.NewDomainError(httpapi.CodeInvalidArg, "地区不能为空", http.StatusBadRequest)
	}
	if utf8.RuneCountInString(strings.TrimSpace(req.RegionCode)) > 12 {
		return httpapi.NewDomainError(httpapi.CodeInvalidArg, "地区编码非法", http.StatusBadRequest)
	}
	switch req.ProviderType {
	case "", ProviderEnterprise, ProviderIndividual, ProviderBroker:
	default:
		return httpapi.NewDomainError(httpapi.CodeInvalidArg, "提供方类型非法", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Contact.ContactName) == "" {
		return httpapi.NewDomainError(httpapi.CodeInvalidArg, "联系人不能为空", http.StatusBadRequest)
	}
	phone := normalizePhone(req.Contact.Phone)
	wechat := strings.TrimSpace(req.Contact.Wechat)
	if phone == "" && wechat == "" {
		return httpapi.NewDomainError(httpapi.CodeInvalidArg, "手机号与微信至少填写一个", http.StatusBadRequest)
	}
	if phone != "" && (len(phone) != 11 || phone[0] != '1') {
		return httpapi.NewDomainError(httpapi.CodeInvalidArg, "手机号格式不正确", http.StatusBadRequest)
	}
	if wechat != "" && utf8.RuneCountInString(wechat) > 64 {
		return httpapi.NewDomainError(httpapi.CodeInvalidArg, "微信号过长", http.StatusBadRequest)
	}
	return nil
}

func normalizePhone(p string) string {
	var b strings.Builder
	for _, r := range p {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
