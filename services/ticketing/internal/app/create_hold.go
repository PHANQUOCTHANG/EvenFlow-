package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// CreateHold tao mot lenh giu ghe.
//
// Day la use case nong nhat va cung la noi bat bien quan trong nhat cua he thong
// duoc bao ve: KHONG BAO GIO BAN QUA SO VE (BR-O1).
//
// Trinh tu hai lop, co chu y:
//
//	1. Redis (nhanh, khong ben)  -- chan 95% request thua truoc khi chung cham DB.
//	2. Postgres (cham, ben vung) -- NGUON SU THAT. Constraint CHECK (available >= 0)
//	   la chot chan cuoi cung; du tang tren sai het, DB van tu choi.
//
// Neu lop 2 that bai sau khi lop 1 da thanh cong, phai tra lai kho cho Redis --
// neu khong, ve se "bien mat" (bi giu vinh vien ma khong ai mua duoc).
type CreateHold struct {
	gate  port.InventoryGate // Redis
	repo  port.Repository    // Postgres
	log   *slog.Logger
	rng   *rand.Rand
}

func NewCreateHold(gate port.InventoryGate, repo port.Repository, log *slog.Logger) *CreateHold {
	return &CreateHold{
		gate: gate,
		repo: repo,
		log:  log,
		rng:  rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

type CreateHoldInput struct {
	EventID      string
	TicketTypeID string
	IdentityID   string
	QueueToken   string
	Quantity     int
}

// CreateHoldOutput la ket qua tra ve cho client.
// ExpiresAt la NGUON SU THAT cua dong ho dem nguoc 10:00 phia frontend (BR-O2):
// client khong duoc tu tinh, de doi gio may khach khong keo dai duoc thoi gian giu.
type CreateHoldOutput = port.HoldView

func (uc *CreateHold) Execute(ctx context.Context, in CreateHoldInput) (CreateHoldOutput, error) {
	// 0. Chi nguoi da duoc phong cho tha vao moi duoc mua. Day la cach tai xuong
	//    Postgres bi ghim o muc hang so du co bao nhieu nguoi dang xep hang.
	admitted, err := uc.gate.IsAdmitted(ctx, in.EventID, in.QueueToken)
	if err != nil {
		return CreateHoldOutput{}, fmt.Errorf("kiem tra admit: %w", err)
	}
	if !admitted {
		return CreateHoldOutput{}, domain.ErrNotAdmitted
	}

	ev, err := uc.repo.GetSaleConfig(ctx, in.EventID, in.TicketTypeID)
	if err != nil {
		return CreateHoldOutput{}, err
	}

	// 1. Kiem gioi han mua (BR-O4) truoc khi dong vao ton kho.
	if in.Quantity <= 0 || in.Quantity > ev.MaxPerOrder {
		return CreateHoldOutput{}, domain.ErrQuantityNotAllowed
	}
	bought, err := uc.repo.CountPurchased(ctx, in.EventID, in.IdentityID)
	if err != nil {
		return CreateHoldOutput{}, err
	}
	if bought+in.Quantity > ev.MaxPerIdentity {
		return CreateHoldOutput{}, domain.ErrPerIdentityLimit
	}

	// 2. Cong chan Redis. Bucket bat dau ngau nhien de trai deu contention tren
	//    32 dong ton kho thay vi dam ca vao mot dong.
	holdID := domain.NewID()
	startBucket := uc.rng.Intn(ev.BucketCount)
	ttl := time.Duration(ev.HoldTTLSeconds) * time.Second

	res, err := uc.gate.Hold(ctx, port.HoldRequest{
		EventID:      in.EventID,
		TicketTypeID: in.TicketTypeID,
		IdentityID:   in.IdentityID,
		HoldID:       holdID,
		Quantity:     in.Quantity,
		TTL:          ttl,
		BucketCount:  ev.BucketCount,
		StartBucket:  startBucket,
	})
	if err != nil {
		return CreateHoldOutput{}, fmt.Errorf("redis gate: %w", err)
	}
	switch res.Status {
	case port.HoldSoldOut:
		return CreateHoldOutput{}, domain.ErrSoldOut
	case port.HoldAlreadyHolding:
		// BR-O3. Tra ve hold dang co thay vi bao loi: khach bam lai nut mua
		// khong nen bi phat.
		return uc.repo.GetActiveHold(ctx, in.EventID, in.IdentityID)
	case port.HoldOK:
	default:
		return CreateHoldOutput{}, fmt.Errorf("redis gate tra ve trang thai la: %s", res.Status)
	}

	// 3. Chot vao Postgres. Toan bo (tru ton kho + tao don + tao hold + outbox)
	//    nam trong MOT transaction.
	expiresAt := time.Now().Add(ttl)
	out, err := uc.repo.PersistHold(ctx, port.PersistHoldParams{
		EventID:      in.EventID,
		TicketTypeID: in.TicketTypeID,
		IdentityID:   in.IdentityID,
		HoldID:       holdID,
		Bucket:       res.Bucket,
		Quantity:     in.Quantity,
		UnitCents:    ev.PriceCents,
		ExpiresAt:    expiresAt,
	})
	if err != nil {
		// DEN BU: Redis da tru kho nhung Postgres tu choi. Phai tra lai ngay,
		// neu khong so ve do se bi khoa cho toi khi job doi soat phat hien.
		if rerr := uc.gate.Release(ctx, in.EventID, holdID); rerr != nil {
			// Khong tra lai duoc thi job doi soat (moi 10s) se sua. Ghi log muc
			// ERROR vi day la sai lech that su can theo doi.
			uc.log.Error("khong tra lai duoc kho sau khi persist that bai",
				"event", in.EventID, "hold", holdID, "err", rerr)
		}
		if errors.Is(err, domain.ErrInventoryExhausted) {
			// Postgres noi het ve trong khi Redis noi con -> Redis dang lech.
			// Tin Postgres. Day chinh la ly do he thong khong the oversell.
			uc.log.Warn("redis lech so voi postgres, tin postgres",
				"event", in.EventID, "ticket_type", in.TicketTypeID, "bucket", res.Bucket)
			return CreateHoldOutput{}, domain.ErrSoldOut
		}
		return CreateHoldOutput{}, err
	}

	return out, nil
}
