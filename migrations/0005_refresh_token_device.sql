-- +goose Up
-- EV-113: phien dang nhap gan voi thiet bi. Access token mang claim "did" lay tu cot nay; xoay refresh
-- token giu nguyen thiet bi. Xoa thiet bi thi phien khong con gan (SET NULL), khong xoa phien.
ALTER TABLE refresh_tokens ADD COLUMN device_id UUID REFERENCES devices(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS device_id;
