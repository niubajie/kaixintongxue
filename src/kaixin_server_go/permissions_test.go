package main

import "testing"

func user(id int64, account, role string) *User {
	return &User{ID: id, Account: account, Role: role}
}

// 对应草案第 7 节验收用例的单元测试版本。
func TestCanManageUser(t *testing.T) {
	owner := user(1, "owner", "Owner")
	admin := user(2, "admin", "Admin")
	admin2 := user(3, "admin2", "Admin")
	member := user(4, "member", "Member")

	cases := []struct {
		name   string
		actor  *User
		target *User
		allow  bool
		reason string
	}{
		{"Owner 管 Admin", owner, admin, true, ""},
		{"Owner 管 Member", owner, member, true, ""},
		{"Admin 管 Member", admin, member, true, ""},
		{"Admin 管 Admin", admin, admin2, false, reasonAdminOnAdmin},
		{"Admin 管 Owner", admin, owner, false, reasonTargetIsOwner},
		{"Member 管别人", member, admin, false, reasonNotAdmin},
		{"Owner 管自己", owner, owner, false, reasonSelfTarget},
		{"Admin 管自己", admin, admin, false, reasonSelfTarget},
		{"未登录", nil, member, false, reasonNotLoggedIn},
		{"目标不存在", owner, nil, false, "target_not_found"},
	}

	for _, c := range cases {
		allowed, reason := canManageUser(c.actor, c.target)
		if allowed != c.allow {
			t.Errorf("%s：期望允许=%v，实际=%v（原因 %s）", c.name, c.allow, allowed, reason)
		}
		if !c.allow && reason != c.reason {
			t.Errorf("%s：期望原因 %q，实际 %q", c.name, c.reason, reason)
		}
	}
}

func TestCanAssignRole(t *testing.T) {
	owner := user(1, "owner", "Owner")
	admin := user(2, "admin", "Admin")
	member := user(4, "member", "Member")

	if ok, _ := canAssignRole(owner, "Admin"); !ok {
		t.Error("Owner 应能指派 Admin")
	}
	if ok, _ := canAssignRole(owner, "Owner"); !ok {
		t.Error("Owner 应能指派 Owner")
	}
	if ok, _ := canAssignRole(admin, "Admin"); ok {
		t.Error("Admin 不应能指派 Admin")
	}
	if ok, reason := canAssignRole(admin, "Member"); !ok {
		t.Errorf("Admin 应能把人降为 Member，原因 %s", reason)
	}
	if ok, _ := canAssignRole(member, "Member"); !ok {
		t.Error("Member 降为 Member 本身不违规（越权由 canManageUser 拦）")
	}
}

func TestVisibleUsers(t *testing.T) {
	all := []User{
		{ID: 1, Account: "owner", Role: "Owner"},
		{ID: 2, Account: "admin", Role: "Admin"},
		{ID: 4, Account: "member", Role: "Member"},
	}

	if got := visibleUsers(user(1, "owner", "Owner"), all); len(got) != 3 {
		t.Errorf("Owner 应看到 3 条，实际 %d", len(got))
	}
	if got := visibleUsers(user(2, "admin", "Admin"), all); len(got) != 3 {
		t.Errorf("Admin 应看到 3 条，实际 %d", len(got))
	}
	got := visibleUsers(user(4, "member", "Member"), all)
	if len(got) != 1 || got[0].ID != 4 {
		t.Errorf("Member 应只看到自己，实际 %d 条", len(got))
	}
}

func TestBackupAndAuditPermissions(t *testing.T) {
	owner := user(1, "owner", "Owner")
	admin := user(2, "admin", "Admin")
	member := user(4, "member", "Member")

	if !canTriggerBackup(owner) || !canTriggerBackup(admin) {
		t.Error("Owner 与 Admin 应能触发备份")
	}
	if canTriggerBackup(member) {
		t.Error("Member 不应能触发备份")
	}
	if !canRestoreBackup(owner) {
		t.Error("Owner 应能恢复")
	}
	if canRestoreBackup(admin) || canRestoreBackup(member) {
		t.Error("只有 Owner 能恢复")
	}
	if !canReadAudit(owner) || !canReadAudit(admin) {
		t.Error("Owner 与 Admin 应能看审计")
	}
	if canReadAudit(member) {
		t.Error("Member 不应能看审计")
	}
	if !canEditCatalog(owner) || !canEditCatalog(admin) {
		t.Error("Owner 与 Admin 应能改目录")
	}
	if canEditCatalog(member) {
		t.Error("Member 不应能改目录")
	}
}
