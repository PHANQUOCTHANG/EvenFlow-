/** Nhãn hiển thị cho khu Moderator (EV-121). */

import type { AiVerdict } from "@/lib/moderation-service";

/** Khoá trục do ai-worker gửi (app/domain/moderation.py, BR-E4). Khoá lạ vẫn hiện bằng tên gốc. */
export const AXIS_LABELS: Record<string, string> = {
  forbidden_content: "Nội dung cấm",
  misleading: "Thông tin gây hiểu lầm",
  copyright: "Bản quyền hình ảnh",
  incomplete_info: "Thiếu thông tin",
};

export const AXIS_ORDER = ["forbidden_content", "misleading", "copyright", "incomplete_info"];

export const VERDICT_LABELS: Record<AiVerdict, string> = {
  AUTO_APPROVED: "AI đã duyệt",
  NEEDS_HUMAN: "Cần người duyệt",
  AUTO_REJECTED: "AI đã từ chối",
};

const AUDIT_LABELS: Record<string, string> = {
  "event.needs_human_review": "AI chuyển người duyệt",
  "event.auto_approved": "AI tự duyệt",
  "event.auto_rejected": "AI tự từ chối",
  "event.moderator_approved": "Moderator duyệt",
  "event.moderator_rejected": "Moderator từ chối",
  "event.moderator_overrode_ai": "Moderator lật ngược quyết định AI",
  "event.moderator_overrode_decision": "Moderator lật ngược quyết định trước",
};

export function auditLabel(action: string): string {
  return AUDIT_LABELS[action] ?? action;
}

export function actorLabel(actorType: string): string {
  if (actorType === "ai") return "AI";
  if (actorType === "human") return "Moderator";
  if (actorType === "system") return "Hệ thống";
  return actorType;
}

export function percent(score: number): number {
  return Math.round(Math.min(1, Math.max(0, score)) * 100);
}

/** Bỏ dấu + thường hoá để tìm "nha hat" khớp "Nhà hát". */
export function foldVi(text: string): string {
  return text
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .replace(/đ/g, "d")
    .replace(/Đ/g, "D")
    .toLowerCase();
}
