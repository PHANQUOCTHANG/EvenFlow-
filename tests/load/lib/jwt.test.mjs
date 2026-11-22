// AC-4 / AC-12: access token encoder must produce the same bytes as libs/go/authx.SignAccessToken.
import test from "node:test";
import assert from "node:assert/strict";
import { createHmac } from "node:crypto";
import { readFileSync } from "node:fs";
import { accessClaims, encodeJwt, base64url, accessTokenExpS } from "./jwt.js";

const fixture = JSON.parse(readFileSync(new URL("./fixtures/access-token.json", import.meta.url), "utf8"));
const signer = (secret) => (input) => createHmac("sha256", secret).update(input).digest("base64url");

function decodePart(part) {
  return JSON.parse(Buffer.from(part, "base64url").toString("utf8"));
}

test("token matches the fixture signed by Go authx.SignAccessToken", () => {
  const tok = encodeJwt(accessClaims(fixture.claims), signer(fixture.secret));
  assert.equal(tok, fixture.token);
});

test("claims carry iss, aud, typ, ver, did, role, sub, jti, iat, exp", () => {
  const tok = encodeJwt(accessClaims(fixture.claims), signer(fixture.secret));
  const [h, p, s] = tok.split(".");
  assert.deepEqual(decodePart(h), { alg: "HS256", typ: "JWT" });
  const c = decodePart(p);
  assert.equal(c.iss, "eventflow-identity");
  assert.deepEqual(c.aud, ["eventflow-api"]);
  assert.equal(c.typ, "access");
  assert.equal(c.ver, true);
  assert.equal(c.did, fixture.claims.did);
  assert.equal(c.role, "buyer");
  assert.equal(c.sub, fixture.claims.sub);
  assert.equal(c.jti, fixture.claims.jti);
  assert.equal(c.iat, fixture.claims.iatS);
  assert.equal(c.exp, fixture.claims.expS);
  assert.match(s, /^[A-Za-z0-9_-]{43}$/);
});

test("role defaults to buyer and a wrong secret changes the signature", () => {
  const { role, ...rest } = fixture.claims;
  assert.equal(role, "buyer");
  const tok = encodeJwt(accessClaims(rest), signer("another-secret-0123456789"));
  assert.equal(tok.split(".")[1], fixture.token.split(".")[1]);
  assert.notEqual(tok, fixture.token);
});

test("accessClaims rejects missing sub, did, jti or times", () => {
  for (const k of ["sub", "did", "jti", "iatS", "expS"]) {
    const c = { ...fixture.claims };
    delete c[k];
    assert.throws(() => accessClaims(c), new RegExp(k));
  }
  assert.throws(() => accessClaims({ ...fixture.claims, expS: fixture.claims.iatS }), /exp/);
});

test("base64url has no padding and handles UTF-8", () => {
  for (const s of ["", "a", "ab", "abc", "{\"x\":\"đăng ký\"}", "~~~???"]) {
    assert.equal(base64url(s), Buffer.from(s, "utf8").toString("base64url"));
  }
});

test("token expiry is at least run end + 5 minutes (AC-4, AC-9)", () => {
  const now = 1767225600;
  assert.equal(accessTokenExpS(now, 1800) >= now + 1800 + 300, true);
  assert.equal(accessTokenExpS(now, 60) >= now + 60 + 300, true);
  assert.throws(() => accessTokenExpS(now, -1), /run/);
});
