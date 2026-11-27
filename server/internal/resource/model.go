// Package resource 实现资源库：CRUD、审核状态机、去重与质量分。
//
// 分层：handler → service → repository。
// 联系方式独立表 resource_contacts，检索/列表接口只读脱敏主表。
package resource

import "time"

// 资源状态
const (
	StatusDraft    = "draft"
	StatusPending  = "pending"
	StatusOnline   = "online"
	StatusRejected = "rejected"
	StatusOffline  = "offline"
	StatusExpired  = "expired"
)

// 提供方类型
const (
	ProviderEnterprise = "enterprise"
	ProviderIndividual = "individual"
	ProviderBroker     = "broker"
)

// Resource 主表实体（脱敏）。对外统一 camelCase，内部 id 不暴露。
type Resource struct {
	ID           int64      `json:"-"`
	UUID         string     `json:"uuid"`
	OwnerUserID  int64      `json:"ownerUserId"`
	CategoryCode string     `json:"categoryCode"`
	Title        string     `json:"title"`
	Summary      string     `json:"summary"`
	DetailMD     string     `json:"detailMd"`
	RegionCode   string     `json:"regionCode"`
	ProviderType string     `json:"providerType"`
	ContactMode  string     `json:"contactMode"`
	Status       string     `json:"status"`
	QualityScore int        `json:"qualityScore"`
	RejectReason string     `json:"rejectReason"`
	ExpiresAt    *time.Time `json:"expiresAt"`
	PublishedAt  *time.Time `json:"publishedAt"`
	CreatedAt    time.Time  `json:"createdAt"`
}

// PublicItem 列表项（不含联系方式）。
type PublicItem struct {
	UUID         string     `json:"uuid"`
	CategoryCode string     `json:"categoryCode"`
	Title        string     `json:"title"`
	Summary      string     `json:"summary"`
	RegionCode   string     `json:"regionCode"`
	ProviderType string     `json:"providerType"`
	QualityScore int        `json:"qualityScore"`
	PublishedAt  *time.Time `json:"publishedAt"`
}

// PublicDetail 详情（脱敏，含分类免责声明）。
type PublicDetail struct {
	PublicItem
	CategoryName   string `json:"categoryName"`
	DetailMD       string `json:"detailMd"`
	DisclaimerText string `json:"disclaimerText"`
	ContactMode    string `json:"contactMode"`
}

// ContactInput 提交时的联系方式（明文仅存在于内存，落库前加密）。
type ContactInput struct {
	ContactName string `json:"contactName" binding:"required"`
	Phone       string `json:"phone"`
	Wechat      string `json:"wechat"`
}

// CreateRequest 资源提交表单。
type CreateRequest struct {
	CategoryCode string       `json:"categoryCode" binding:"required"`
	Title        string       `json:"title" binding:"required"`
	Summary      string       `json:"summary"`
	DetailMD     string       `json:"detailMd"`
	RegionCode   string       `json:"regionCode"`
	ProviderType string       `json:"providerType"`
	Contact      ContactInput `json:"contact"`
	ExpireDays   int          `json:"expireDays"`
	Force        bool         `json:"force"` // 疑似重复时用户确认继续
}

type CreateResult struct {
	UUID         string `json:"uuid"`
	Status       string `json:"status"`
	QualityScore int    `json:"qualityScore"`
	NeedReview   bool   `json:"needReview"`
}

// ReviewRequest 审核动作。
type ReviewRequest struct {
	Action string `json:"action" binding:"required"` // approve | reject | offline
	Reason string `json:"reason"`
}

// ListFilter 查询过滤条件。
type ListFilter struct {
	CategoryCode string
	RegionCode   string
	Keyword      string
	Status       string
	Page         int
	Size         int
}

// ContactCipher 落库用的联系方式密文。
type ContactCipher struct {
	ContactName  string
	PhoneCipher  []byte
	PhoneHMAC    string
	PhoneNonce   []byte
	WechatCipher []byte
	WechatHMAC   string
	WechatNonce  []byte
	DEKWrapped   []byte
	KeyID        string
}

// CreateParams 仓库插入参数。
type CreateParams struct {
	OwnerUserID    int64
	CategoryCode   string
	Title          string
	Summary        string
	DetailMD       string
	RegionCode     string
	ProviderType   string
	Status         string
	QualityScore   int
	DedupHMAC      string
	ContentSimhash int64
	ExpiresAt      *time.Time
}
