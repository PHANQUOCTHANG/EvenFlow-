// Package signaladapter doc suc khoe that cua tang duoi (ticketing) cho admit controller.
package signaladapter

import (
	"context"
	"errors"
	"fmt"

	"github.com/eventflow/eventflow/libs/go/loadsignal"
	"github.com/eventflow/eventflow/services/waitingroom/internal/app"
)

// Reader la phan cua loadsignal.Reader ma probe can (de test bang fake).
type Reader interface {
	Read(ctx context.Context) (loadsignal.Aggregate, error)
}

// Probe hien thuc app.HealthProbe tu anh chup do cac pod ticketing tu ghi len Redis.
//
// Tin hieu la cua CA tang duoi (p99 / loi cua luong giu ghe, muc dung pool), khong theo su kien:
// Postgres va pool la tai nguyen chung, nen mot su kien cham keo ca he thong cham.
type Probe struct{ r Reader }

func NewProbe(r Reader) *Probe { return &Probe{r: r} }

// Health tra ve loi khi KHONG BIET (khong pod nao bao cao, Redis loi). Controller coi loi la suc
// ep va giam nhip (BR-Q5): mu ma van tha nguoi la cach nhanh nhat de lam sap he thong.
//
// Truoc T0, khi chua co yeu cau nao, pod van bao cao 0 yeu cau / p99 0 -- do la "khoe", khong phai
// "khong biet".
func (p *Probe) Health(ctx context.Context, _ string) (app.Health, error) {
	a, err := p.r.Read(ctx)
	if err != nil {
		if errors.Is(err, loadsignal.ErrNoSignal) {
			return app.Health{}, fmt.Errorf("khong co tin hieu suc khoe cua ticketing: %w", err)
		}
		return app.Health{}, err
	}
	return app.Health{
		CheckoutP99: a.P99,
		ErrorRate:   a.ErrorRate,
		DBPoolUsage: a.PoolUsage,
	}, nil
}
