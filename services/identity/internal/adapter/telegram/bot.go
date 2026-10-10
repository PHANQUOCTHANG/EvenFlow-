package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eventflow/eventflow/services/identity/internal/adapter/redis"
	tele "gopkg.in/telebot.v3"
)

type Bot struct {
	bot   *tele.Bot
	redis redis.Client
}

func NewBot(token string, redisClient redis.Client) (*Bot, error) {
	if token == "" {
		return nil, fmt.Errorf("telegram bot token is empty")
	}

	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tele.NewBot(pref)
	if err != nil {
		return nil, err
	}

	return &Bot{
		bot:   b,
		redis: redisClient,
	}, nil
}

func (b *Bot) Start() {
	b.bot.Handle("/start", func(c tele.Context) error {
		menu := &tele.ReplyMarkup{ResizeKeyboard: true}
		btnShare := menu.Contact("Bấm vào đây để Chia sẻ Số điện thoại")
		menu.Reply(
			menu.Row(btnShare),
		)

		return c.Send("Chào mừng đến với EventFlow! \nĐể nhận mã xác thực (OTP), vui lòng bấm nút chia sẻ số điện thoại bên dưới. 👇", menu)
	})

	b.bot.Handle(tele.OnContact, func(c tele.Context) error {
		contact := c.Message().Contact
		if contact == nil {
			return c.Send("Vui lòng sử dụng nút chia sẻ số điện thoại.")
		}

		// Normalize phone number (remove +, remove spaces)
		phone := strings.ReplaceAll(contact.PhoneNumber, "+", "")
		phone = strings.ReplaceAll(phone, " ", "")

		// Standardize to VN format
		// If starts with 84, it's fine. If starts with 0, change to 84 (or we just match exactly what web sends).
		// Wait, the web regex accepts both 09x and 849x. It's best if we check both in Redis just in case.
		possibleKeys := []string{phone}
		if strings.HasPrefix(phone, "84") {
			possibleKeys = append(possibleKeys, "0"+phone[2:])
		} else if strings.HasPrefix(phone, "0") {
			possibleKeys = append(possibleKeys, "84"+phone[1:])
		}

		var foundOTP string
		var matchedPhone string
		ctx := context.Background()

		for _, p := range possibleKeys {
			// Check Registration OTP
			otp, err := b.redis.GetOTP(ctx, p)
			if err == nil && otp != "" {
				foundOTP = otp
				matchedPhone = p
				break
			}
			
			// Check Reset Password OTP
			resetOtp, err := b.redis.GetOTP(ctx, "reset_"+p)
			if err == nil && resetOtp != "" {
				foundOTP = resetOtp
				matchedPhone = p
				break
			}
		}

		if foundOTP != "" {
			// Reply with OTP
			msg := fmt.Sprintf("✅ Xác thực thành công!\n\nMã OTP của bạn cho số %s là: **%s**\n\nVui lòng quay lại web và nhập mã này.", matchedPhone, foundOTP)
			return c.Send(msg, tele.ModeMarkdown)
		}

		return c.Send("❌ Không tìm thấy yêu cầu xác thực nào cho số điện thoại này, hoặc mã đã hết hạn. Vui lòng quay lại web và thử lại.")
	})

	fmt.Println("[INFO] Telegram Bot đang lắng nghe...")
	b.bot.Start()
}

func (b *Bot) Stop() {
	b.bot.Stop()
}
