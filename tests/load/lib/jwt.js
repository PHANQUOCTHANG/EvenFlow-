// HS256 access token encoder, byte-compatible with libs/go/authx.SignAccessToken (spec D1, AC-4).
// Pure: the HMAC is injected. In k6: (s) => crypto.hmac("sha256", secret, s, "base64rawurl");
// in node: (s) => createHmac("sha256", secret).update(s).digest("base64url").

export const ACCESS_ISSUER = "eventflow-identity";
export const ACCESS_AUDIENCE = "eventflow-api";

const B64URL = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";

function utf8Bytes(str) {
  const out = [];
  for (let i = 0; i < str.length; i++) {
    let c = str.charCodeAt(i);
    if (c >= 0xd800 && c <= 0xdbff && i + 1 < str.length) {
      const d = str.charCodeAt(i + 1);
      if (d >= 0xdc00 && d <= 0xdfff) {
        c = 0x10000 + ((c - 0xd800) << 10) + (d - 0xdc00);
        i++;
      }
    }
    if (c < 0x80) out.push(c);
    else if (c < 0x800) out.push(0xc0 | (c >> 6), 0x80 | (c & 63));
    else if (c < 0x10000) out.push(0xe0 | (c >> 12), 0x80 | ((c >> 6) & 63), 0x80 | (c & 63));
    else out.push(0xf0 | (c >> 18), 0x80 | ((c >> 12) & 63), 0x80 | ((c >> 6) & 63), 0x80 | (c & 63));
  }
  return out;
}

// base64url without padding (RFC 7515) of the UTF-8 bytes of str.
export function base64url(str) {
  const b = utf8Bytes(str);
  let out = "";
  for (let i = 0; i < b.length; i += 3) {
    const n = (b[i] << 16) | ((b[i + 1] || 0) << 8) | (b[i + 2] || 0);
    out += B64URL[(n >> 18) & 63] + B64URL[(n >> 12) & 63];
    if (i + 1 < b.length) out += B64URL[(n >> 6) & 63];
    if (i + 2 < b.length) out += B64URL[n & 63];
  }
  return out;
}

// Claims in the key order Go's encoding/json emits for authx.accessJWT
// (role, typ, ver, did, then RegisteredClaims: iss, sub, aud, exp, iat, jti).
export function accessClaims({ sub, did, jti, iatS, expS, role = "buyer" }) {
  for (const [k, v] of [["sub", sub], ["did", did], ["jti", jti], ["role", role]]) {
    if (typeof v !== "string" || v === "") throw new Error(`jwt: claim ${k} is required`);
  }
  for (const [k, v] of [["iatS", iatS], ["expS", expS]]) {
    if (!Number.isInteger(v) || v <= 0) throw new Error(`jwt: claim ${k} must be a positive integer (seconds)`);
  }
  if (expS <= iatS) throw new Error("jwt: exp must be after iat");
  return {
    role,
    typ: "access",
    ver: true,
    did,
    iss: ACCESS_ISSUER,
    sub,
    aud: [ACCESS_AUDIENCE],
    exp: expS,
    iat: iatS,
    jti,
  };
}

const HEADER = base64url(JSON.stringify({ alg: "HS256", typ: "JWT" }));

// sign(signingInput) must return the base64url (no padding) HMAC-SHA256 of signingInput.
export function encodeJwt(claims, sign) {
  const input = `${HEADER}.${base64url(JSON.stringify(claims))}`;
  return `${input}.${sign(input)}`;
}

// exp for a token that must outlive the whole run: run end + 5 min safety (AC-4, AC-9).
export function accessTokenExpS(nowS, runSeconds) {
  if (!Number.isFinite(runSeconds) || runSeconds < 0) throw new Error("jwt: run seconds must be >= 0");
  return Math.floor(nowS) + Math.ceil(runSeconds) + 300 + 60;
}
