CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 1. Bảng Users (Lưu thông tin tài khoản cơ bản)
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    role VARCHAR(20) NOT NULL DEFAULT 'buyer',
    status VARCHAR(20) NOT NULL DEFAULT 'inactive', -- active, inactive, banned
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 2. Bảng User Identities (Lưu phương thức đăng nhập)
-- Mỗi user có thể đăng nhập bằng nhiều cách: Email, Phone, VNeID
CREATE TABLE IF NOT EXISTS user_identities (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider VARCHAR(20) NOT NULL, -- 'email', 'phone', 'vneid'
    identifier VARCHAR(255) NOT NULL, -- nguyenvana@gmail.com, 0987654321, 001099001122
    password_hash VARCHAR(255), -- Có thể null nếu dùng VNeID
    is_verified BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(provider, identifier)
);

-- 3. Bảng User Profiles (Lưu thông tin cá nhân)
CREATE TABLE IF NOT EXISTS user_profiles (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    full_name VARCHAR(100),
    dob DATE,
    gender VARCHAR(10),
    avatar_url TEXT,
    cccd_number VARCHAR(20) UNIQUE, -- Số thẻ CCCD nếu được định danh
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Tạo Index để truy vấn nhanh khi Đăng nhập
CREATE INDEX idx_user_identities_identifier ON user_identities(provider, identifier);
