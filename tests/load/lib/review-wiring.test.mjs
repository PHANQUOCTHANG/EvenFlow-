// Review round 1, main.js wiring (R-6 FEW_LEFT is sellable, R-7 run id never falls back to a shared name).
import test from "node:test";
import assert from "node:assert/strict";
import { register } from "node:module";

register("./fixtures/k6-stub-hooks.mjs", import.meta.url);
const MAIN = new URL("../main.js", import.meta.url).href;

function gateway(level) {
  const r = (status, body) => ({ status, body: JSON.stringify(body), headers: {}, timings: { duration: 1 } });
  return {
    setResponseCallback() {},
    expectedStatuses(o) {
      return o;
    },
    get(url) {
      if (url.endsWith("/healthz")) return r(200, {});
      if (url.endsWith("/availability")) return r(200, { tiers: [{ ticket_type_id: "tt-1", availability: level }] });
      return r(404, {});
    },
    post() {
      return r(404, {});
    },
  };
}

let seq = 0;
async function loadMain(env, level = "AVAILABLE") {
  globalThis.__ENV = env;
  globalThis.__http = gateway(level);
  globalThis.__exec = { scenario: { iterationInTest: 0, startTime: Date.now() }, vu: { idInTest: 1 } };
  return import(`${MAIN}?rw=${++seq}`);
}

test("R-6: setup accepts FEW_LEFT and rejects SOLD_OUT", async () => {
  const few = await loadMain({ PROFILE: "smoke" }, "FEW_LEFT");
  assert.equal(few.setup().ticketType, "tt-1");
  const sold = await loadMain({ PROFILE: "smoke" }, "SOLD_OUT");
  assert.throws(() => sold.setup(), /not sellable after 30 s \(SOLD_OUT\)/);
});

test("R-7: without RUN_ID and without setup_data the result file still gets a unique run id", async () => {
  const mod = await loadMain({ PROFILE: "smoke" });
  const keys = Object.keys(mod.handleSummary({ metrics: {} })).filter((k) => k.endsWith(".json"));
  assert.equal(keys.length, 1);
  assert.match(keys[0], /^tests\/load\/results\/smoke-r[0-9a-z]+\.json$/);
  assert.ok(!keys[0].includes("norun"));
  const data = mod.setup();
  assert.equal(Object.keys(mod.handleSummary({ metrics: {} }))[1], `tests/load/results/smoke-${data.runId}.json`);
});

test("R-7: RUN_ID wins when given", async () => {
  const mod = await loadMain({ PROFILE: "smoke", RUN_ID: "rfix" });
  assert.equal(mod.setup().runId, "rfix");
  assert.ok("tests/load/results/smoke-rfix.json" in mod.handleSummary({ metrics: {} }));
});
