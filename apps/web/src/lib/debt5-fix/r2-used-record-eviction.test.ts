/** R-2 (AC-2 record rules, plan P-4): `ef:rt:used` keeps the 16 newest marks; the 17th mark drops the oldest,
 *  and re-marking a token moves it to the newest slot. */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import { newTab } from "../debt/harness";

beforeEach(() => setupEnv());
afterEach(() => resetEnv());

const t = (i: number) => "tok-" + i;

describe("R-2 ef:rt:used eviction order", () => {
  it("marking t0..t16 (17 marks) evicts t0 only; t1..t16 stay used", async () => {
    const { sess } = await newTab();
    for (let i = 0; i <= 16; i++) sess.markRefreshUsed(t(i));
    expect(sess.isRefreshUsed(t(0))).toBe(false);
    for (let i = 1; i <= 16; i++) expect(sess.isRefreshUsed(t(i))).toBe(true);
    expect(JSON.parse(sessionStorage.getItem("ef:rt:used") as string)).toHaveLength(16);
  });

  it("re-marking t1 moves it to newest: after t17, t1 is still used and t2 is evicted", async () => {
    const { sess } = await newTab();
    for (let i = 0; i <= 16; i++) sess.markRefreshUsed(t(i)); // t0 evicted, t1 oldest
    sess.markRefreshUsed(t(1));
    sess.markRefreshUsed(t(17));
    expect(sess.isRefreshUsed(t(1))).toBe(true);
    expect(sess.isRefreshUsed(t(2))).toBe(false);
    expect(sess.isRefreshUsed(t(17))).toBe(true);
    const arr = JSON.parse(sessionStorage.getItem("ef:rt:used") as string) as string[];
    expect(arr).toHaveLength(16);
    expect(new Set(arr).size).toBe(16);
  });

  it("re-marking an existing entry does not create a duplicate", async () => {
    const { sess } = await newTab();
    sess.markRefreshUsed(t(0));
    sess.markRefreshUsed(t(1));
    sess.markRefreshUsed(t(0));
    expect(JSON.parse(sessionStorage.getItem("ef:rt:used") as string)).toHaveLength(2);
  });
});
