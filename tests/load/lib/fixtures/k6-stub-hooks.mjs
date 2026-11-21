// Node module hooks that replace k6/* imports with in-memory stubs, so main-wiring.test.mjs can load main.js
// without k6. The stubs read globalThis.__http (fake gateway) / globalThis.__exec and record metric samples in
// globalThis.__m.
const stubs = {
  k6: "export const check=(v,c)=>{let ok=true;for(const k in c) ok=!!c[k](v)&&ok;return ok}; export const sleep=()=>{};",
  "k6/crypto":
    "import {createHmac} from 'node:crypto'; export default {hmac:(a,k,d,e)=>{if(e!=='base64rawurl')throw new Error('encoding '+e);return createHmac(a,k).update(d).digest('base64url')}};",
  // modules are cached by node: delegate to the current globals so each test can swap them
  "k6/execution":
    "export default {get scenario(){return globalThis.__exec.scenario}, get vu(){return globalThis.__exec.vu}};",
  "k6/http":
    "const g=()=>globalThis.__http; export default {get:(...a)=>g().get(...a), post:(...a)=>g().post(...a), setResponseCallback:(...a)=>g().setResponseCallback(...a), expectedStatuses:(...a)=>g().expectedStatuses(...a)};",
  "k6/metrics":
    "class M{constructor(n){(globalThis.__m||(globalThis.__m={}))[n]=this.v=[]} add(x){this.v.push(x)}} export class Counter extends M{}; export class Rate extends M{}; export class Trend extends M{};",
};

export async function resolve(spec, ctx, next) {
  if (Object.prototype.hasOwnProperty.call(stubs, spec)) return { url: `k6stub:${spec}`, shortCircuit: true };
  return next(spec, ctx);
}

export async function load(url, ctx, next) {
  if (url.startsWith("k6stub:")) return { format: "module", source: stubs[url.slice(7)], shortCircuit: true };
  if (/\/tests\/load\/.*\.js(\?|$)/.test(url)) {
    const r = await next(url, { ...ctx, format: "module" });
    return { ...r, format: "module" };
  }
  return next(url, ctx);
}
