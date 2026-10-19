package authx

import (
	"errors"
	"os"
	"strings"
)

// DevQueueTokenSecret chi dung khi chay local: waitingroom va ticketing cung dung gia tri nay nen
// token cap o mot ben van xac minh duoc o ben kia ma khong can .env. Production thieu secret thi
// QueueTokenSecretFromEnv bao loi thay vi roi ve gia tri nay.
const DevQueueTokenSecret = "development-queue-token-secret"

// QueueTokenSecretFromEnv doc QUEUE_TOKEN_SECRET (it nhat 16 byte). Khi chua dat: APP_ENV=production
// -> loi; nguoc lai tra DevQueueTokenSecret va dev=true de noi goi canh bao.
func QueueTokenSecretFromEnv() (secret []byte, dev bool, err error) {
	s := os.Getenv("QUEUE_TOKEN_SECRET")
	if s == "" {
		if strings.EqualFold(os.Getenv("APP_ENV"), "production") {
			return nil, false, errors.New("QUEUE_TOKEN_SECRET bat buoc khi APP_ENV=production")
		}
		return []byte(DevQueueTokenSecret), true, nil
	}
	if len(s) < minSecretLen {
		return nil, false, ErrInvalidSecret
	}
	return []byte(s), false, nil
}
