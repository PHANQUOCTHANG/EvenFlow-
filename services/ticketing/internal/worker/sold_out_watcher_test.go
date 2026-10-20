package worker_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/eventflow/eventflow/services/ticketing/internal/port"
	"github.com/eventflow/eventflow/services/ticketing/internal/worker"
)

type fakeTiers struct {
	ids   []string
	err   error
	calls int
}

func (f *fakeTiers) TicketTypeIDs(context.Context, string) ([]string, error) {
	f.calls++
	return f.ids, f.err
}

type fakeStock struct {
	info  port.StockInfo
	err   error
	calls int
}

func (f *fakeStock) Stock(context.Context, string, []string) (port.StockInfo, error) {
	f.calls++
	return f.info, f.err
}

type fakeSink struct {
	writes []port.SaleState
	err    error
}

func (f *fakeSink) SetSaleState(_ context.Context, _ string, s port.SaleState) error {
	if f.err != nil {
		return f.err
	}
	f.writes = append(f.writes, s)
	return nil
}

type watchFixture struct {
	w     *worker.SoldOutWatcher
	tiers *fakeTiers
	stock *fakeStock
	sink  *fakeSink
}

func newWatchFixture() *watchFixture {
	f := &watchFixture{
		tiers: &fakeTiers{ids: []string{"tt1", "tt2"}},
		stock: &fakeStock{info: port.StockInfo{Total: 100, Seeded: true}},
		sink:  &fakeSink{},
	}
	f.w = worker.NewSoldOutWatcher(f.tiers, f.stock, f.sink, worker.DefaultSoldOutWatcherConfig([]string{"e1"}),
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	return f
}

func (f *watchFixture) tick(t *testing.T, n int) {
	t.Helper()
	for range n {
		if err := f.w.Check(context.Background(), "e1"); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *watchFixture) last() port.SaleState { return f.sink.writes[len(f.sink.writes)-1] }

func TestWatcher_ConVeThiChiGhiMotLanDeXoaCoCu(t *testing.T) {
	f := newWatchFixture()
	f.tick(t, 5)
	if len(f.sink.writes) != 1 || f.last() != (port.SaleState{}) {
		t.Fatalf("muon dung 1 lan ghi trang thai rong (xoa co cu sau khi khoi dong lai), nhan %+v", f.sink.writes)
	}
}

func TestWatcher_ChuaNapKhoThiKhongKetLuanHetVe(t *testing.T) {
	f := newWatchFixture()
	f.stock.info = port.StockInfo{Total: 0, Seeded: false}
	f.tick(t, 10)
	if len(f.sink.writes) != 0 {
		t.Fatalf("chua nap kho ma da ghi co: %+v", f.sink.writes)
	}
}

func TestWatcher_Kho0ConHoldChiNgungThaNguoi(t *testing.T) {
	f := newWatchFixture()
	f.stock.info = port.StockInfo{Total: 0, Seeded: true, ActiveHolds: 2}
	f.tick(t, 20)
	if f.last() != (port.SaleState{StockZero: true}) {
		t.Fatalf("trang thai = %+v, muon chi StockZero khi con hold", f.last())
	}
}

func TestWatcher_HetHanChiSauNhieuLanLienTiepKhongHold(t *testing.T) {
	f := newWatchFixture()
	f.stock.info = port.StockInfo{Total: 0, Seeded: true}

	f.tick(t, 2)
	if f.last().SoldOut {
		t.Fatal("moi 2 lan quan sat da bao het han")
	}
	f.tick(t, 1)
	if got := f.last(); !got.SoldOut || !got.StockZero {
		t.Fatalf("sau 3 lan lien tiep phai SoldOut+StockZero, nhan %+v", got)
	}
	n := len(f.sink.writes)
	f.tick(t, 5)
	if len(f.sink.writes) != n {
		t.Fatal("trang thai khong doi nhung van ghi lai")
	}
}

func TestWatcher_HoldXuatHienLamLaiBoDem(t *testing.T) {
	f := newWatchFixture()
	f.stock.info = port.StockInfo{Total: 0, Seeded: true}
	f.tick(t, 2)
	f.stock.info.ActiveHolds = 1 // khach khac vua giu duoc ve cuoi
	f.tick(t, 1)
	f.stock.info.ActiveHolds = 0
	f.tick(t, 2)
	if f.last().SoldOut {
		t.Fatal("bo dem phai bat dau lai sau khi co hold")
	}
	f.tick(t, 1)
	if !f.last().SoldOut {
		t.Fatal("du 3 lan lien tiep moi lai phai SoldOut")
	}
}

func TestWatcher_KhoCoLaiGoCoNgay(t *testing.T) {
	f := newWatchFixture()
	f.stock.info = port.StockInfo{Total: 0, Seeded: true}
	f.tick(t, 3)
	if !f.last().SoldOut {
		t.Fatal("tien quyet: phai SoldOut")
	}

	f.stock.info = port.StockInfo{Total: 2, Seeded: true} // hoan tien / hold het han tra ve
	f.tick(t, 1)
	if f.last() != (port.SaleState{}) {
		t.Fatalf("kho co lai phai go het co ngay, nhan %+v", f.last())
	}
}

func TestWatcher_LoiGhiDuocThuLaiVongSau(t *testing.T) {
	f := newWatchFixture()
	f.stock.info = port.StockInfo{Total: 0, Seeded: true, ActiveHolds: 1}
	f.sink.err = errors.New("redis down")
	if err := f.w.Check(context.Background(), "e1"); err == nil {
		t.Fatal("loi ghi phai duoc tra ra")
	}
	f.sink.err = nil
	f.tick(t, 1)
	if len(f.sink.writes) != 1 || !f.last().StockZero {
		t.Fatalf("vong sau phai ghi lai: %+v", f.sink.writes)
	}
}

func TestWatcher_LoiDocKhoKhongGhiCo(t *testing.T) {
	f := newWatchFixture()
	f.stock.err = errors.New("redis down")
	if err := f.w.Check(context.Background(), "e1"); err == nil {
		t.Fatal("loi doc kho phai duoc tra ra")
	}
	if len(f.sink.writes) != 0 {
		t.Fatal("khong doc duoc kho thi khong duoc ghi co")
	}
}

func TestWatcher_SuKienKhongCoHangVeThiBoQua(t *testing.T) {
	f := newWatchFixture()
	f.tiers.ids = nil
	f.tick(t, 3)
	if f.stock.calls != 0 || len(f.sink.writes) != 0 {
		t.Fatalf("stock=%d writes=%d", f.stock.calls, len(f.sink.writes))
	}
}

func TestWatcher_DanhSachHangVeDuocGiuTrongKhoangLamMoi(t *testing.T) {
	f := newWatchFixture()
	f.tick(t, 5)
	if f.tiers.calls != 1 {
		t.Fatalf("hoi hang ve %d lan trong khoang lam moi, muon 1", f.tiers.calls)
	}
	f.tiers.err = errors.New("pg down")
	f.tick(t, 3) // van trong khoang giu: loi PG khong anh huong
}

func TestWatcher_CheckAllKhongDungLaiKhiMotSuKienLoi(t *testing.T) {
	f := newWatchFixture()
	cfg := worker.DefaultSoldOutWatcherConfig([]string{"bad", "e1"})
	w := worker.NewSoldOutWatcher(&selectiveTiers{bad: "bad", ok: []string{"tt1"}}, f.stock, f.sink, cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	w.CheckAll(context.Background())
	if len(f.sink.writes) != 1 {
		t.Fatalf("su kien tot van phai duoc xu ly, writes=%d", len(f.sink.writes))
	}
}

type selectiveTiers struct {
	bad string
	ok  []string
}

func (s *selectiveTiers) TicketTypeIDs(_ context.Context, eventID string) ([]string, error) {
	if eventID == s.bad {
		return nil, errors.New("loi")
	}
	return s.ok, nil
}

func TestTierSource_LayHangVeKhongTrung(t *testing.T) {
	src := stubInventory{rows: []port.InventoryRow{
		{TicketTypeID: "a", Bucket: 0}, {TicketTypeID: "a", Bucket: 1}, {TicketTypeID: "b", Bucket: 0},
	}}
	ids, err := worker.NewTierSource(src).TicketTypeIDs(context.Background(), "e1")
	if err != nil || len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
}

type stubInventory struct{ rows []port.InventoryRow }

func (s stubInventory) ListInventory(context.Context, string) ([]port.InventoryRow, error) {
	return s.rows, nil
}
