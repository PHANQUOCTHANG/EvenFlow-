package domain_test

import (
	"testing"

	"github.com/eventflow/eventflow/services/identity/internal/domain"
)

// TestRBACMatrix_Coverage kiểm tra toàn diện 32 tổ hợp (4 roles x 8 permissions)
// khớp 100% với Ma trận quyền trong docs/01-nghiep-vu.md mục 3.
func TestRBACMatrix_Coverage(t *testing.T) {
	tests := []struct {
		name       string
		role       domain.Role
		permission domain.Permission
		expected   bool
	}{
		// --- 1. Buyer (Khách mua vé) ---
		{"Buyer xem sự kiện đã duyệt", domain.RoleBuyer, domain.PermEventViewApproved, true},
		{"Buyer tạo/sửa sự kiện", domain.RoleBuyer, domain.PermEventManageOwn, false},
		{"Buyer duyệt sự kiện", domain.RoleBuyer, domain.PermEventModerate, false},
		{"Buyer xếp hàng & mua vé", domain.RoleBuyer, domain.PermTicketQueueAndBuy, true},
		{"Buyer chỉnh admit_rate", domain.RoleBuyer, domain.PermOpsTuneAdmitRate, false},
		{"Buyer bật suy biến AI", domain.RoleBuyer, domain.PermOpsToggleAIDegradation, false},
		{"Buyer xem báo cáo sau bán", domain.RoleBuyer, domain.PermReportViewSale, false},
		{"Buyer gỡ chặn anti-bot", domain.RoleBuyer, domain.PermAntiBotUnban, false},

		// --- 2. Organizer (Ban tổ chức) ---
		{"Organizer xem sự kiện đã duyệt", domain.RoleOrganizer, domain.PermEventViewApproved, true},
		{"Organizer tạo/sửa sự kiện của mình", domain.RoleOrganizer, domain.PermEventManageOwn, true},
		{"Organizer duyệt sự kiện", domain.RoleOrganizer, domain.PermEventModerate, false},
		{"Organizer xếp hàng & mua vé (cấm xung đột lợi ích)", domain.RoleOrganizer, domain.PermTicketQueueAndBuy, false},
		{"Organizer chỉnh admit_rate", domain.RoleOrganizer, domain.PermOpsTuneAdmitRate, false},
		{"Organizer bật suy biến AI", domain.RoleOrganizer, domain.PermOpsToggleAIDegradation, false},
		{"Organizer xem báo cáo sau bán", domain.RoleOrganizer, domain.PermReportViewSale, true},
		{"Organizer gỡ chặn anti-bot", domain.RoleOrganizer, domain.PermAntiBotUnban, false},

		// --- 3. Moderator (Kiểm duyệt viên) ---
		{"Moderator xem sự kiện đã duyệt", domain.RoleModerator, domain.PermEventViewApproved, true},
		{"Moderator tạo/sửa sự kiện", domain.RoleModerator, domain.PermEventManageOwn, false},
		{"Moderator duyệt sự kiện", domain.RoleModerator, domain.PermEventModerate, true},
		{"Moderator xếp hàng & mua vé (cấm xung đột lợi ích)", domain.RoleModerator, domain.PermTicketQueueAndBuy, false},
		{"Moderator chỉnh admit_rate", domain.RoleModerator, domain.PermOpsTuneAdmitRate, false},
		{"Moderator bật suy biến AI", domain.RoleModerator, domain.PermOpsToggleAIDegradation, false},
		{"Moderator xem báo cáo sau bán", domain.RoleModerator, domain.PermReportViewSale, true},
		{"Moderator gỡ chặn anti-bot", domain.RoleModerator, domain.PermAntiBotUnban, true},

		// --- 4. Ops (Vận hành / SRE / Admin) ---
		{"Ops xem sự kiện đã duyệt", domain.RoleOps, domain.PermEventViewApproved, true},
		{"Ops tạo/sửa sự kiện", domain.RoleOps, domain.PermEventManageOwn, false},
		{"Ops duyệt sự kiện", domain.RoleOps, domain.PermEventModerate, false},
		{"Ops xếp hàng & mua vé (cấm xung đột lợi ích)", domain.RoleOps, domain.PermTicketQueueAndBuy, false},
		{"Ops chỉnh admit_rate", domain.RoleOps, domain.PermOpsTuneAdmitRate, true},
		{"Ops bật suy biến AI", domain.RoleOps, domain.PermOpsToggleAIDegradation, true},
		{"Ops xem báo cáo sau bán", domain.RoleOps, domain.PermReportViewSale, true},
		{"Ops gỡ chặn anti-bot", domain.RoleOps, domain.PermAntiBotUnban, true},

		// --- 5. Vai trò không hợp lệ / không xác định ---
		{"Role rỗng không có quyền", domain.Role(""), domain.PermEventViewApproved, false},
		{"Role lạ không có quyền", domain.Role("guest"), domain.PermTicketQueueAndBuy, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.HasPermission(tc.role, tc.permission)
			if got != tc.expected {
				t.Errorf("HasPermission(%q, %q) = %v; mong muốn %v", tc.role, tc.permission, got, tc.expected)
			}
		})
	}
}

func TestIsValidRole(t *testing.T) {
	validRoles := []domain.Role{
		domain.RoleBuyer,
		domain.RoleOrganizer,
		domain.RoleModerator,
		domain.RoleOps,
	}

	for _, r := range validRoles {
		if !domain.IsValidRole(r) {
			t.Errorf("Role %q mong muốn hợp lệ nhưng IsValidRole trả về false", r)
		}
	}

	invalidRoles := []domain.Role{
		domain.Role(""),
		domain.Role("admin"),
		domain.Role("superadmin"),
		domain.Role("user"),
	}

	for _, r := range invalidRoles {
		if domain.IsValidRole(r) {
			t.Errorf("Role %q mong muốn không hợp lệ nhưng IsValidRole trả về true", r)
		}
	}
}
