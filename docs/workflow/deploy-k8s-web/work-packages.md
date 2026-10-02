# P1 — Phân rã work package (slice deploy)

Theo §7 của `multi_agent_coding_workflow.pdf`. Đọc cùng `spec.md` (AC) và `contracts.md` (hợp đồng đã
đóng băng).

---

## 1. Vì sao slice này chia song song được

§7.1 của PDF cho phép song song khi *"các phần có ranh giới file/module rõ ràng"* và *"có thể định
nghĩa interface/contract trước khi code"*. Cả hai đều đúng ở đây:

- Ba phần việc nằm ở **ba vùng file không giao nhau**: mã ứng dụng, manifest hạ tầng, script công cụ.
- Thứ duy nhất nối chúng là **đường dẫn + mã trạng thái của health endpoint**, và cái đó chốt được
  trước khi viết một dòng code nào (C1 trong `contracts.md`).

Đối chiếu với cột "không nên song song" của §7.1: không phần nào cùng sửa một file; kiến trúc đã rõ;
contract không còn thay đổi sau P2; task không nhỏ tới mức chi phí điều phối lớn hơn lợi ích; và
**không có ràng buộc thứ tự bắt buộc** giữa ba WP — không WP nào cần kết quả chạy của WP khác.

> Ghi chú trung thực: hai slice trước **không** song song, và đó là quyết định đúng theo chính §7.1 —
> contract lúc đó còn đổi giữa chừng (`totalAhead` → `initialRank`, thêm `ready`). Nếu khi ấy có ba
> agent code song song theo contract cũ thì phải vứt cả ba. Slice này khác ở chỗ contract là hạ tầng,
> ổn định, và chốt được trước.

## 2. Đồ thị phụ thuộc

```
                 P2 Contract Freeze & Review
                            |
          +-----------------+-----------------+
          v                 v                 v
        WP-A              WP-B              WP-C
   health endpoint    k8s manifest     gate script + compose
          |                 |                 |
          +-----------------+-----------------+
                            v
                  P4 Integration (gates trên kết quả đã merge)
                            |
                            v
              P4.5 Xác minh chạy thật (NỐI TIẾP, không song song)
              build image -> apply -> port-forward -> SIGTERM -> rollout undo
```

Ba WP **không phụ thuộc lẫn nhau**, chỉ cùng phụ thuộc contract. Chạy đồng thời cả ba, dưới giới hạn
khuyến nghị 3–5 agent của §7.2.

## 3. Work package

### WP-A — Health endpoint

| | |
|---|---|
| AC phụ trách | AC-1, AC-2, **AC-2b** (graceful shutdown, tách từ AC-4b vì AC đó có hai chủ) |
| Sở hữu | `apps/web/src/app/api/healthz/route.ts`, `apps/web/src/app/api/readyz/route.ts`, `apps/web/src/lib/readiness.ts`, `apps/web/src/instrumentation.ts` — **tên file cụ thể, không dùng glob `**`**: glob của bản 1 nuốt luôn file test, xung đột với quy tắc "không WP nào sửa test" |
| Contract | C1 |
| Test | Do bước Test Design viết **trước**; WP-A **không** được sửa file test |

Hai route handler theo đúng C1. Phần dễ sai nhất **không** phải code, mà là hai điều kiện:
liveness không chạm dependency ngoài, và cả hai route phải thực sự động (nếu bị prerender thì probe
trả về một kết quả đóng băng từ lúc build và **luôn xanh kể cả khi tiến trình đã hỏng**).

### WP-B — Manifest Kubernetes

| | |
|---|---|
| AC phụ trách | AC-4, **AC-4b** (rolling update), **AC-4c** (tham số probe) |
| Sở hữu | `deploy/k8s/**` |
| Contract | C1 (đường dẫn probe), C2 (cổng, tên image, namespace, nhãn) |

Namespace, Deployment, Service, Ingress, HPA. Điểm bắt buộc: probe trỏ đúng hai đường dẫn của C1;
`resources.requests` **và** `limits` cho cpu + memory (thiếu `requests` thì HPA không có mẫu số và
không scale được); `securityContext` chạy non-root drop ALL capabilities; `minReplicas` ≥ 2.

⚠️ **Sửa khẳng định sai của bản 1** (B1, đã kiểm bằng lệnh): `kubectl apply --dry-run=client`
**không** chạy offline — nó tải openapi từ API server và fail khi chưa có cluster. Bằng chứng hợp lệ
duy nhất cho AC-4 là `kubectl apply --dry-run=server` ở **bước P4.5**, sau khi bật Kubernetes của
Docker Desktop. Trong lúc P3, WP-B **không có cách tự chứng minh manifest đúng** — đây là giới hạn đã
biết, không phải thiếu sót của WP-B. Xem C4.4.

### WP-C — Script gate + healthcheck compose

| | |
|---|---|
| AC phụ trách | AC-3, AC-5, **AC-5b** (chặn .skip/.only mới thêm + chặn probe bị prerender) |
| Sở hữu | `.github/scripts/gates.sh`, `Makefile`, `deploy/compose/docker-compose.yml` |
| Contract | C1 (đường dẫn), C3 (lệnh healthcheck), C4 (hành vi script) |

Script gate tồn tại vì một lý do cụ thể, không phải vì đẹp quy trình: ở slice trước một type error đã
lọt vào commit `9607fa6` do chạy `test` + `lint` + `build` mà **quên `typecheck`** — nhưng ⚠️ **sửa lý
do**: đó là vòng chạy **cục bộ**, không phải lỗ hổng CI. `ci.yml` **đã có đủ bốn lệnh** (dòng 104, 105,
170, 227). Script này để chạy đúng bốn lệnh đó cục bộ trước khi push, cộng **hai check CI không có**:
chặn `.skip`/`.only` mới thêm, và chặn probe bị prerender. PDF bước 6 ghi rõ
gate là *"script, không dùng LLM"* đúng để chặn kiểu lỗi đó.

Service `web` trong compose chưa có `healthcheck` trong khi postgres/redis/rabbitmq đều có, nên
`docker compose -f deploy/compose/docker-compose.yml up -d` báo "đã chạy" trước khi web thật sự phục vụ được.

## 4. Việc của Integrator (không giao cho WP nào)

**P4 — tích hợp**
- Merge ba nhánh theo DAG
- Chạy **toàn bộ** gates trên **kết quả đã merge** — §P4: *"Branch xanh chưa chắc merge xanh"*
- Kiểm diff từng WP có nằm trong ownership không
- Sửa `deploy/docker/web.Dockerfile`: thêm `ENV HOSTNAME=0.0.0.0`, `ENV PORT=3000` (C3), và
  `ARG NEXT_PUBLIC_API_BASE` (C7)
- `git update-index --chmod=+x .github/scripts/gates.sh`

**P4.5 — xác minh chạy thật (bổ sung sau P2, AC-8)**

Bản 1 kết thúc ở "merge + gates + runbook", tức slice sinh ra một đống YAML **chưa ai chạy**, và nhãn
"deploy được lên Kubernetes" không có cơ sở. Bước này **nối tiếp, không song song**:

1. Bật Kubernetes của Docker Desktop
2. `docker build` image — lần đầu tiên file này được build thành công, — lý do chưa build được là
   **Docker daemon chưa chạy**, không phải lỗi Dockerfile (`apps/web/public/.gitkeep` đã tồn tại)
3. `kubectl apply --dry-run=server` (lúc này mới chạy được), rồi apply thật
4. `kubectl port-forward`, xác nhận pod `Ready`, `/api/healthz` 200, `/api/readyz` 200
5. Gửi `SIGTERM`, xác nhận `/api/readyz` chuyển 503 **trước khi** pod biến mất
6. `kubectl rollout undo`, xác nhận quay về đúng tag trước
7. Log thô dán vào `integration-report.md`

- Viết `runbook.md` (AC-6)
- Viết `integration-report.md`

## 5. Giới hạn quyền của agent WP

Theo §6 và §7.2 của PDF:

- Chỉ sửa file trong ownership của mình. Chạm file khác là fail ở P4.
- **Không** sửa test, không sửa CI, không sửa contract.
- **Không** merge. Agent chỉ báo cáo trạng thái; chỉ Integrator được merge.
- Không đọc branch của WP khác. Mọi trao đổi qua artifact.
- Không truy cập secret, không push.
- Tự chạy gates trên nhánh của mình, tối đa 3 vòng, sau đó escalate.

## 6. Xử lý sự cố (§7.5)

| Tình huống | Hành động |
|---|---|
| Một WP fail sau 3 vòng | Escalate lên người dùng. Hai WP còn lại vẫn tiếp tục vì không phụ thuộc. |
| Contract sai hoặc thiếu | Dừng mọi WP, quay lại P1, duyệt lại ở P2. Không tự "chọn bên". |
| Conflict khi merge | Do chồng ownership → quay lại P1 tách lại, **không** giải quyết conflict bằng cách đoán. |
| Gates fail sau merge dù từng branch đều xanh | Integrator xác định WP gây lỗi và trả lại đúng WP đó kèm log. |
