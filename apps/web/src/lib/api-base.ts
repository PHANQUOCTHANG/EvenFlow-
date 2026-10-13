/** Goc URL cua API backend (EV-198, B2).
 *
 * Next INLINE `process.env.NEXT_PUBLIC_API_BASE` vao bundle luc build, nhung chi khi bieu thuc
 * duoc viet NGUYEN VAN nhu duoi day: destructure (`const { NEXT_PUBLIC_API_BASE } = process.env`)
 * hay khoa dong (`process.env[name]`) se KHONG duoc thay va tra `undefined` tren trinh duyet.
 * Doc trong than ham (khong phai hang module) de `vi.stubEnv` trong test co tac dung.
 *
 * Mac dinh "" = URL tuong doi tren chinh origin cua web — chi dung cho test/dev. */
export function apiBase(): string {
  return process.env.NEXT_PUBLIC_API_BASE ?? "";
}
