/** Gop className. Gia tri falsy bi bo qua.
 *
 * Co tinh de don gian: KHONG tu y go class trung hay class doi nhau. Class truyen tu
 * ngoai vao duoc GIU LAI va dat sau class mac dinh, nho vay caller ghi de duoc bang
 * thu tu CSS ma component khong bao gio "an mat" class cua caller (AC-3). */
export function cn(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(" ");
}
