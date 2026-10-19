package authx

import (
	"net/http"
	"regexp"
	"strconv"
)

// Hop dong danh tinh giua gateway va cac service phia sau.
//
// Gateway la noi DUY NHAT xac minh access token. Sau khi xac minh, no XOA moi header danh
// tinh do client gui len roi gan lai tu token. Service phia sau chi doc cac header nay va
// KHONG duoc mo cong ra ngoai mang noi bo: ai goi thang service se gia danh duoc bat ky ai.
const (
	HeaderIdentityID   = "X-Identity-Id"
	HeaderIdentityRole = "X-Identity-Role"
	// HeaderIdentityVerified la "true" khi identity da xac thuc OTP/IdP (BR-Q3).
	HeaderIdentityVerified = "X-Identity-Verified"
	// HeaderDeviceID la thiet bi cua phien (claim "did"); vang khi phien chua gan thiet bi.
	HeaderDeviceID = "X-Device-Id"
)

// identityHeaders la moi header ma client co the dung de gia danh. Gom ca ten cu/bien the
// de mot service doc nham ten van khong nhan duoc gia tri do client dat.
var identityHeaders = []string{
	HeaderIdentityID,
	HeaderIdentityRole,
	HeaderIdentityVerified,
	HeaderDeviceID,
	"X-User-Id",
	"X-Role",
	"X-Organizer-Id",
}

// Principal la nguoi goi da duoc gateway xac thuc.
type Principal struct {
	ID   string // identity_id (UUID)
	Role string // buyer | organizer | moderator | ops
	// Verified: identity da xac thuc OTP (hoac IdP). Phong cho tu choi join khi false.
	Verified bool
	// DeviceID la UUID thiet bi cua phien, rong neu chua gan (EV-113).
	DeviceID string
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// validRoles giu dong bo voi CHECK cua cot identities.role.
var validRoles = map[string]bool{"buyer": true, "organizer": true, "moderator": true, "ops": true}

// Valid bao Principal co id dang UUID va role nam trong danh sach.
func (p Principal) Valid() bool {
	return uuidRe.MatchString(p.ID) && validRoles[p.Role]
}

// StripIdentityHeaders xoa moi header danh tinh do client gui. Gateway goi tren MOI request,
// ke ca route cong khai.
func StripIdentityHeaders(h http.Header) {
	for _, k := range identityHeaders {
		h.Del(k)
	}
}

// SetIdentityHeaders gan danh tinh da xac minh. Luon xoa truoc de khong con gia tri thu hai.
func SetIdentityHeaders(h http.Header, p Principal) {
	StripIdentityHeaders(h)
	h.Set(HeaderIdentityID, p.ID)
	h.Set(HeaderIdentityRole, p.Role)
	h.Set(HeaderIdentityVerified, strconv.FormatBool(p.Verified))
	if p.DeviceID != "" {
		h.Set(HeaderDeviceID, p.DeviceID)
	}
}

// PrincipalFromHeaders doc danh tinh gateway da gan. ok=false neu thieu hoac sai dinh dang.
func PrincipalFromHeaders(h http.Header) (Principal, bool) {
	p := Principal{
		ID:       h.Get(HeaderIdentityID),
		Role:     h.Get(HeaderIdentityRole),
		Verified: h.Get(HeaderIdentityVerified) == "true",
		DeviceID: h.Get(HeaderDeviceID),
	}
	if p.DeviceID != "" && !uuidRe.MatchString(p.DeviceID) {
		return Principal{}, false
	}
	if !p.Valid() {
		return Principal{}, false
	}
	return p, true
}
