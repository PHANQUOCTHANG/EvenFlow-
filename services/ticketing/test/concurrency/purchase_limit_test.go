//go:build integration

package concurrency

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// EV-130 -- BR-O4 (gioi han mua) tren Postgres va Redis THAT.
//
// Lop kiem nhanh trong CreateHold chay ngoai transaction, nen chi lop chot trong PersistHold moi
// bao dam duoc han muc duoi dong thoi. Cac test o day goi thang PersistHold de chung minh lop
// chot tu dung duoc, roi chay luong that voi thanh toan xen giua.

const maxPerIdentity = 4 // mac dinh cua events.max_per_identity (BR-O4)

// seedOrder ghi mot don co trang thai cho truoc, nhu luong thanh toan/hoan tien se de lai. Khong
// dong vao ton kho: test o day do han muc, khong do ton kho.
func (e *TestEnv) seedOrder(t *testing.T, eventID, ticketTypeID, identityID, status string, qty int) {
	t.Helper()
	ctx := context.Background()
	var orderID string
	if err := e.pool.QueryRow(ctx, `
		INSERT INTO orders (event_id, identity_id, status, total_cents, expires_at)
		VALUES ($1::uuid, $2::uuid, $3, 0, now() + interval '10 minutes')
		RETURNING id::text`, eventID, identityID, status).Scan(&orderID); err != nil {
		t.Fatalf("tao don %s: %v", status, err)
	}
	if _, err := e.pool.Exec(ctx, `
		INSERT INTO order_items (order_id, ticket_type_id, bucket, quantity, unit_cents)
		VALUES ($1::uuid, $2::uuid, 0, $3, 0)`, orderID, ticketTypeID, qty); err != nil {
		t.Fatalf("tao dong don %s: %v", status, err)
	}
}

// persistOne giu qty ve o bucket cho truoc. Moi truong hop dung bucket rieng: mot bucket chi co
// quota/32 ve, dung chung thi truong hop sau co the het ton kho vi truong hop truoc.
func (e *TestEnv) persistOne(eventID, ticketTypeID, identityID string, bucket, qty int) (port.HoldView, error) {
	return e.Repo.PersistHold(context.Background(), port.PersistHoldParams{
		EventID:        eventID,
		TicketTypeID:   ticketTypeID,
		IdentityID:     identityID,
		HoldID:         domain.NewID(),
		Bucket:         bucket,
		Quantity:       qty,
		UnitCents:      50_000_000,
		ExpiresAt:      time.Now().Add(10 * time.Minute),
		MaxPerIdentity: maxPerIdentity,
	})
}

// Lop chot: khach da co don chiem du 4 ve thi transaction tu choi -- KE CA khi lop kiem nhanh bi
// vuot qua (o day goi thang PersistHold). Don het han / huy / da hoan tien tra lai han muc.
// Bi tu choi thi khong tru ton kho, khong tao don.
func TestPurchaseLimit_LopChotTrongTransactionTheoTrangThaiDon(t *testing.T) {
	env := setupEnv(t)
	eventID, ticketTypeID := env.SeedEvent(t, quota)

	cases := []struct {
		status    string
		wantLimit bool
	}{
		{"HELD", true},
		{"PAYMENT_PENDING", true},
		{"PAID", true},
		{"ISSUED", true},
		{"REFUNDING", true},
		{"EXPIRED", false},
		{"CANCELLED", false},
		{"REFUNDED", false},
	}
	for i, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			identityID := env.SeedIdentity(t, 9000+i)
			env.seedOrder(t, eventID, ticketTypeID, identityID, tc.status, maxPerIdentity)
			before := env.TotalAvailable(t, ticketTypeID)

			_, err := env.persistOne(eventID, ticketTypeID, identityID, i, 1)

			if !tc.wantLimit {
				if err != nil {
					t.Fatalf("don %s tra lai han muc, phai giu duoc 1 ve; nhan %v", tc.status, err)
				}
				return
			}
			if !errors.Is(err, domain.ErrPerIdentityLimit) {
				t.Fatalf("da co don %s 4 ve: transaction phai tu choi bang ErrPerIdentityLimit, nhan %v",
					tc.status, err)
			}
			if after := env.TotalAvailable(t, ticketTypeID); after != before {
				t.Errorf("bi tu choi thi khong duoc tru ton kho: truoc %d, sau %d", before, after)
			}
			if n := env.queryInt(t, `SELECT count(*)::int FROM orders
				WHERE event_id = $1::uuid AND identity_id = $2::uuid`, eventID, identityID); n != 1 {
				t.Errorf("bi tu choi thi khong duoc tao don moi, nhan %d don", n)
			}
		})
	}

	// Dung bien: da chot 3 ve thi giu them 1 duoc, giu them 2 thi khong.
	t.Run("dung bien", func(t *testing.T) {
		identityID := env.SeedIdentity(t, 9100)
		env.seedOrder(t, eventID, ticketTypeID, identityID, "PAID", 3)
		if _, err := env.persistOne(eventID, ticketTypeID, identityID, len(cases), 2); !errors.Is(err, domain.ErrPerIdentityLimit) {
			t.Fatalf("3 + 2 > 4 phai bi tu choi, nhan %v", err)
		}
		if _, err := env.persistOne(eventID, ticketTypeID, identityID, len(cases), 1); err != nil {
			t.Fatalf("3 + 1 = 4 phai duoc giu, nhan %v", err)
		}
	})
}

// Noi goi quen truyen han muc (0) thi PersistHold tu choi thay vi bo qua lop chot.
func TestPurchaseLimit_ThieuHanMucThiTuChoi(t *testing.T) {
	env := setupEnv(t)
	eventID, ticketTypeID := env.SeedEvent(t, quota)
	before := env.TotalAvailable(t, ticketTypeID)

	_, err := env.Repo.PersistHold(context.Background(), port.PersistHoldParams{
		EventID: eventID, TicketTypeID: ticketTypeID, IdentityID: env.SeedIdentity(t, 9400),
		HoldID: domain.NewID(), Bucket: 0, Quantity: 1, UnitCents: 1,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	})
	if err == nil {
		t.Fatal("thieu max_per_identity phai bi tu choi, khong duoc bo qua BR-O4")
	}
	if after := env.TotalAvailable(t, ticketTypeID); after != before {
		t.Errorf("bi tu choi thi khong duoc tru ton kho: truoc %d, sau %d", before, after)
	}
}

// Lop nhanh chi dem don da chot; API han muc dem moi don chiem han muc va bao co hold dang giu.
func TestPurchaseLimit_DemDungTheoDinhNghiaBRO4(t *testing.T) {
	env := setupEnv(t)
	eventID, ticketTypeID := env.SeedEvent(t, quota)
	identityID := env.SeedIdentity(t, 9200)

	for _, o := range []struct {
		status string
		qty    int
	}{
		{"HELD", 1}, {"PAID", 2}, {"ISSUED", 1}, {"REFUNDING", 1},
		{"EXPIRED", 3}, {"CANCELLED", 2}, {"REFUNDED", 2},
	} {
		env.seedOrder(t, eventID, ticketTypeID, identityID, o.status, o.qty)
	}

	settled, err := env.repo.CountPurchased(context.Background(), eventID, identityID)
	if err != nil {
		t.Fatal(err)
	}
	if settled != 4 { // PAID 2 + ISSUED 1 + REFUNDING 1
		t.Errorf("CountPurchased = %d, muon 4 (chi don da chot)", settled)
	}

	l, err := app.NewGetPurchaseLimit(env.repo).Execute(context.Background(), eventID, identityID)
	if err != nil {
		t.Fatal(err)
	}
	if l.Used != 5 || !l.HasActiveHold || l.MaxSelectable != 0 || l.MaxPerOrder != 4 || l.MaxPerIdentity != 4 {
		t.Errorf("han muc = %+v, muon used=5 (them HELD 1), has_active_hold, max_selectable=0, 4/4", l)
	}

	// Khach moi: du han muc; su kien khong ton tai: ErrEventNotFound.
	fresh, err := app.NewGetPurchaseLimit(env.repo).Execute(context.Background(), eventID, env.SeedIdentity(t, 9201))
	if err != nil || fresh.Used != 0 || fresh.MaxSelectable != 4 || fresh.HasActiveHold {
		t.Errorf("khach moi: %+v, err %v; muon used=0, max_selectable=4", fresh, err)
	}
	if _, err := env.repo.GetPurchaseLimit(context.Background(), newUUID(t), identityID); !errors.Is(err, domain.ErrEventNotFound) {
		t.Errorf("su kien khong ton tai: mong doi ErrEventNotFound, nhan %v", err)
	}
	if _, err := env.repo.GetPurchaseLimit(context.Background(), "khong-phai-uuid", identityID); !errors.Is(err, domain.ErrEventNotFound) {
		t.Errorf("id sai dinh dang: mong doi ErrEventNotFound, nhan %v", err)
	}
}

// Kich ban TOCTOU that: mot khach bam mua 1 ve lien tuc tu nhieu tab trong khi don cua chinh ho
// duoc thanh toan xen giua (HELD -> PAYMENT_PENDING -> PAID, Redis Consume hold). Sau Consume,
// Redis khong con chan hold moi; neu chi co lop kiem nhanh ngoai transaction, mot request da dem
// truoc luc thanh toan se lot qua. Bat bien: tong ve chiem han muc khong bao gio vuot 4.
//
// Cua so that chi ~1 ms, ngan hon mot lan thanh toan, nen test noi rong no: lop kiem nhanh doc
// xong thi cho 20 ms (nhu Postgres cham luc cao diem). Khong noi thi test xanh ca khi bo lop chot.
func TestPurchaseLimit_ThanhToanXenGiuaKhongVuotHanMuc(t *testing.T) {
	env := setupEnv(t)
	eventID, ticketTypeID := env.SeedEvent(t, quota)
	identityID := env.SeedIdentity(t, 9300)
	token := env.AdmitToken(t, eventID, identityID)
	uc := app.NewCreateHold(env.Gate, slowCountRepo{env.Repo}, env.Logger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		wg       sync.WaitGroup
		paid     atomic.Int64
		limited  atomic.Int64
		holdRace atomic.Int64
		otherErr atomic.Int64
	)

	// Nguoi tra tien: thay don HELD nao thi tra tien ngay, nhu cong thanh toan bao thanh cong.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ctx.Err() == nil {
			if env.payActiveOrder(ctx, eventID, identityID) {
				paid.Add(1)
			}
		}
	}()

	// 8 tab cung bam mua 1 ve.
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				_, err := uc.Execute(ctx, app.CreateHoldInput{
					EventID: eventID, TicketTypeID: ticketTypeID,
					IdentityID: identityID, QueueToken: token, Quantity: 1,
				})
				switch {
				case err == nil, ctx.Err() != nil:
				case errors.Is(err, domain.ErrPerIdentityLimit):
					limited.Add(1)
				case errors.Is(err, domain.ErrOrderNotFound):
					// No cu cua BR-O3, co tu truoc EV-130: tab khac vua giu ghe o Redis nhung chua
					// commit Postgres, nen nhanh ALREADY_HOLDING khong tim thay hold. Khach thu lai la
					// duoc; khong anh huong han muc. Xem docs/workflow/EV-130-purchase-limit/spec.md.
					holdRace.Add(1)
				default:
					if otherErr.Add(1) <= 3 {
						t.Logf("loi khac: %v", err)
					}
				}
			}
		}()
	}
	wg.Wait()

	used := env.queryInt(t, `SELECT COALESCE(SUM(oi.quantity), 0)::int
		  FROM orders o JOIN order_items oi ON oi.order_id = o.id
		 WHERE o.event_id = $1::uuid AND o.identity_id = $2::uuid AND o.status = ANY($3::text[])`,
		eventID, identityID, domain.QuotaOrderStatuses)
	t.Logf("da tra tien %d don, %d lan bi chan vi han muc, %d lan dua hold BR-O3, %d loi khac, %d ve chiem han muc",
		paid.Load(), limited.Load(), holdRace.Load(), otherErr.Load(), used)

	if used > maxPerIdentity {
		t.Fatalf("BR-O4 BI VI PHAM: khach chiem %d ve, han muc %d", used, maxPerIdentity)
	}
	if otherErr.Load() > 0 {
		t.Fatalf("%d request loi ngoai du kien", otherErr.Load())
	}
	if paid.Load() < maxPerIdentity {
		t.Fatalf("test khong tao du ap luc: chi tra tien %d don, can >= %d", paid.Load(), maxPerIdentity)
	}
	if limited.Load() == 0 {
		t.Fatal("test khong cham toi han muc: khong lan nao bi chan")
	}
	// Toan ven ton kho: moi ve da tru deu nam trong mot hold chua tra.
	if total := env.TotalAvailable(t, ticketTypeID) + env.TotalHeldOrSold(t, ticketTypeID); total != quota {
		t.Fatalf("ton kho + dang giu/da ban = %d, ky vong %d", total, quota)
	}
}

// slowCountRepo noi rong cua so giua luc lop kiem nhanh dem va luc ghi.
type slowCountRepo struct{ port.Repository }

func (r slowCountRepo) CountPurchased(ctx context.Context, eventID, identityID string) (int, error) {
	n, err := r.Repository.CountPurchased(ctx, eventID, identityID)
	time.Sleep(20 * time.Millisecond)
	return n, err
}

// payActiveOrder tra tien don dang HELD cua khach (neu co): HELD -> PAYMENT_PENDING -> PAID, danh
// dau hold da consume, roi Consume hold o Redis nhu luong thanh toan that. Tra ve true neu da tra.
func (e *TestEnv) payActiveOrder(ctx context.Context, eventID, identityID string) bool {
	var orderID, holdID string
	err := e.pool.QueryRow(ctx, `
		SELECT o.id::text, h.id::text
		  FROM orders o JOIN seat_holds h ON h.order_id = o.id
		 WHERE o.event_id = $1::uuid AND o.identity_id = $2::uuid AND o.status = 'HELD'
		   AND h.released_at IS NULL AND h.consumed_at IS NULL`, eventID, identityID).Scan(&orderID, &holdID)
	if err != nil {
		return false
	}
	for _, q := range []string{
		`UPDATE orders SET status = 'PAYMENT_PENDING' WHERE id = $1::uuid AND status = 'HELD'`,
		`UPDATE orders SET status = 'PAID' WHERE id = $1::uuid AND status = 'PAYMENT_PENDING'`,
	} {
		if _, err := e.pool.Exec(ctx, q, orderID); err != nil {
			return false
		}
	}
	if _, err := e.pool.Exec(ctx, `UPDATE seat_holds SET consumed_at = now()
		WHERE id = $1::uuid AND released_at IS NULL`, holdID); err != nil {
		return false
	}
	return e.gate.Consume(ctx, eventID, strings.ReplaceAll(holdID, "-", "")) == nil
}
