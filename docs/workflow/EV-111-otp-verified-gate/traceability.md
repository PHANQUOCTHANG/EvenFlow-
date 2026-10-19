# EV-111 — Truy vết AC → test

| AC | Test |
|---|---|
| AC-1 | `identity/internal/app/register_test.go` (`successful registration with phone` kiểm `OtpVerifiedAt`) · `postgres/identity_repo_pii_integration_test.go` `TestIdentityRepo_GhiOtpVerifiedAt` (Postgres thật) |
| AC-2 | `authx` `TestAccessToken_CoXacThucOTPDiTheoToken`, `TestPrincipal_VerifiedChiLaTrueKhiGatewayGan` · `identity/app` `TestSession_TokenMangCoXacThucOTP` · `gateway` `TestGateway_ChuyenCoXacThucOTPTuToken` |
| AC-3 | `waitingroom/internal/adapter/http/handler_test.go` `TestJoin_ChiChoIdentityDaXacThucOTP` (5 ca, kiểm không ghi danh khi bị từ chối) |
| AC-4 | `TestIdentityRepo_GhiOtpVerifiedAt` (chạy lại đúng câu backfill của migration 0004) |
