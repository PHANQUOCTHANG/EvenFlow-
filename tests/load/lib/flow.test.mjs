// AC-4 (identities, remote guard), AC-5 (poll delay rule), AC-6 (idempotency key) / AC-12.
import test from "node:test";
import assert from "node:assert/strict";
import { identityFor, deviceFor, idemKey, nextPollDelayMs, guardTarget, shouldReplay } from "./flow.js";
import { mulberry32 } from "./rng.js";

test("identityFor uses the 9000 range with a 12-digit decimal suffix (matches seed-load.sh)", () => {
  assert.equal(identityFor(1), "00000000-0000-4000-9000-000000000001");
  assert.equal(identityFor(123456), "00000000-0000-4000-9000-000000123456");
  for (const bad of [0, -1, 1.5, NaN, 1e12]) assert.throws(() => identityFor(bad), /identity/);
});

test("deviceFor is stable, distinct per identity and a valid anti-bot fp", () => {
  assert.equal(deviceFor(1), deviceFor(1));
  assert.notEqual(deviceFor(1), deviceFor(2));
  assert.match(deviceFor(42), /^[A-Za-z0-9-]{8,64}$/);
});

test("idemKey is lt-{RUN_ID}-{identity}-{step} and a valid Idempotency-Key", () => {
  const id = identityFor(7);
  const k = idemKey("r20261122a", id, "hold");
  assert.equal(k, `lt-r20261122a-${id}-hold`);
  assert.ok(k.length <= 255);
  assert.match(k, /^[\x21-\x7e]+$/);
  assert.equal(idemKey("r1", id, "pay"), `lt-r1-${id}-pay`);
  assert.equal(idemKey("r1", id, "issue"), `lt-r1-${id}-issue`);
});

test("idemKey rejects bad run ids, identities and steps", () => {
  const id = identityFor(7);
  assert.throws(() => idemKey("", id, "hold"), /RUN_ID/);
  assert.throws(() => idemKey("has space", id, "hold"), /RUN_ID/);
  assert.throws(() => idemKey("x".repeat(65), id, "hold"), /RUN_ID/);
  assert.throws(() => idemKey("r1", "not-a-uuid", "hold"), /identity/);
  assert.throws(() => idemKey("r1", id, "refund"), /step/);
});

const zeroRng = () => 0;

test("200: sleeps poll_after_ms from the body", () => {
  assert.equal(nextPollDelayMs({ status: 200, headers: {}, body: { poll_after_ms: 2500 } }, { rng: zeroRng }), 2500);
});

test("304: sleeps X-Poll-After-Ms (header lookup is case-insensitive)", () => {
  assert.equal(nextPollDelayMs({ status: 304, headers: { "X-Poll-After-Ms": "4000" }, body: null }, { rng: zeroRng }), 4000);
  assert.equal(nextPollDelayMs({ status: 304, headers: { "x-poll-after-ms": "3000" }, body: null }, { rng: zeroRng }), 3000);
});

test("429: sleeps Retry-After seconds", () => {
  assert.equal(nextPollDelayMs({ status: 429, headers: { "Retry-After": "2" }, body: null }, { rng: zeroRng }), 2000);
});

test("missing or invalid server value: floor 1000 ms", () => {
  assert.equal(nextPollDelayMs({ status: 200, headers: {}, body: {} }, { rng: zeroRng }), 1000);
  assert.equal(nextPollDelayMs({ status: 200, headers: {}, body: { poll_after_ms: -5 } }, { rng: zeroRng }), 1000);
  assert.equal(nextPollDelayMs({ status: 304, headers: { "X-Poll-After-Ms": "abc" }, body: null }, { rng: zeroRng }), 1000);
  assert.equal(nextPollDelayMs({ status: 503, headers: {}, body: null }, { rng: zeroRng }), 1000);
});

test("jitter is added on top and the delay is never below the server value", () => {
  const rng = mulberry32(7);
  for (let i = 0; i < 500; i++) {
    const d = nextPollDelayMs({ status: 200, headers: {}, body: { poll_after_ms: 1500 } }, { jitterMaxMs: 250, rng });
    assert.ok(d >= 1500 && d <= 1750, `delay ${d}`);
    assert.equal(Number.isInteger(d), true);
  }
});

test("guardTarget allows local targets", () => {
  for (const b of ["http://localhost:8080", "http://127.0.0.1:8080/", "https://localhost", "http://[::1]:8080"]) {
    assert.doesNotThrow(() => guardTarget(b, {}), b);
  }
});

test("guardTarget refuses remote targets unless ALLOW_REMOTE=1", () => {
  for (const b of ["http://staging.example.com", "http://10.0.0.5:8080", "http://localhost.evil.com", "https://127.0.0.1.nip.io"]) {
    assert.throws(() => guardTarget(b, {}), /ALLOW_REMOTE/, b);
    assert.throws(() => guardTarget(b, { ALLOW_REMOTE: "true" }), /ALLOW_REMOTE/, b);
    assert.doesNotThrow(() => guardTarget(b, { ALLOW_REMOTE: "1" }), b);
  }
});

test("guardTarget rejects malformed BASE", () => {
  for (const b of ["", "localhost:8080", "ftp://localhost", "http://"]) {
    assert.throws(() => guardTarget(b, { ALLOW_REMOTE: "1" }), /BASE/, b);
  }
});

test("shouldReplay samples about REPLAY_RATIO", () => {
  const rng = mulberry32(1);
  let n = 0;
  for (let i = 0; i < 10000; i++) if (shouldReplay(rng, 0.1)) n++;
  assert.ok(n > 800 && n < 1200, `replays ${n}`);
  assert.equal(shouldReplay(rng, 0), false);
  assert.equal(shouldReplay(() => 0.999, 1), true);
});

test("mulberry32 is deterministic per seed and in [0,1)", () => {
  const a = mulberry32(99), b = mulberry32(99);
  for (let i = 0; i < 100; i++) {
    const x = a();
    assert.equal(x, b());
    assert.ok(x >= 0 && x < 1);
  }
});
