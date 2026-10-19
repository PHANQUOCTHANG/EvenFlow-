-- +goose Up
-- EV-111 (BR-Q3): identity chi duoc xep hang khi otp_verified_at IS NOT NULL.
-- Truoc EV-111 khong code nao ghi cot nay, nhung moi duong tao tai khoan (OTP email/SDT, Google,
-- VNeID) deu da xac thuc nguoi dung; coi cac tai khoan cu la da xac thuc tai thoi diem tao.
UPDATE identities SET otp_verified_at = created_at WHERE otp_verified_at IS NULL;

-- +goose Down
-- Khong hoan lai duoc: khong phan biet dong da backfill voi dong da co gia tri.
SELECT 1;
