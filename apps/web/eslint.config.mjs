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
      // `caughtErrors` GIU mac dinh "all" cua ESLint 9 — khong noi long cho toan repo.
      "@typescript-eslint/no-unused-vars": [
        "warn",
        {
          args: "after-used",
          argsIgnorePattern: "^_",
          varsIgnorePattern: "^_",
        },
      ],
      "@typescript-eslint/no-explicit-any": "warn",
      "@next/next/no-img-element": "warn",
    },
  },
];

export default config;
