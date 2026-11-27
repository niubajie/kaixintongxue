package resource

import "testing"

func TestDedupKeyNormalization(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	a := DedupKey(secret, "factory_office", "440300", "深圳 宝安 厂房 出租！")
	b := DedupKey(secret, "factory_office", "440300", "深圳宝安厂房出租")
	if a != b {
		t.Fatalf("标点/空格归一化后去重键应一致: %s != %s", a, b)
	}
	c := DedupKey(secret, "factory_office", "440100", "深圳宝安厂房出租")
	if a == c {
		t.Fatal("不同地区的去重键不应相同")
	}
}

func TestSimhashNearDuplicate(t *testing.T) {
	// 仅标点/空白差异 → 归一化后完全一致，距离应为 0
	a := simhash("深圳宝安标准厂房出租 面积2000平米 可分割 带行车")
	b := simhash("深圳宝安标准厂房出租，面积2000平米，可分割，带行车。")
	if Hamming(a, b) != 0 {
		t.Fatalf("归一化后相同的文本距离应为 0，实际 %d", Hamming(a, b))
	}
	// 无关文本距离应明显大于阈值
	c := simhash("杭州西湖区写字楼出售 精装修 近地铁")
	if Hamming(a, c) <= 3 {
		t.Fatalf("无关文本不应被判定近似，实际距离 %d", Hamming(a, c))
	}
}

func TestQualityScore(t *testing.T) {
	fullSummary := "面积2000平米，可分割，带行车，适合制造业，租金面议，欢迎实地考察，交通便利，配套齐全，随时可看房。"
	fullDetail := "位于宝安区福永街道，独栋三层标准厂房，单层面积2000平米，层高6米，承重1吨，水电齐全，配备货梯与卸货平台；" +
		"周边高速出入口3公里，距机场15分钟车程，员工宿舍与食堂配套完善；" +
		"可整租可分租，最短租期三年，租金面议，欢迎制造业、仓储物流企业实地考察洽谈；" +
		"厂区24小时安保巡逻，监控全覆盖，货车可直达车间门口；" +
		"租赁可开增值税专用发票，支持企业注册地址挂靠；" +
		"物业管理费按建筑面积每月每平米3元，水电按市政标准收取，无其他隐性费用。"
	full := QualityScore("深圳宝安标准厂房出租信息", fullSummary, fullDetail, "440300", true, true)
	if full != 100 {
		t.Fatalf("完整信息质量分应为 100，实际 %d", full)
	}

	low := QualityScore("厂房", "", "", "", false, false)
	if low >= 50 {
		t.Fatalf("信息缺失时质量分应 < 50，实际 %d", low)
	}
}

func TestValidateCreate(t *testing.T) {
	base := &CreateRequest{
		CategoryCode: "factory_office",
		Title:        "深圳宝安标准厂房出租",
		RegionCode:   "440300",
		Contact:      ContactInput{ContactName: "张三", Phone: "13800138000"},
	}
	if err := validateCreate(base); err != nil {
		t.Fatalf("合法请求不应报错: %v", err)
	}

	bad := *base
	bad.Title = "厂房"
	if err := validateCreate(&bad); err == nil {
		t.Fatal("标题过短应报错")
	}

	noContact := *base
	noContact.Contact = ContactInput{ContactName: "张三"}
	if err := validateCreate(&noContact); err == nil {
		t.Fatal("无联系方式应报错")
	}
}
