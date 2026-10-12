package domain

// Permission đại diện cho một quyền cụ thể trong ma trận quyền của EventFlow (docs/01-nghiep-vu.md mục 3).
type Permission string

const (
	// Xem sự kiện đã duyệt (Buyer, Organizer, Moderator, Ops)
	PermEventViewApproved Permission = "event:view_approved"

	// Tạo/sửa sự kiện của mình (Chỉ Organizer)
	PermEventManageOwn Permission = "event:manage_own"

	// Duyệt / từ chối sự kiện (Chỉ Moderator)
	PermEventModerate Permission = "event:moderate"

	// Xếp hàng & mua vé (Chỉ Buyer - các role khác bị cấm để tránh xung đột lợi ích)
	PermTicketQueueAndBuy Permission = "ticket:queue_and_buy"

	// Chỉnh admit_rate (Chỉ Ops)
	PermOpsTuneAdmitRate Permission = "ops:tune_admit_rate"

	// Bật chế độ suy biến AI (Chỉ Ops)
	PermOpsToggleAIDegradation Permission = "ops:toggle_ai_degradation"

	// Xem báo cáo sau bán (Ops, Moderator, Organizer sự kiện của mình)
	PermReportViewSale Permission = "report:view_sale"

	// Gỡ chặn anti-bot (Moderator, Ops)
	PermAntiBotUnban Permission = "antibot:unban"
)

// rolePermissions định nghĩa ma trận ánh xạ vai trò sang danh sách các quyền được phép
var rolePermissions = map[Role]map[Permission]bool{
	RoleBuyer: {
		PermEventViewApproved: true,
		PermTicketQueueAndBuy: true,
	},
	RoleOrganizer: {
		PermEventViewApproved: true,
		PermEventManageOwn:    true,
		PermReportViewSale:   true, // Lưu ý: Cần thêm kiểm tra sở hữu (Ownership Tenancy)
	},
	RoleModerator: {
		PermEventViewApproved: true,
		PermEventModerate:    true,
		PermReportViewSale:   true,
		PermAntiBotUnban:     true,
	},
	RoleOps: {
		PermEventViewApproved:      true,
		PermOpsTuneAdmitRate:       true,
		PermOpsToggleAIDegradation: true,
		PermReportViewSale:        true,
		PermAntiBotUnban:          true,
	},
}

// HasPermission kiểm tra xem một vai trò có quyền thực hiện hành động tương ứng hay không
func HasPermission(role Role, perm Permission) bool {
	perms, exists := rolePermissions[role]
	if !exists {
		return false
	}
	return perms[perm]
}

// IsValidRole kiểm tra xem một role có hợp lệ trong hệ thống hay không
func IsValidRole(role Role) bool {
	switch role {
	case RoleBuyer, RoleOrganizer, RoleModerator, RoleOps:
		return true
	default:
		return false
	}
}

// AllRoles trả về danh sách tất cả các role có trong hệ thống
func AllRoles() []Role {
	return []Role{RoleBuyer, RoleOrganizer, RoleModerator, RoleOps}
}
