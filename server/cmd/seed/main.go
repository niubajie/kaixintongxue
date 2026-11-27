// Command seed 生成演示数据，并走真实的资源提交流程与审核状态机。
//
// 用法（在 server 目录下）：
//
//	go run ./cmd/seed            # 默认 100 条
//	go run ./cmd/seed -count 20  # 自定义条数
//
// 行为：
//  1. 建 10 个演示用户（固定 uuid，幂等）
//  2. 通过 resource.Service.Create 逐条提交资源（真实校验 + 去重 + 质量分 + 联系方式加密）
//  3. 通过 resource.Service.Review 流转：通过 / 拒绝 / 通过后再下架 / 保持待审核
//
// 安全：不删除任何数据；库中已有资源时直接跳过，避免重复造数。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"gocircle/server/internal/audit"
	"gocircle/server/internal/config"
	"gocircle/server/internal/httpapi"
	"gocircle/server/internal/resource"
)

type seedCity struct {
	code      string
	name      string
	districts []string
}

var seedCities = []seedCity{
	{"440300", "深圳", []string{"龙岗区", "宝安区", "龙华区", "坪山区"}},
	{"440100", "广州", []string{"番禺区", "白云区", "黄埔区", "增城区"}},
	{"441900", "东莞", []string{"长安镇", "虎门镇", "寮步镇", "塘厦镇"}},
	{"440600", "佛山", []string{"南海区", "顺德区", "禅城区", "三水区"}},
	{"310000", "上海", []string{"嘉定区", "松江区", "青浦区", "奉贤区"}},
	{"330100", "杭州", []string{"余杭区", "萧山区", "钱塘区", "临平区"}},
	{"320500", "苏州", []string{"吴中区", "相城区", "昆山市", "常熟市"}},
	{"110000", "北京", []string{"大兴区", "顺义区", "通州区", "昌平区"}},
	{"510100", "成都", []string{"双流区", "龙泉驿区", "新都区", "郫都区"}},
	{"420100", "武汉", []string{"东西湖区", "江夏区", "蔡甸区", "黄陂区"}},
}

type seedCategory struct {
	code     string
	provider string
}

var seedCategories = []seedCategory{
	{"factory_office", resource.ProviderEnterprise},
	{"partner_talent", resource.ProviderEnterprise},
	{"industrial_product", resource.ProviderEnterprise},
	{"commodity", resource.ProviderIndividual},
	{"business_service", resource.ProviderBroker},
	{"ip_trade", resource.ProviderEnterprise},
	{"finance", resource.ProviderEnterprise},
	{"other", resource.ProviderBroker},
}

// 固定 uuid，保证多次运行不会重复创建用户。
var seedUserUUIDs = []string{
	"11111111-1111-4111-8111-000000000001",
	"11111111-1111-4111-8111-000000000002",
	"11111111-1111-4111-8111-000000000003",
	"11111111-1111-4111-8111-000000000004",
	"11111111-1111-4111-8111-000000000005",
	"11111111-1111-4111-8111-000000000006",
	"11111111-1111-4111-8111-000000000007",
	"11111111-1111-4111-8111-000000000008",
	"11111111-1111-4111-8111-000000000009",
	"11111111-1111-4111-8111-000000000010",
}

func main() {
	count := flag.Int("count", 100, "生成的资源条数")
	flag.Parse()
	if *count <= 0 {
		log.Fatalf("count 必须大于 0")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config.Load: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DBConnStr)
	if err != nil {
		log.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("postgres ping: %v", err)
	}

	var existing int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM circle.resources WHERE deleted_at IS NULL`).Scan(&existing); err != nil {
		log.Fatalf("count resources: %v", err)
	}
	if existing > 0 {
		log.Printf("[seed] 库中已有 %d 条资源，跳过（本命令不删除、不重复造数）", existing)
		return
	}

	userIDs, err := ensureSeedUsers(ctx, pool)
	if err != nil {
		log.Fatalf("ensure users: %v", err)
	}

	adminID, err := findAdminID(ctx, pool)
	if err != nil {
		log.Fatalf("find admin: %v", err)
	}

	auditSvc := audit.New(pool)
	repo := resource.NewRepository(pool)
	svc := resource.NewService(cfg, pool, repo, auditSvc)

	created := make([]string, 0, *count)
	failures := 0
	retried := 0
	start := time.Now()

	counters := make([]int, len(seedCategories))
	for i := 0; i < *count; i++ {
		catIdx := i % len(seedCategories)
		counters[catIdx]++
		n := counters[catIdx]

		city := seedCities[(i*3+catIdx*7)%len(seedCities)]
		district := city.districts[n%len(city.districts)]

		title, summary, detail := buildContent(i, catIdx, n, city, district)
		req := &resource.CreateRequest{
			CategoryCode: seedCategories[catIdx].code,
			Title:        title,
			Summary:      summary,
			DetailMD:     detail,
			RegionCode:   city.code,
			ProviderType: seedCategories[catIdx].provider,
			Contact: resource.ContactInput{
				ContactName: pick([]string{"张", "王", "李", "赵", "陈", "刘", "周", "吴", "郑", "孙"}, i) +
					pick([]string{"先生", "女士"}, i),
				Phone:  seedPhone(i),
				Wechat: seedWechat(i),
			},
			ExpireDays: 30 + (i%6)*15,
		}
		// 三分之一只留手机、三分之一只留微信，其余两者都有。
		switch i % 3 {
		case 0:
			req.Contact.Wechat = ""
		case 1:
			req.Contact.Phone = ""
		}

		ownerID := userIDs[i%len(userIDs)]
		uuidStr, wasRetried, err := createWithRetry(ctx, svc, ownerID, req, i)
		if err != nil {
			failures++
			log.Printf("[seed] #%03d 创建失败：%v（标题：%s）", i+1, err, req.Title)
			continue
		}
		if wasRetried {
			retried++
		}
		created = append(created, uuidStr)
	}

	// ---- 审核流转 ----
	stats := map[string]int{}
	for i, u := range created {
		trace := fmt.Sprintf("seed-%03d", i+1)
		switch i % 10 {
		case 0, 1, 2, 3, 4, 5: // 通过
			if err := review(ctx, svc, u, adminID, trace, "approve", ""); err != nil {
				log.Printf("[seed] 审核通过失败 %s：%v", u, err)
				stats["reviewFailed"]++
				continue
			}
			stats["online"]++
		case 6, 7: // 拒绝（原因必填）
			reason := pick([]string{
				"联系方式无法接通，请补充有效手机号后重新提交",
				"详情描述过于简单，请补充面积、价格等关键信息",
				"资质或证明缺失，请补充材料后重新提交",
				"疑似重复发布，请勿重复提交同一资源",
			}, i)
			if err := review(ctx, svc, u, adminID, trace, "reject", reason); err != nil {
				log.Printf("[seed] 审核拒绝失败 %s：%v", u, err)
				stats["reviewFailed"]++
				continue
			}
			stats["rejected"]++
		case 8: // 先通过再下架
			if err := review(ctx, svc, u, adminID, trace, "approve", ""); err != nil {
				log.Printf("[seed] 审核通过失败 %s：%v", u, err)
				stats["reviewFailed"]++
				continue
			}
			if err := review(ctx, svc, u, adminID, trace, "offline", ""); err != nil {
				log.Printf("[seed] 审核下架失败 %s：%v", u, err)
				stats["reviewFailed"]++
				continue
			}
			stats["offline"]++
		default: // 保持待审核
			stats["pending"]++
		}
	}

	log.Printf("[seed] 完成：提交 %d 条，失败 %d 条，去重重试 %d 条，用时 %s",
		len(created), failures, retried, time.Since(start).Round(time.Millisecond))
	log.Printf("[seed] 审核分布：通过 %d，拒绝 %d，下架 %d，待审核 %d，审核失败 %d",
		stats["online"], stats["rejected"], stats["offline"], stats["pending"], stats["reviewFailed"])
}

// ensureSeedUsers 幂等创建演示用户并返回其内部 id。
func ensureSeedUsers(ctx context.Context, pool *pgxpool.Pool) ([]int64, error) {
	ids := make([]int64, 0, len(seedUserUUIDs))
	for i, u := range seedUserUUIDs {
		nickname := fmt.Sprintf("种子用户%02d", i+1)
		if _, err := pool.Exec(ctx,
			`INSERT INTO circle.users(uuid, nickname, status) VALUES($1,$2,'active')
			 ON CONFLICT (uuid) DO NOTHING`, u, nickname); err != nil {
			return nil, err
		}
		var id int64
		if err := pool.QueryRow(ctx, `SELECT id FROM circle.users WHERE uuid=$1`, u).Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// findAdminID 取一个超级管理员作为审核人。
func findAdminID(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var id int64
	err := pool.QueryRow(ctx,
		`SELECT id FROM circle.admin_users WHERE role_code='super_admin' ORDER BY id LIMIT 1`).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("未找到超级管理员，请先启动一次 api 服务完成 bootstrap：%w", err)
	}
	return fmt.Sprintf("%d", id), nil
}

// createWithRetry 提交资源；若被弱/强去重拦截，追加唯一标记后重试一次。
func createWithRetry(ctx context.Context, svc *resource.Service, ownerID int64, req *resource.CreateRequest, idx int) (string, bool, error) {
	res, err := svc.Create(ctx, ownerID, fmt.Sprintf("seed-%03d", idx+1), req)
	if err == nil {
		return res.UUID, false, nil
	}
	if !isDuplicate(err) {
		return "", false, err
	}

	// 标题加序号（改变强去重 key），详情追加唯一段落（改变 SimHash）。
	req.Title = trimToRunes(req.Title, 50) + fmt.Sprintf("（%03d）", idx+1)
	req.DetailMD += retryParagraph(idx)
	res, err = svc.Create(ctx, ownerID, fmt.Sprintf("seed-%03d-r", idx+1), req)
	if err == nil {
		return res.UUID, true, nil
	}
	if !isDuplicate(err) {
		return "", true, err
	}

	// 兜底：模板化正文仍可能命中弱去重，种子数据显式确认继续（强去重 key 已因标题变化而不同）。
	req.Force = true
	res, err = svc.Create(ctx, ownerID, fmt.Sprintf("seed-%03d-f", idx+1), req)
	if err != nil {
		return "", true, err
	}
	return res.UUID, true, nil
}

// retryParagraph 生成一段与模板正文差异较大的补充说明，用于绕过弱去重。
func retryParagraph(idx int) string {
	pools := [][]string{
		{"现场核验", "视频看房", "样品寄送", "资质预审", "方案报价", "合同模板", "进度周报", "专人对接", "分批交付", "售后回访", "验收清单", "电子签章"},
		{"工作日响应", "节假日值班", "同城上门", "跨区协调", "线上会议", "实地考察", "第三方检测", "保险投保", "物流跟踪", "票据齐全"},
		{"价格透明", "节点付款", "违约说明", "保密条款", "验收标准", "变更流程", "退款规则", "争议协商", "长期优惠", "增值服务"},
		{"可开发票", "支持对公", "签订合同", "提供样例", "试运行期", "专属顾问", "定期复盘", "数据留痕"},
	}
	return fmt.Sprintf("\n\n补充说明：本资源为种子数据，编号 SEED-%03d；可提供%s、%s、%s与%s，同时支持%s、%s和%s，具体条款以双方书面约定为准。",
		idx+1,
		pick(pools[0], idx), pick(pools[1], idx*3+1), pick(pools[2], idx*5+2), pick(pools[3], idx*7+3),
		pick(pools[1], idx*11+4), pick(pools[2], idx*13+5), pick(pools[3], idx*17+6))
}

func review(ctx context.Context, svc *resource.Service, uuidStr, adminID, trace, action, reason string) error {
	_, err := svc.Review(ctx, uuidStr, adminID, trace, &resource.ReviewRequest{Action: action, Reason: reason})
	return err
}

func isDuplicate(err error) bool {
	var de *httpapi.DomainError
	return errors.As(err, &de) && de.Code == httpapi.CodeDuplicateResource
}

func seedPhone(i int) string {
	return fmt.Sprintf("13%09d", 100000000+i*137)
}

func seedWechat(i int) string {
	return fmt.Sprintf("wx_seed_%03d", i+1)
}

func pick[T any](items []T, n int) T {
	return items[n%len(items)]
}

func trimToRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// buildContent 按分类生成标题 / 摘要 / 详情。n 为该分类内的序号。
func buildContent(idx, catIdx, n int, city seedCity, district string) (string, string, string) {
	place := city.name + district
	switch seedCategories[catIdx].code {
	case "factory_office":
		park := pick([]string{"智造园", "科创园", "物流园", "工业园", "产业园", "高新园", "装备园", "电子城", "五金城", "食品园", "医疗园", "新能源园", "数智园"}, n)
		area := 600 + n*130
		price := 18 + n%7
		floorH := 5 + n%3
		title := fmt.Sprintf("%s%s独院厂房出租 %d平方米 可环评", place, park, area)
		summary := fmt.Sprintf("园区位于%s，独栋厂房建筑面积%d平方米，层高%d米，配%d吨行车与货梯，租金每平方米每月%d元，可整租可分租，工业用电独立计量，适合生产、仓储与研发办公，工作日与周末均可预约看房。",
			place, area, floorH, 3+n%3, price)
		detail := strings.Join([]string{
			"一、房源概况",
			fmt.Sprintf("本房源位于%s%s，独栋单层厂房，建筑面积%d平方米，层高%d米，柱距8米，地面承重每平方米1.2吨，可满足中型设备进场与生产线布置。", place, park, area, floorH),
			"二、配套条件",
			fmt.Sprintf("园区配备%d吨货梯与%d吨行车各一台，工业用电容量%d千伏安，水电独立计量，配套员工宿舍%d间与食堂，货车可直达装卸平台。", 3+n%3, 5+n%3, 400+n*20, 6+n%5),
			"三、租金与交付",
			fmt.Sprintf("租金每平方米每月%d元，物业费每平方米每月%d元，押金三个月，最短租期两年，签约后%d天内可交付使用，租金可按年小幅递增。", price, 3+n%3, 15+n%10),
			"四、交通与周边",
			fmt.Sprintf("距离高速入口约%d公里，距物流园约%d公里，周边劳动力资源充足，公交线路覆盖，员工通勤与货运都较方便。", 3+n%4, 5+n%3),
			"五、看房方式",
			fmt.Sprintf("工作日与周末均可预约看房，请通过平台联系方式与发布方对接，沟通时说明资源编号 SEED-%03d，便于快速核验房源。", idx+1),
		}, "\n\n")
		return title, summary, detail

	case "partner_talent":
		project := pick([]string{"跨境电商", "社区团购", "工业品分销", "餐饮连锁", "企业服务", "品牌代运营", "仓储物流", "设备租赁", "农产品供应链", "企业培训", "财税服务", "短视频运营", "智能制造配套"}, n)
		turnover := 300 + n*90
		title := fmt.Sprintf("%s%s项目寻合伙人 利润分成可谈", place, project)
		summary := fmt.Sprintf("%s的%s项目现寻运营合伙人，项目已有稳定客户与供应链基础，年营业额约%d万元，希望合作方带来渠道、团队或资金资源，分成比例与出资方式面谈，合作前可先小范围试跑验证。",
			place, project, turnover)
		detail := strings.Join([]string{
			"一、项目介绍",
			fmt.Sprintf("项目位于%s，主营%s，已运营%d年，现有团队%d人，年营业额约%d万元，客户以本地中小企业为主，供应链与交付流程已经跑通。", place, project, 1+n%4, 5+n%6, turnover),
			"二、合作方式",
			fmt.Sprintf("计划引入%d名合伙人，可采取资源入股、现金入股或纯分成合作三种方式，具体股权比例、分红节奏与退出机制在尽职沟通后签署书面协议。", 1+n%3),
			"三、对合伙方的期待",
			"希望合作方在渠道拓展、团队管理或供应链整合方面具备实际经验，能够投入稳定时间参与经营，认同长期合作与合规经营的原则。",
			"四、收益与风险提示",
			"合作收益取决于实际经营结果，存在市场波动与经营风险，双方需在协议中明确权责，平台仅提供信息展示，不参与具体合作谈判。",
			"五、对接方式",
			fmt.Sprintf("请通过平台联系方式与发布方沟通，说明资源编号 SEED-%03d 及可投入的资源类型，便于对方判断合作匹配度。", idx+1),
		}, "\n\n")
		return title, summary, detail

	case "industrial_product":
		factory := pick([]string{"恒信机械", "华工精工", "联创智造", "金鼎重工", "瑞达机电", "宏泰五金", "正泰装备", "中科自动化", "东方精机", "新元制造", "卓越电控", "德立机械", "天工部件"}, n)
		product := pick([]string{"不锈钢法兰", "精密齿轮", "液压油缸", "工业铝型材", "变频器", "伺服电机", "输送带", "轴承座", "模具钢材", "包装机械", "空压机", "减速机", "配电柜"}, n)
		moq := 50 + n*10
		title := fmt.Sprintf("%s%s%s厂家直供 %s 现货", city.name, district, factory, product)
		summary := fmt.Sprintf("%s%s的%s厂家直供%s，常用规格现货充足，起订量%d件，支持来图定制与非标加工，出厂附带材质证明与检验报告，交期%d天，可发全国物流。",
			city.name, district, factory, product, moq, 3+n%7)
		detail := strings.Join([]string{
			"一、产品说明",
			fmt.Sprintf("本产品为%s生产的%s，采用国标原材料，关键尺寸经三坐标检测，批次可追溯，适用于机械设备、自动化产线与工程配套等场景。", factory, product),
			"二、规格与起订",
			fmt.Sprintf("常规规格覆盖%d种型号，起订量%d件，批量采购可按阶梯价结算，非标尺寸支持来图定制，打样周期约%d天。", 8+n%10, moq, 5+n%5),
			"三、质量与认证",
			fmt.Sprintf("工厂通过 ISO9001 质量管理体系认证，出厂提供材质证明、尺寸检验报告与合格证，质保期%d个月，非人为损坏可换货。", 12+n%13),
			"四、物流与交付",
			fmt.Sprintf("常用规格下单后%d天内发货，支持汽运、铁路与快递，运费按实际重量与目的地计算，大件可协助安排专线物流。", 3+n%7),
			"五、对接方式",
			fmt.Sprintf("请通过平台联系方式提供所需型号、数量与交期，说明资源编号 SEED-%03d，厂家将给出正式报价单。", idx+1),
		}, "\n\n")
		return title, summary, detail

	case "commodity":
		product := pick([]string{"抽纸", "洗衣液", "保温杯", "数据线", "收纳箱", "纯棉毛巾", "厨房用具", "儿童玩具", "办公文具", "宠物用品", "小家电", "零食礼盒", "洗护套装"}, n)
		spec := pick([]string{"家庭装", "商用装", "礼盒装", "便携装", "组合装", "整箱装", "散装"}, n)
		title := fmt.Sprintf("%s%s%s批发 %s 支持一件代发", city.name, district, spec, product)
		summary := fmt.Sprintf("%s%s的批发商供应%s%s，规格齐全，起批量%d件，支持一件代发与贴牌定制，日均发货能力%d单，可对接电商平台与社区团购，售后按平台规则处理。",
			city.name, district, spec, product, 10+n*5, 500+n*60)
		detail := strings.Join([]string{
			"一、商品说明",
			fmt.Sprintf("本商品为%s%s，适合家庭、办公与礼赠场景，包装完整，批次新鲜，支持查看实拍图与检测报告。", spec, product),
			"二、价格与起批",
			fmt.Sprintf("起批量%d件，批量越大单价越低，支持混批凑单，一件代发不加价，贴牌定制起订量%d件，需提前确认包装设计稿。", 10+n*5, 200+n*20),
			"三、发货与物流",
			fmt.Sprintf("现货下单后%d小时内出库，默认发快递，偏远地区改发物流，日均发货能力%d单，可提供物流单号与售后跟踪。", 12+n%12, 500+n*60),
			"四、售后说明",
			"商品签收后七天内支持按平台规则申请售后，非质量问题运费由买家承担，质量问题由供货方承担往返运费并补发。",
			"五、对接方式",
			fmt.Sprintf("请通过平台联系方式说明所需规格、数量与发货方式，资源编号 SEED-%03d，供货方将提供最新报价表。", idx+1),
		}, "\n\n")
		return title, summary, detail

	case "business_service":
		agency := pick([]string{"启航企服", "正合商务", "恒信财税", "领航咨询", "中企服务", "诚达代办", "睿信商务", "众合企服", "金桥咨询", "博远商务", "同创企服", "安信代办", "和顺商务"}, n)
		service := pick([]string{"公司注册", "代理记账", "商标注册", "资质代办", "社保代缴", "税务筹划", "审计报告", "高新认定", "ISO认证", "知识产权贯标", "工商变更", "注销清算", "人力资源服务"}, n)
		days := 3 + n%15
		title := fmt.Sprintf("%s%s%s %s 全程跟进", city.name, district, agency, service)
		summary := fmt.Sprintf("%s%s的%s提供%s服务，熟悉本地办理流程，材料清单一次性告知，常规业务%d个工作日内完成，费用透明可分阶段支付，进度全程可查询，服务过程签署正式合同。",
			city.name, district, agency, service, days)
		detail := strings.Join([]string{
			"一、服务内容",
			fmt.Sprintf("%s位于%s，主营%s服务，本次提供%s的咨询、材料准备、递交与跟进，直至事项办结并交付结果文件。", agency, place, service, service),
			"二、办理周期",
			fmt.Sprintf("在材料齐全的前提下，常规业务约%d个工作日完成；涉及主管部门审批的事项，以实际受理进度为准，服务方会同步每个节点的状态。", days),
			"三、费用说明",
			fmt.Sprintf("服务费按事项分档收取，本次报价区间为%d元至%d元，官方规费按实代缴并提供凭证，签署合同后按节点付款，不额外收取隐性费用。", 800+n*100, 2000+n*300),
			"四、所需材料",
			"服务方会在对接时提供完整材料清单，包括主体证明、身份证明、场地证明与授权文件等，仅收集办理所必需的信息，并承诺按约定用途使用。",
			"五、对接方式",
			fmt.Sprintf("请通过平台联系方式说明具体需求与所在区域，资源编号 SEED-%03d，服务方将安排顾问一对一沟通。", idx+1),
		}, "\n\n")
		return title, summary, detail

	case "ip_trade":
		ip := pick([]string{"第9类电子商标", "第35类广告销售商标", "第42类软件商标", "发明专利", "实用新型专利", "外观设计专利", "软件著作权", "第30类食品商标", "第25类服装商标", "第43类餐饮商标", "第41类教育商标", "第36类金融商标", "第39类物流商标"}, n)
		regNo := 30000000 + n*137 + idx
		title := fmt.Sprintf("%s%s%s转让 注册号%d 已下证", city.name, district, ip, regNo)
		summary := fmt.Sprintf("%s%s的%s现对外转让，注册号%d，权利状态有效，无质押与许可备案，可提供证书扫描件核验，转让流程支持公证与平台见证，整体周期约%d个月，费用含官费与代理服务费。",
			city.name, district, ip, regNo, 2+n%4)
		detail := strings.Join([]string{
			"一、标的情况",
			fmt.Sprintf("本次转让标的为%s，注册号%d，申请地与权利人信息可在官方系统核验，当前权利状态有效，未设定质押，未许可他人使用。", ip, regNo),
			"二、适用范围",
			"标的可用于对应类别的商品或服务经营，受让方需根据自身业务确认类别覆盖范围，必要时可同步补充注册相邻类别。",
			"三、转让流程",
			fmt.Sprintf("双方确认意向后签署转让合同，办理公证与著录项目变更，官方审查通过后下发变更证明，整体周期约%d个月，遇补正将顺延。", 2+n%4),
			"四、费用构成",
			fmt.Sprintf("报价含标的转让费、官费与代理服务费，合计区间为%d元至%d元，具体以标的类别与知名度协商确定，所有款项走合同约定账户。", 8000+n*800, 20000+n*1500),
			"五、对接方式",
			fmt.Sprintf("请通过平台联系方式索取证书扫描件与权利状态截图，资源编号 SEED-%03d，核实后再进入合同流程。", idx+1),
		}, "\n\n")
		return title, summary, detail

	case "finance":
		topic := pick([]string{"制造业企业股权融资信息", "供应链服务企业股权融资信息", "新能源企业股权融资信息", "消费品牌企业股权融资信息", "软件服务企业股权融资信息", "农业科技企业股权融资信息", "医疗器械企业股权融资信息", "物流科技企业股权融资信息", "教育服务企业股权融资信息", "环保科技企业股权融资信息", "文旅企业股权融资信息", "食品加工企业股权融资信息", "智能硬件企业股权融资信息"}, n)
		revenue := 800 + n*260
		title := fmt.Sprintf("%s%s%s 仅展示企业信息", city.name, district, topic)
		summary := fmt.Sprintf("%s%s的%s，企业成立%d年，上一年度营业收入约%d万元，团队规模%d人，现对外展示股权融资意向信息，具体估值与条款由企业自行沟通。本平台仅展示企业信息，不提供投融资中介服务，不构成投资建议。",
			city.name, district, topic, 2+n%6, revenue, 10+n*5)
		detail := strings.Join([]string{
			"一、企业基本情况",
			fmt.Sprintf("企业位于%s，主营%s相关业务，成立%d年，上一年度营业收入约%d万元，现有团队%d人，具备完整的经营资质与稳定的客户结构。", place, topic, 2+n%6, revenue, 10+n*5),
			"二、融资意向",
			fmt.Sprintf("企业意向融资额度区间为%d万元至%d万元，资金主要用于产能扩张、研发投入与市场拓展，具体股权比例与估值由企业与合作方自行商谈。", 500+n*100, 1500+n*300),
			"三、合规声明",
			"本平台仅提供企业信息展示服务，不开展投融资中介、借贷撮合或任何形式的金融业务，不对信息的完整性与投资价值作出保证，不构成投资建议。",
			"四、风险提示",
			"股权投资存在较高风险，可能面临本金损失，任何合作前请自行完成尽职调查并咨询专业机构，相关协议与资金往来均由当事方自行负责。",
			"五、对接方式",
			fmt.Sprintf("如为企业方或意向合作方，可通过平台联系方式进一步核实企业信息，资源编号 SEED-%03d，平台不参与双方后续商业谈判。", idx+1),
		}, "\n\n")
		return title, summary, detail

	default: // other
		asset := pick([]string{"闲置仓储空间", "社区活动场地", "直播基地工位", "冷链仓储资源", "会展展位", "跨境物流渠道", "检测实验室档期", "摄影棚场地", "共享办公工位", "货运车队运力", "农产品产地资源", "二手生产设备", "户外广告位"}, n)
		title := fmt.Sprintf("%s%s%s合作 欢迎对接", city.name, district, asset)
		summary := fmt.Sprintf("%s%s现有%s对外合作，可用面积或数量为%d，合作方式包括短租、长租与联合运营，价格按实际使用量协商，支持先小规模试用，双方签署正式协议后执行。",
			city.name, district, asset, 1+n%9)
		detail := strings.Join([]string{
			"一、资源说明",
			fmt.Sprintf("本次对外合作的资源为%s，位于%s%s，权属清晰，可提供现场查看，适合有短期或阶段性需求的合作方使用。", asset, city.name, district),
			"二、合作方式",
			fmt.Sprintf("支持短租、长租与联合运营三种方式，最小合作单元为%d，价格按使用时长与用量分档协商，长期合作可给予一定优惠。", 1+n%9),
			"三、交付与结算",
			fmt.Sprintf("确认合作后%d天内完成交付，结算周期可选月结或季结，押金按一个月费用收取，合作结束验收无误后原路退还。", 3+n%5),
			"四、注意事项",
			"合作方需遵守场地或资源的使用规范，涉及资质要求的场景需提供有效证明，双方在协议中明确安全责任与违约处理方式。",
			"五、对接方式",
			fmt.Sprintf("请通过平台联系方式说明使用场景、时长与预算，资源编号 SEED-%03d，发布方将安排实地查看与报价。", idx+1),
		}, "\n\n")
		return title, summary, detail
	}
}
