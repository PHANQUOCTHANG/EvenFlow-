# Tài liệu Thiết kế Cơ bản - EvenFlow

Tài liệu này cung cấp các sơ đồ thiết kế ban đầu bao gồm Sơ đồ Use Case (ca sử dụng) và Lược đồ cơ sở dữ liệu (ERD) nhằm minh họa cách các tác nhân tương tác với hệ thống và cấu trúc dữ liệu cốt lõi.

---

## 1. Sơ đồ Use Case (Ca sử dụng)

Sơ đồ dưới đây mô tả tương tác giữa các tác nhân (Actors) và các chức năng chính của hệ thống.

```mermaid
graph LR
    %% Actors
    Buyer((Khách hàng))
    Organizer((Ban tổ chức))
    Moderator((Kiểm duyệt viên))
    Ops((Vận hành - Ops))
    AI((AI Agent))

    %% Use Cases for Buyer
    Buyer --> UC1(Tham gia hàng chờ)
    Buyer --> UC2(Hỏi trợ lý AI)
    Buyer --> UC3(Chọn & Giữ vé)
    Buyer --> UC4(Thanh toán đơn hàng)
    Buyer --> UC5(Check-in sự kiện)

    %% Use Cases for Organizer
    Organizer --> UC6(Tạo & Chỉnh sửa sự kiện)
    Organizer --> UC7(Cấu hình hạng vé & Tồn kho)
    Organizer --> UC8(Xem báo cáo sau mở bán)

    %% Use Cases for Moderator
    Moderator --> UC9(Duyệt/Từ chối sự kiện)
    Moderator --> UC10(Xử lý khiếu nại chặn Bot)
    Moderator --> UC11(Phê duyệt Rule chống bot mới)

    %% Use Cases for Ops
    Ops --> UC12(Điều chỉnh Admit Rate)
    Ops --> UC13(Bật/Tắt chế độ suy biến AI)
    Ops --> UC8

    %% Use Cases for AI Agent
    AI --> UC14(Trả lời chat trong phòng chờ)
    AI --> UC15(Kiểm duyệt nội dung sự kiện)
    AI --> UC16(Phát hiện hành vi Bot bất thường)
    AI --> UC17(Sinh báo cáo sau mở bán)
    
    %% Implicit relationships
    UC14 -.->|Hỗ trợ| Buyer
    UC15 -.->|Cảnh báo/Tự động duyệt| UC9
    UC16 -.->|Đề xuất Rule| UC11
```

![Sơ đồ Use Case](./image/Usecase-diagram.png)

---

## 2. Lược đồ Cơ sở dữ liệu (ERD)

Lược đồ dưới đây được kết xuất chi tiết dựa trên file migration `0001_core.sql`. Tất cả các bảng, các trường quan trọng và mối quan hệ khóa ngoại (Foreign Keys) được thể hiện chính xác so với cơ sở dữ liệu vật lý.

```mermaid
erDiagram
    %% Core Entities & Relationships
    identities ||--o{ devices : "has"
    identities ||--o{ events : "organizes"
    identities ||--o{ moderation_reviews : "reviews"
    identities ||--o{ orders : "places"
    identities ||--o{ queue_sessions : "joins"
    
    events ||--o{ ticket_types : "has"
    events ||--o{ moderation_reviews : "reviewed in"
    events ||--o{ orders : "has"
    events ||--o{ sale_reports : "has"
    events ||--o{ queue_sessions : "has"

    ticket_types ||--o{ ticket_inventory : "split into"
    ticket_types ||--o{ order_items : "bought as"
    ticket_types ||--o{ seat_holds : "held as"
    ticket_types ||--o{ tickets : "issued as"

    orders ||--o{ order_items : "contains"
    orders ||--o{ seat_holds : "secures"
    orders ||--o{ payments : "paid via"
    orders ||--o{ tickets : "issues"

    %% Table Definitions
    identities {
        uuid id PK
        text email "UNIQUE"
        text phone_hash "UNIQUE"
        text password_hash
        text role "buyer|organizer|moderator|ops"
        timestamptz created_at
    }

    devices {
        uuid id PK
        uuid identity_id FK
        text fingerprint
        inet last_ip
        timestamptz last_seen
    }

    events {
        uuid id PK
        uuid organizer_id FK
        text slug "UNIQUE"
        text title
        timestamptz sale_start_at
        text status "DRAFT|SCHEDULED|ON_SALE..."
        int hold_ttl_seconds
    }

    ticket_types {
        uuid id PK
        uuid event_id FK
        text name
        bigint price_cents
        int quota
        int bucket_count
    }

    ticket_inventory {
        uuid ticket_type_id PK, FK
        int bucket PK
        int capacity
        int available "CHECK(available >= 0)"
    }

    orders {
        uuid id PK
        uuid event_id FK
        uuid identity_id FK
        text status "HELD|PAYMENT_PENDING|PAID..."
        bigint total_cents
        timestamptz expires_at
    }

    order_items {
        uuid id PK
        uuid order_id FK
        uuid ticket_type_id FK
        int bucket
        int quantity
    }

    seat_holds {
        uuid id PK
        uuid order_id FK
        uuid ticket_type_id FK
        int quantity
        timestamptz expires_at
        timestamptz released_at
    }

    payments {
        uuid id PK
        uuid order_id FK
        text provider
        text provider_txn_id "UNIQUE with provider"
        bigint amount_cents
        text status
    }

    tickets {
        uuid id PK
        uuid order_id FK
        uuid ticket_type_id FK
        text jti "UNIQUE (Anti-copy)"
        text qr_sig
        text status "ISSUED|CHECKED_IN"
    }

    queue_sessions {
        text token PK
        uuid event_id FK
        uuid identity_id FK
        bigint rank
        text state "LOBBY|QUEUED|ADMITTED..."
    }

    %% Audit & System Tables
    outbox_events {
        bigserial id PK
        text aggregate_type
        uuid aggregate_id
        jsonb payload
        timestamptz published_at
    }

    idempotency_keys {
        text key PK
        text endpoint PK
        uuid identity_id
        timestamptz expires_at
    }

    bot_decisions {
        bigserial id PK
        uuid event_id
        uuid identity_id
        numeric score
        text action "ALLOW|CHALLENGE|BLOCK..."
    }
```

![Sơ đồ ERD](./image/ERD-diagram.png)
