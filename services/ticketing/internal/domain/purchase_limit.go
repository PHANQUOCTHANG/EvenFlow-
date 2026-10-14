package domain

// BR-O4 -- gioi han mua. Hai tap trang thai don duoc dinh nghia o day, MOT cho, vi ca lop kiem
// nhanh, lop chot trong transaction va API han muc deu phai dung chung mot nghia.

// SettledOrderStatuses la cac don DA CHOT: tien da tra (hoac dang hoan) va ve khong con quay lai
// kho trong luong ban. Lop kiem nhanh chi dem nhung don nay, de khach dang giu ghe bam mua lai
// van di toi nhanh "tra lai hold dang co" cua BR-O3 thay vi bi bao vuot han muc.
var SettledOrderStatuses = []string{"PAID", "ISSUED", "REFUNDING"}

// QuotaOrderStatuses la moi don DANG CHIEM han muc: don da chot cong voi don dang hoat dong
// (HELD, PAYMENT_PENDING). Don EXPIRED, CANCELLED, REFUNDED tra lai han muc.
//
// Lop chot trong transaction dem tap nay. Moi don duoc PAID deu tung la don dang hoat dong, nen
// mot transaction dem tap nay luon thay don do o mot trong hai trang thai -- khong co thu tu xen
// ke nao lam no lot luoi (xem PersistHold).
var QuotaOrderStatuses = []string{"HELD", "PAYMENT_PENDING", "PAID", "ISSUED", "REFUNDING"}

// MaxSelectable la so ve toi da khach con chon duoc cho don tiep theo:
// min(max_per_order, max_per_identity - so ve dang chiem han muc), khong am.
func MaxSelectable(maxPerOrder, maxPerIdentity, used int) int {
	return max(0, min(maxPerOrder, maxPerIdentity-used))
}
