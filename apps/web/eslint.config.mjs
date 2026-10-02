import { dirname } from "node:path";
import { fileURLToPath } from "node:url";

import { FlatCompat } from "@eslint/eslintrc";

// ESLint 9 dung flat config. `eslint-config-next` van la eslintrc nen phai qua FlatCompat.
// Truoc EVF-1801 repo KHONG co file config nao -> `next lint` roi vao prompt interactive
// va job lint-web trong CI khong thuc su lint gi.
const compat = new FlatCompat({
  baseDirectory: dirname(fileURLToPath(import.meta.url)),
});

const config = [
  {
    ignores: [".next/**", "coverage/**", "node_modules/**", "next-env.d.ts"],
  },
  ...compat.extends("next/core-web-vitals", "next/typescript"),
  {
    rules: {
      // Quy uoc chung: bien/tham so co tien to `_` la co y khong dung.
      // Vi du that trong repo: `for await (const _ of watchQueue(...))` chi de dem vong lap.
      // `caughtErrors` GIU mac dinh "all" cua ESLint 9 — khong noi long cho toan repo.
      "@typescript-eslint/no-unused-vars": [
        "error",
        {
          args: "after-used",
          argsIgnorePattern: "^_",
          varsIgnorePattern: "^_",
        },
      ],
    },
  },
  {
    // Ngoai le thu hep dung MOT file. `src/lib/queue-client.ts:127` co `} catch (err) {` co y
    // khong dung `err` (chi lui dan roi thu lai). ESLint 9 doi mac dinh thanh
    // `caughtErrors: "all"` nen ma viet theo ESLint 8 bi bao loi.
    //
    // Cach dung la doi thanh `} catch {` (optional catch binding, ES2019; target la ES2022) —
    // 6 ky tu, behavior-identical. Nhung plan.md muc 2 liet queue-client.ts vao nhom
    // "KHONG duoc sua" va muc 6 yeu cau dung-va-bao, nen khoanh vung o day thay vi sua file.
    // Code MOI van bi check day du.
    //
    // Can mo ticket rieng: catch block do nuot ca QueueError, nen 403/410 bi retry vo han
    // voi backoff thay vi fail nhanh. Do la bug that, khong phai chuyen lint.
    files: ["src/lib/queue-client.ts"],
    rules: {
      "@typescript-eslint/no-unused-vars": [
        "error",
        { args: "after-used", argsIgnorePattern: "^_", varsIgnorePattern: "^_", caughtErrors: "none" },
      ],
    },
  },
];

export default config;
