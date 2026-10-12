# EVF-1806 — Baseline (trạng thái `develop` trước khi gộp nhánh)

Chụp lúc: 2026-10-12 14:55:47 +07:00, do agent verify độc lập đo.

## 1. Mốc đo

| | |
|---|---|
| Commit | `ad09bf3e76d16bc78c328d2d67a02d6332f748d0` (`origin/develop`, "Merge pull request #44 … EV-114-rbac-authorization", 2026-10-11 18:54 +07:00) |
| Quan hệ với nhánh | `ad09bf3` là merge-base của `feature/EVF-1806-assistant-panel` (HEAD `5f2ec72` = merge `origin/develop` vào nhánh) |
| Cách đo | **Đo thật**, không suy ra. Worktree tạm, không đụng working tree chính |
| Môi trường | Windows 11, Node v22.21.0, npm 10.9.4, Vitest 2.1.8, jsdom |

## 2. Lệnh

```bash
git worktree add --detach <scratchpad>/wt-develop origin/develop
# node_modules: junction tới apps/web/node_modules của working tree chinh
# (package.json / package-lock.json khong khac giua ad09bf3 va HEAD — `git diff --stat origin/develop HEAD` khong liet ke chung)
mklink /J <scratchpad>\wt-develop\apps\web\node_modules  D:\Documents\Nam4-1\Project1\EvenFlow-\apps\web\node_modules
cd <scratchpad>/wt-develop/apps/web && npx vitest run      # = script "test" trong package.json
# don dep: rmdir junction (chi xoa link) -> git worktree remove --force -> git worktree prune
```

## 3. Kết quả

```text
 RUN  v2.1.8 C:/Users/trand/AppData/Local/Temp/claude/.../scratchpad/wt-develop/apps/web
 Test Files  91 passed (91)
      Tests  1345 passed (1345)
   Start at  14:55:50
   Duration  24.24s
EXIT=0
```

- **91 file test, 1345/1345 pass, 0 fail.** Không có file nào của EVF-1806 trong lần chạy này (grep
  `assistant-panel` / `domain-gallery.test` = 0 dòng).
- Đối chiếu: nhánh có **93 file, 1532 test** → chênh **+2 file, +187 test** = đúng 179 (AP) + 8 (DG).
  Không có test nào của `develop` bị mất hay đổi số.

## 4. Lịch sử baseline (để không nhầm số cũ)

| Mốc | Commit develop | Số test | Ghi chú |
|---|---|---|---|
| Lúc viết spec | `a8fb8c9` (develop cục bộ) | 1086 | `spec-plan.md` §5 bước 1 |
| Round 0 (trước sửa review) | `a8fb8c9` | 1246 tổng trên nhánh | `HANDOFF.md` |
| **Hiện tại** | `ad09bf3` (`origin/develop`) | **1345** | develop đã nhận thêm EV-185, EV-186, EV-114… (+259 test so với 1086) |

**Lưu ý:** `a8fb8c9` ("chore(web): exclude test files from coverage measurement") **không** nằm trong
`origin/develop`; nó đi theo nhánh này vào PR (xem `evidence.md` §6).

## 5. Test có thể bị ảnh hưởng

| File | Ảnh hưởng dự kiến |
|---|---|
| `src/components/assistant/{chat-panel,faq-chips,streaming-message}.test.tsx` (EV-185) | Cùng thư mục, `index.ts` re-export cả hai bộ. Không được gãy |
| `src/components/showcase/*` | Chỉ thêm section mới vào `domain-gallery.tsx`; trên develop chưa có test showcase |
| `src/lib/server-time.test.ts`, `src/hooks/use-server-countdown.test.tsx` | `AssistantPanel` dùng `toEpochMs` (chỉ đọc, không sửa) |
