import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

/**
 * Quet axe-core cho EV-188 (spec AC-1, AC-2; plan §1.4 + §6 m1).
 *
 * - Tag: wcag2a, wcag2aa, wcag21a, wcag21aa (spec AC-1).
 * - FAIL khi co vi pham impact `serious` / `critical`; thong bao gom nhan trang, rule id, impact,
 *   so node, selector tung node va help URL (AC-2).
 * - `moderate` / `minor` va `incomplete` (axe khong tu ket luan duoc, vd. tuong phan tren nen
 *   gradient/anh) KHONG lam fail: in bang ngan ra log test va dinh kem toan bo ket qua (JSON).
 * - Dung `expect.soft`: mot luong quet nhieu trang van chay het va bao du moi trang vi pham,
 *   test van FAIL o cuoi.
 * - KHONG disableRules / exclude (AC-2). Request ben thu ba duoc chan o tung spec (vd. sign-in),
 *   khong phai o day.
 */

export const WCAG_TAGS = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"] as const;

const BLOCKING_IMPACTS = new Set(["serious", "critical"]);

type AxeResults = Awaited<ReturnType<AxeBuilder["analyze"]>>;
type AxeResult = AxeResults["violations"][number];

function nodeTarget(node: AxeResult["nodes"][number]): string {
  return node.target.map((t) => (Array.isArray(t) ? t.join(" >>> ") : String(t))).join(" | ");
}

function formatBlocking(label: string, violations: AxeResult[]): string {
  const lines = [`[${label}] ${violations.length} vi pham serious/critical (axe, ${WCAG_TAGS.join(", ")}):`];
  for (const v of violations) {
    lines.push(`- ${v.id} (${v.impact}): ${v.help} — ${v.nodes.length} node`);
    for (const node of v.nodes) {
      lines.push(`    ${nodeTarget(node)}`);
    }
    lines.push(`    ${v.helpUrl}`);
  }
  return lines.join("\n");
}

function slug(label: string): string {
  return label
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/[^a-zA-Z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .toLowerCase();
}

/**
 * Quet trang hien tai. `label` nen gom trang + trang thai + theme (+ locale), vd. "checkout sau giu | dark".
 * Tra ve ket qua axe de spec co the kiem them neu can.
 */
export async function expectNoSeriousA11yViolations(page: Page, label: string): Promise<AxeResults> {
  const results = await new AxeBuilder({ page }).withTags([...WCAG_TAGS]).analyze();

  const blocking = results.violations.filter((v) => BLOCKING_IMPACTS.has(String(v.impact)));
  const nonBlocking = results.violations.filter((v) => !BLOCKING_IMPACTS.has(String(v.impact)));

  await test.info().attach(`axe-${slug(label)}.json`, {
    contentType: "application/json",
    body: JSON.stringify(
      {
        label,
        url: results.url,
        timestamp: results.timestamp,
        tags: WCAG_TAGS,
        violations: results.violations,
        incomplete: results.incomplete,
        passes: results.passes.length,
        inapplicable: results.inapplicable.length,
      },
      null,
      2,
    ),
  });

  const rows = [
    ...nonBlocking.map((v) => ({ loai: "violation", rule: v.id, impact: v.impact ?? "-", nodes: v.nodes.length })),
    ...results.incomplete.map((v) => ({ loai: "incomplete", rule: v.id, impact: v.impact ?? "-", nodes: v.nodes.length })),
  ];
  if (rows.length > 0) {
    console.log(
      [
        `[axe] ${label} — khong chan (moderate/minor + incomplete):`,
        ...rows.map((r) => `  ${r.loai.padEnd(10)} ${r.rule.padEnd(32)} ${String(r.impact).padEnd(9)} ${r.nodes} node`),
      ].join("\n"),
    );
  }

  expect
    .soft(
      blocking.map((v) => `${v.id} (${v.impact}) x${v.nodes.length}`),
      formatBlocking(label, blocking),
    )
    .toEqual([]);
  return results;
}
