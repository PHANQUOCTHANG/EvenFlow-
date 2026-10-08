package app_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// Test o day viet tu nghiep vu (docs/01-nghiep-vu.md muc 2.3, BR-O1..BR-O7), khong
// tu cach code dang lam. Fake chi mo phong hop dong cua port, khong cham ha tang that.

const (
	suKien    = "ev-concert"
	hangVe    = "tt-vip"
	khachHang = "id-khach-1"
	veXepHang = "queue-token-1"
	giaVe     = int64(1_500_000)
	tenPhut   = 10 * time.Minute
)

// cauHinhMacDinh dung gia tri mac dinh cua nghiep vu: giu 10 phut (BR-O2),
// 4 ve/don va 4 ve/nguoi (BR-O4), 32 bucket ton kho.
func cauHinhMacDinh() port.SaleConfig {
	return port.SaleConfig{
		PriceCents:     giaVe,
		BucketCount:    32,
		HoldTTLSeconds: 600,
		MaxPerOrder:    4,
		MaxPerIdentity: 4,
	}
}

// ===================== Fake InventoryGate (Redis) =====================

type releaseCall struct {
	EventID string
	HoldID  string
}

type fakeGate struct {
	mu sync.Mutex

	admitted bool
	admitErr error

	// holdFn quyet dinh ket qua Hold; nil thi tra HoldOK o bucket 0.
	holdFn func(req port.HoldRequest) (port.HoldResult, error)

	releaseErr error

	admitCalls   []string
	holdCalls    []port.HoldRequest
	releaseCalls []releaseCall
	consumeCalls int
	events       *[]string // nhat ky thu tu thao tac dung chung voi fakeRepo
}

func (g *fakeGate) log(s string) {
	if g.events != nil {
		*g.events = append(*g.events, s)
	}
}

func (g *fakeGate) Hold(_ context.Context, req port.HoldRequest) (port.HoldResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.holdCalls = append(g.holdCalls, req)
	g.log("gate.Hold:" + req.HoldID)
	if g.holdFn != nil {
		return g.holdFn(req)
	}
	return port.HoldResult{Status: port.HoldOK, Bucket: 0}, nil
}

func (g *fakeGate) Release(_ context.Context, eventID, holdID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.releaseCalls = append(g.releaseCalls, releaseCall{EventID: eventID, HoldID: holdID})
	g.log("gate.Release:" + holdID)
	return g.releaseErr
}

func (g *fakeGate) Consume(context.Context, string, string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.consumeCalls++
	return nil
}

func (g *fakeGate) IsAdmitted(_ context.Context, _ string, queueToken string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.admitCalls = append(g.admitCalls, queueToken)
	return g.admitted, g.admitErr
}

func (g *fakeGate) ExpiredHolds(context.Context, string, int) ([]string, error) {
	return nil, nil
}

// ===================== Fake Repository (Postgres) =====================

type fakeRepo struct {
	mu sync.Mutex

	cfg    port.SaleConfig
	cfgErr error

	purchased int
	countErr  error

	active    port.HoldView
	activeErr error

	persistErr error

	configCalls  int
	countCalls   int
	activeCalls  int
	persistCalls []port.PersistHoldParams
	releaseCalls []string
	events       *[]string
}

func (r *fakeRepo) log(s string) {
	if r.events != nil {
		*r.events = append(*r.events, s)
	}
}

func (r *fakeRepo) GetSaleConfig(context.Context, string, string) (port.SaleConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.configCalls++
	return r.cfg, r.cfgErr
}

func (r *fakeRepo) CountPurchased(context.Context, string, string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.countCalls++
	return r.purchased, r.countErr
}

func (r *fakeRepo) GetActiveHold(context.Context, string, string) (port.HoldView, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activeCalls++
	return r.active, r.activeErr
}

// PersistHold mo phong mot DB tra lai dung nhung gi da ghi.
func (r *fakeRepo) PersistHold(_ context.Context, p port.PersistHoldParams) (port.HoldView, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.persistCalls = append(r.persistCalls, p)
	r.log("repo.PersistHold:" + p.HoldID)
	if r.persistErr != nil {
		return port.HoldView{}, r.persistErr
	}
	return port.HoldView{
		OrderID:   "order-" + p.HoldID,
		HoldID:    p.HoldID,
		Bucket:    p.Bucket,
		ExpiresAt: p.ExpiresAt,
	}, nil
}

func (r *fakeRepo) ReleaseHold(_ context.Context, holdID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.releaseCalls = append(r.releaseCalls, holdID)
	r.log("repo.ReleaseHold:" + holdID)
	return true, nil
}

// ===================== Tien ich =====================

func loggerBo() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func dauVaoHopLe(qty int) app.CreateHoldInput {
	return app.CreateHoldInput{
		EventID:      suKien,
		TicketTypeID: hangVe,
		IdentityID:   khachHang,
		QueueToken:   veXepHang,
		Quantity:     qty,
	}
}

func moiTruongHopLe() (*fakeGate, *fakeRepo) {
	return &fakeGate{admitted: true}, &fakeRepo{cfg: cauHinhMacDinh()}
}

// ===================== Luong thanh cong =====================

// Khach da duoc tha vao, so luong hop le, con ve: hold duoc chot o ca Redis va
// Postgres voi CUNG mot ma hold, dung gia, dung bucket Redis da tru.
func TestCreateHold_ThanhCong_ChotCungMotHoldORedisVaPostgres(t *testing.T) {
	gate, repo := moiTruongHopLe()
	gate.holdFn = func(port.HoldRequest) (port.HoldResult, error) {
		return port.HoldResult{Status: port.HoldOK, Bucket: 7}, nil
	}
	uc := app.NewCreateHold(gate, repo, loggerBo())

	out, err := uc.Execute(context.Background(), dauVaoHopLe(2))
	if err != nil {
		t.Fatalf("mong doi giu ghe thanh cong, nhan loi: %v", err)
	}

	if len(gate.admitCalls) != 1 || gate.admitCalls[0] != veXepHang {
		t.Fatalf("phai kiem tra admit bang queue token cua khach, nhan %v", gate.admitCalls)
	}
	if len(gate.holdCalls) != 1 {
		t.Fatalf("phai giu kho Redis dung 1 lan, nhan %d", len(gate.holdCalls))
	}
	if len(repo.persistCalls) != 1 {
		t.Fatalf("phai chot Postgres dung 1 lan, nhan %d", len(repo.persistCalls))
	}
	req, p := gate.holdCalls[0], repo.persistCalls[0]

	if req.HoldID == "" || req.HoldID != p.HoldID {
		t.Errorf("ma hold Redis (%q) va Postgres (%q) phai trung nhau", req.HoldID, p.HoldID)
	}
	if req.EventID != suKien || req.TicketTypeID != hangVe || req.IdentityID != khachHang || req.Quantity != 2 {
		t.Errorf("yeu cau giu kho Redis sai thong tin: %+v", req)
	}
	if p.EventID != suKien || p.TicketTypeID != hangVe || p.IdentityID != khachHang || p.Quantity != 2 {
		t.Errorf("ban ghi Postgres sai thong tin: %+v", p)
	}
	if p.Bucket != 7 {
		t.Errorf("Postgres phai tru dung bucket Redis da tru (7), nhan %d", p.Bucket)
	}
	if p.UnitCents != giaVe {
		t.Errorf("don gia phai lay tu cau hinh ban (%d), nhan %d", giaVe, p.UnitCents)
	}
	if req.BucketCount != 32 {
		t.Errorf("so bucket phai lay tu cau hinh (32), nhan %d", req.BucketCount)
	}
	if out.HoldID != p.HoldID || out.OrderID == "" {
		t.Errorf("ket qua tra client phai la hold vua chot, nhan %+v", out)
	}
	if len(gate.releaseCalls) != 0 {
		t.Errorf("thanh cong thi KHONG duoc tra kho, nhan %v", gate.releaseCalls)
	}
	if gate.consumeCalls != 0 {
		t.Errorf("giu ghe khong duoc consume hold (chi dung khi da thanh toan)")
	}
}

// BR-O2: giu ghe dung 10:00 tinh tu luc hold thanh cong; expires_at do SERVER
// tinh (client khong tu tinh) va duoc tra ve cho client.
func TestCreateHold_BRO2_HetHanDungMuoiPhutTheoDongHoServer(t *testing.T) {
	gate, repo := moiTruongHopLe()
	uc := app.NewCreateHold(gate, repo, loggerBo())

	truoc := time.Now()
	out, err := uc.Execute(context.Background(), dauVaoHopLe(1))
	sau := time.Now()
	if err != nil {
		t.Fatalf("mong doi thanh cong, nhan loi: %v", err)
	}

	if got := gate.holdCalls[0].TTL; got != tenPhut {
		t.Errorf("TTL giu kho Redis phai dung 10 phut, nhan %v", got)
	}
	exp := repo.persistCalls[0].ExpiresAt
	if exp.Before(truoc.Add(tenPhut)) || exp.After(sau.Add(tenPhut)) {
		t.Errorf("expires_at phai = thoi diem server + 10 phut, trong [%v, %v], nhan %v",
			truoc.Add(tenPhut), sau.Add(tenPhut), exp)
	}
	if !out.ExpiresAt.Equal(exp) {
		t.Errorf("client phai nhan dung expires_at da chot (%v), nhan %v", exp, out.ExpiresAt)
	}
}

// Moi lan giu ghe thanh cong la mot hold rieng: ma hold khong bao gio lap lai,
// neu khong viec tra kho theo ma hold (BR-O7) se tra nham hold cua nguoi khac.
func TestCreateHold_MoiHoldCoMaRiengKhongTrung(t *testing.T) {
	gate, repo := moiTruongHopLe()
	uc := app.NewCreateHold(gate, repo, loggerBo())

	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		out, err := uc.Execute(context.Background(), dauVaoHopLe(1))
		if err != nil {
			t.Fatalf("lan %d: %v", i, err)
		}
		if seen[out.HoldID] {
			t.Fatalf("ma hold bi lap lai: %s", out.HoldID)
		}
		seen[out.HoldID] = true
	}
}

// Bucket bat dau phai nam trong [0, bucket_count): neu lech ra ngoai, Redis se
// doc mot dong ton kho khong ton tai va bao het ve sai.
func TestCreateHold_BucketBatDauLuonNamTrongSoBucket(t *testing.T) {
	cases := []struct {
		name        string
		bucketCount int
	}{
		{"mot bucket duy nhat", 1},
		{"hai bucket", 2},
		{"cau hinh mac dinh 32 bucket", 32},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gate, repo := moiTruongHopLe()
			repo.cfg.BucketCount = tc.bucketCount
			uc := app.NewCreateHold(gate, repo, loggerBo())

			for i := 0; i < 200; i++ {
				if _, err := uc.Execute(context.Background(), dauVaoHopLe(1)); err != nil {
					t.Fatalf("lan %d: %v", i, err)
				}
			}
			for _, req := range gate.holdCalls {
				if req.StartBucket < 0 || req.StartBucket >= tc.bucketCount {
					t.Fatalf("start bucket %d nam ngoai [0,%d)", req.StartBucket, tc.bucketCount)
				}
				if req.BucketCount != tc.bucketCount {
					t.Fatalf("bucket count phai = %d, nhan %d", tc.bucketCount, req.BucketCount)
				}
			}
		})
	}
}

// ===================== BR-O4: gioi han mua =====================

// Truong hop nam dung bien gioi han van phai duoc mua.
func TestCreateHold_BRO4_DungBienGioiHanVanDuocMua(t *testing.T) {
	cases := []struct {
		name      string
		maxOrder  int
		maxIdent  int
		purchased int
		qty       int
	}{
		{"mua 1 ve", 4, 4, 0, 1},
		{"mua dung max_per_order", 4, 4, 0, 4},
		{"da mua 2 mua them 2 = max_per_identity", 4, 4, 2, 2},
		{"da mua 3 mua them 1 = max_per_identity", 4, 4, 3, 1},
		{"max_per_order nho hon max_per_identity", 2, 6, 0, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gate, repo := moiTruongHopLe()
			repo.cfg.MaxPerOrder = tc.maxOrder
			repo.cfg.MaxPerIdentity = tc.maxIdent
			repo.purchased = tc.purchased
			uc := app.NewCreateHold(gate, repo, loggerBo())

			if _, err := uc.Execute(context.Background(), dauVaoHopLe(tc.qty)); err != nil {
				t.Fatalf("dung bien gioi han phai duoc mua, nhan loi: %v", err)
			}
			if len(repo.persistCalls) != 1 {
				t.Fatalf("phai chot dung 1 hold, nhan %d", len(repo.persistCalls))
			}
		})
	}
}

// ===================== Cac truong hop bi tu choi =====================

// Moi truong hop bi tu choi deu KHONG duoc dong vao ton kho: khong giu Redis,
// khong ghi Postgres, khong co gi phai tra lai.
func TestCreateHold_TuChoiTruocKhiDongVaoTonKho(t *testing.T) {
	loiHaTang := errors.New("mat ket noi")

	cases := []struct {
		name    string
		setup   func(g *fakeGate, r *fakeRepo)
		qty     int
		wantErr error
		// chamDB=false: khach chua duoc admit thi khong duoc cham Postgres (BR-Q7).
		chamDB bool
	}{
		{
			name:    "BR-Q7 khach chua duoc phong cho tha vao",
			setup:   func(g *fakeGate, _ *fakeRepo) { g.admitted = false },
			qty:     1,
			wantErr: domain.ErrNotAdmitted,
			chamDB:  false,
		},
		{
			name:    "khong kiem tra duoc admit thi khong ban",
			setup:   func(g *fakeGate, _ *fakeRepo) { g.admitted, g.admitErr = false, loiHaTang },
			qty:     1,
			wantErr: loiHaTang,
			chamDB:  false,
		},
		{
			name:    "khong doc duoc cau hinh ban",
			setup:   func(_ *fakeGate, r *fakeRepo) { r.cfgErr = loiHaTang },
			qty:     1,
			wantErr: loiHaTang,
			chamDB:  true,
		},
		{
			name:    "BR-O4 so luong bang 0",
			qty:     0,
			wantErr: domain.ErrQuantityNotAllowed,
			chamDB:  true,
		},
		{
			name:    "BR-O4 so luong am",
			qty:     -1,
			wantErr: domain.ErrQuantityNotAllowed,
			chamDB:  true,
		},
		{
			name:    "BR-O4 vuot max_per_order",
			qty:     5,
			wantErr: domain.ErrQuantityNotAllowed,
			chamDB:  true,
		},
		{
			name:    "BR-O4 vuot max_per_order du con han muc ca nhan",
			setup:   func(_ *fakeGate, r *fakeRepo) { r.cfg.MaxPerOrder, r.cfg.MaxPerIdentity = 2, 10 },
			qty:     3,
			wantErr: domain.ErrQuantityNotAllowed,
			chamDB:  true,
		},
		{
			name:    "BR-O4 da mua 3 mua them 2 vuot max_per_identity",
			setup:   func(_ *fakeGate, r *fakeRepo) { r.purchased = 3 },
			qty:     2,
			wantErr: domain.ErrPerIdentityLimit,
			chamDB:  true,
		},
		{
			name:    "BR-O4 da mua du 4 ve mua them 1",
			setup:   func(_ *fakeGate, r *fakeRepo) { r.purchased = 4 },
			qty:     1,
			wantErr: domain.ErrPerIdentityLimit,
			chamDB:  true,
		},
		{
			name:    "BR-O4 don hop le nhung gop lai vuot han muc ca nhan",
			setup:   func(_ *fakeGate, r *fakeRepo) { r.cfg.MaxPerOrder, r.cfg.MaxPerIdentity, r.purchased = 6, 4, 0 },
			qty:     5,
			wantErr: domain.ErrPerIdentityLimit,
			chamDB:  true,
		},
		{
			name:    "khong dem duoc so ve da mua thi khong ban",
			setup:   func(_ *fakeGate, r *fakeRepo) { r.countErr = loiHaTang },
			qty:     1,
			wantErr: loiHaTang,
			chamDB:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gate, repo := moiTruongHopLe()
			if tc.setup != nil {
				tc.setup(gate, repo)
			}
			uc := app.NewCreateHold(gate, repo, loggerBo())

			out, err := uc.Execute(context.Background(), dauVaoHopLe(tc.qty))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("mong doi loi %v, nhan %v", tc.wantErr, err)
			}
			if out != (app.CreateHoldOutput{}) {
				t.Errorf("bi tu choi thi khong duoc tra hold nao, nhan %+v", out)
			}
			if len(gate.holdCalls) != 0 {
				t.Errorf("bi tu choi thi KHONG duoc giu kho Redis, nhan %d lan", len(gate.holdCalls))
			}
			if len(repo.persistCalls) != 0 {
				t.Errorf("bi tu choi thi KHONG duoc ghi Postgres, nhan %d lan", len(repo.persistCalls))
			}
			if len(gate.releaseCalls) != 0 {
				t.Errorf("chua giu gi thi khong co gi de tra, nhan %v", gate.releaseCalls)
			}
			if !tc.chamDB && (repo.configCalls+repo.countCalls) != 0 {
				t.Errorf("khach chua duoc admit thi khong duoc cham Postgres, nhan %d truy van",
					repo.configCalls+repo.countCalls)
			}
		})
	}
}

// ===================== BR-O1: cong Redis =====================

// Redis bao het ve hoac tra trang thai bat thuong: khong duoc tao hold o Postgres.
func TestCreateHold_BRO1_CongRedisTuChoiThiKhongTaoHold(t *testing.T) {
	loiRedis := errors.New("redis timeout")

	cases := []struct {
		name    string
		res     port.HoldResult
		err     error
		wantErr error // nil = chi can co loi
	}{
		{"het ve", port.HoldResult{Status: port.HoldSoldOut}, nil, domain.ErrSoldOut},
		{"so luong khong hop le theo Redis", port.HoldResult{Status: port.HoldBadQty}, nil, nil},
		{"trang thai la", port.HoldResult{Status: port.HoldStatus("WAT")}, nil, nil},
		{"Redis loi", port.HoldResult{}, loiRedis, loiRedis},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gate, repo := moiTruongHopLe()
			gate.holdFn = func(port.HoldRequest) (port.HoldResult, error) { return tc.res, tc.err }
			uc := app.NewCreateHold(gate, repo, loggerBo())

			out, err := uc.Execute(context.Background(), dauVaoHopLe(1))
			if err == nil {
				t.Fatalf("mong doi loi, nhan hold %+v", out)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("mong doi loi %v, nhan %v", tc.wantErr, err)
			}
			if len(repo.persistCalls) != 0 {
				t.Errorf("Redis khong giu duoc thi KHONG duoc ghi Postgres, nhan %d lan", len(repo.persistCalls))
			}
			if out != (app.CreateHoldOutput{}) {
				t.Errorf("khong duoc tra hold nao, nhan %+v", out)
			}
		})
	}
}

// ===================== BR-O1 + BR-O7: den bu khi Postgres tu choi =====================

// Redis da tru kho nhung Postgres tu choi: phai tra lai kho Redis DUNG MOT LAN,
// DUNG hold vua giu; neu khong ve se bi khoa ma khong ai mua duoc. Neu Postgres
// noi het ve thi tin Postgres va bao het ve (BR-O1: Postgres la nguon su that).
func TestCreateHold_PostgresTuChoi_PhaiTraKhoRedisDungMotLan(t *testing.T) {
	loiDB := errors.New("postgres: connection reset")
	loiRelease := errors.New("redis: release that bai")

	cases := []struct {
		name       string
		persistErr error
		releaseErr error
		wantErr    error
		wantErrLog bool
	}{
		{
			name:       "Postgres het ton kho thi bao het ve",
			persistErr: domain.ErrInventoryExhausted,
			wantErr:    domain.ErrSoldOut,
		},
		{
			name:       "Postgres het ton kho (loi bi boc) van bao het ve",
			persistErr: fmt.Errorf("persist hold: %w", domain.ErrInventoryExhausted),
			wantErr:    domain.ErrSoldOut,
		},
		{
			name:       "Postgres loi khac thi tra dung loi do",
			persistErr: loiDB,
			wantErr:    loiDB,
		},
		{
			name:       "tra kho Redis that bai van bao loi goc va ghi log ERROR",
			persistErr: loiDB,
			releaseErr: loiRelease,
			wantErr:    loiDB,
			wantErrLog: true,
		},
		{
			name:       "het ton kho va tra kho that bai van bao het ve",
			persistErr: domain.ErrInventoryExhausted,
			releaseErr: loiRelease,
			wantErr:    domain.ErrSoldOut,
			wantErrLog: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gate, repo := moiTruongHopLe()
			repo.persistErr = tc.persistErr
			gate.releaseErr = tc.releaseErr
			var logBuf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logBuf, nil))
			uc := app.NewCreateHold(gate, repo, logger)

			out, err := uc.Execute(context.Background(), dauVaoHopLe(2))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("mong doi loi %v, nhan %v", tc.wantErr, err)
			}
			if out != (app.CreateHoldOutput{}) {
				t.Errorf("Postgres tu choi thi khong duoc tra hold, nhan %+v", out)
			}
			if len(gate.releaseCalls) != 1 {
				t.Fatalf("phai tra kho Redis dung 1 lan (BR-O7), nhan %d", len(gate.releaseCalls))
			}
			rc := gate.releaseCalls[0]
			if rc.EventID != suKien || rc.HoldID != gate.holdCalls[0].HoldID {
				t.Errorf("phai tra dung hold vua giu (%s/%s), nhan %+v",
					suKien, gate.holdCalls[0].HoldID, rc)
			}
			if tc.wantErrLog {
				s := logBuf.String()
				if !strings.Contains(s, "level=ERROR") || !strings.Contains(s, rc.HoldID) {
					t.Errorf("tra kho that bai phai ghi log ERROR kem ma hold de doi soat, log: %s", s)
				}
			}
		})
	}
}

// BR-O3 (bat bien yeu nhat, dung voi moi cach hien thuc): khach dang co hold con
// han bam mua lai thi KHONG BAO GIO duoc co hai hold cung hoat dong. Neu he thong
// tao hold moi thi hold cu phai duoc tra truoc do.
func TestCreateHold_BRO3_KhongBaoGioCoHaiHoldCungHoatDong(t *testing.T) {
	const holdCu = "hold-cu"
	var events []string
	gate := &fakeGate{admitted: true, events: &events}
	repo := &fakeRepo{
		cfg:    cauHinhMacDinh(),
		active: port.HoldView{OrderID: "order-cu", HoldID: holdCu, ExpiresAt: time.Now().Add(5 * time.Minute)},
		events: &events,
	}
	lan := 0
	gate.holdFn = func(port.HoldRequest) (port.HoldResult, error) {
		lan++
		if lan == 1 {
			return port.HoldResult{Status: port.HoldAlreadyHolding, ExistingHold: holdCu}, nil
		}
		return port.HoldResult{Status: port.HoldOK}, nil
	}
	uc := app.NewCreateHold(gate, repo, loggerBo())

	out, err := uc.Execute(context.Background(), dauVaoHopLe(1))
	if err != nil {
		t.Fatalf("khach bam mua lai khong nen bi bao loi, nhan %v", err)
	}
	if out.HoldID == "" {
		t.Fatalf("khach phai nhan ve mot hold dang hoat dong, nhan %+v", out)
	}

	// Moi PersistHold cua hold MOI phai co mot lan tra hold cu dung truoc no.
	daTraHoldCu := false
	for _, e := range events {
		if e == "gate.Release:"+holdCu || e == "repo.ReleaseHold:"+holdCu {
			daTraHoldCu = true
		}
		if strings.HasPrefix(e, "repo.PersistHold:") && !daTraHoldCu {
			t.Fatalf("tao hold moi khi hold cu %s chua duoc tra -> hai hold cung hoat dong; thu tu: %v",
				holdCu, events)
		}
	}
}
