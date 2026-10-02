"use client";

import { useState } from "react";

import {
  Alert,
  Badge,
  Button,
  Card,
  Checkbox,
  Input,
  RadioGroup,
  Select,
  Spinner,
  useToast,
  type BadgeVariant,
  type ButtonSize,
  type ButtonVariant,
} from "@/components/ui";

import { Row, Section } from "./section";

const VARIANTS: ButtonVariant[] = ["primary", "secondary", "tertiary", "destructive"];
const SIZES: ButtonSize[] = ["sm", "md", "lg"];

const BADGES: BadgeVariant[] = [
  "lobby",
  "queued",
  "admitted",
  "holding",
  "paid",
  "pending",
  "expired",
  "soldout",
  "failed",
  "neutral",
];

export function PrimitiveGallery() {
  const { toast } = useToast();
  const [quantity, setQuantity] = useState("2");
  const [agreed, setAgreed] = useState(false);
  const [method, setMethod] = useState("card");

  return (
    <Section
      title="Thành phần dùng chung"
      description="Bộ component ở src/components/ui. Mọi trạng thái dưới đây đều có test tự động."
    >
      <Row label="Button — 4 biến thể × 3 kích thước">
        {VARIANTS.map((variant) =>
          SIZES.map((size) => (
            <Button key={`${variant}-${size}`} variant={variant} size={size}>
              {variant} {size}
            </Button>
          )),
        )}
      </Row>

      <Row label="Button — trạng thái">
        <Button disabled>Vô hiệu hoá</Button>
        <Button loading>Đang xử lý</Button>
        <Button variant="destructive">Huỷ lô vé</Button>
      </Row>

      <Row label="Badge — trạng thái nghiệp vụ">
        {BADGES.map((variant) => (
          <Badge key={variant} variant={variant}>
            {variant}
          </Badge>
        ))}
      </Row>

      <Row label="Spinner">
        <Spinner size="sm" />
        <Spinner size="md" />
        <Spinner size="lg" />
      </Row>

      <div className="grid gap-md md:grid-cols-2">
        <Card header="Trường nhập liệu">
          <div className="flex flex-col gap-md">
            <Input label="Họ và tên" placeholder="Nguyễn Văn A" />
            <Input label="Email" type="email" helpText="Dùng để gửi vé điện tử." />
            <Input label="Mật khẩu" type="password" />
            <Input label="Mã OTP" type="otp" helpText="6 chữ số gửi qua SMS." />
            <Input label="Số lượng" error="Vượt quá giới hạn 4 vé mỗi đơn." value={quantity} onChange={(event) => setQuantity(event.target.value)} />
            <Select
              label="Hạng vé"
              placeholder="Chọn hạng vé"
              options={[
                { value: "standard", label: "Vé thường" },
                { value: "vip", label: "Vé VIP" },
                { value: "sold", label: "Vé hạng A (hết)", disabled: true },
              ]}
            />
            <Checkbox
              label="Tôi đồng ý với điều khoản"
              checked={agreed}
              onChange={(event) => setAgreed(event.target.checked)}
            />
            <RadioGroup
              name="payment"
              legend="Phương thức thanh toán"
              value={method}
              onValueChange={setMethod}
              options={[
                { value: "card", label: "Thẻ nội địa" },
                { value: "ewallet", label: "Ví điện tử" },
              ]}
            />
          </div>
        </Card>

        <Card header="Thông báo">
          <div className="flex flex-col gap-md">
            <Alert variant="info" title="Đang xếp hàng">
              Giữ nguyên tab này, trang tự cập nhật.
            </Alert>
            <Alert variant="success" title="Thanh toán thành công">
              Vé điện tử đã được gửi tới email của bạn.
            </Alert>
            <Alert variant="warning" title="Sắp hết thời gian giữ vé">
              Hoàn tất thanh toán trước khi đồng hồ về 0.
            </Alert>
            <Alert variant="error" title="Giao dịch thất bại">
              Cổng thanh toán từ chối giao dịch này.
            </Alert>
            <Button
              variant="secondary"
              onClick={() =>
                toast({ variant: "success", title: "Đây là toast mẫu", description: "Tự ẩn sau 5 giây." })
              }
            >
              Hiện một toast
            </Button>
          </div>
        </Card>
      </div>
    </Section>
  );
}
