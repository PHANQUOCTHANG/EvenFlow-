// usage: node apply.js <webdir> <id>
const fs=require('fs'),path=require('path');
const [web,id]=process.argv.slice(2);
const A='src/lib/api-client.ts',S='src/lib/session.ts',P='src/lib/payment-client.ts',H='src/hooks/use-hold-timer.ts',V='src/components/checkout/checkout-view.tsx',R='src/components/checkout/order-result-view.tsx';
const M={
'M-1':[A,'if (isRefreshUsed(cur.refreshToken)) {','if (false) {'],
'M-2':[S,'if (!held) continue;','if (false) continue;'],
'M-3':[A,'    // Mat mang: giu phien, de lan sau thu lai.\n    throw networkError();','    markRefreshUsed(refreshToken);\n    throw networkError();'],
'M-4':[A,'  try {\n    raw = await res.json();\n  } catch (err) {\n    if (controller.signal.aborted) throw err;\n    return failedRefresh(epoch, cur);\n  }','  raw = await res.json();'],
'M-5':[A,'    stored.user.id === cur.user.id &&\n',''],
'M-6':[A,'    epoch === getSessionEpoch() &&\n    stored.user.id','    stored.user.id'],
'M-7':[A,'if (!session && startUserId !== null) throw','if (false) throw'],
'M-8':[A,'typeof token !== "string" || !token','!token'],
'M-8b':[A,'    if (typeof token !== "string" || !token) throw networkError();\n',''],
'M-9':[S,'typeof user.id !== "string" || !isRole(user.role)','typeof user.id !== "string" || typeof user.role !== "string"'],
'M-10':[P,'protocol === "http:" || protocol === "https:" ? raw : fallback','raw'],
'M-11':[A,'  if (isRefreshUsed(cur.refreshToken)) {\n    clearSessionHoldingRefresh(cur.refreshToken);\n    throw new ApiError("AUTH_REQUIRED", 401, {}, "Phien dang nhap da het han");\n  }\n  if (usedRefresh && cur.refreshToken !== usedRefresh) return cur.accessToken;','  if (usedRefresh && cur.refreshToken !== usedRefresh) return cur.accessToken;\n  if (isRefreshUsed(cur.refreshToken)) {\n    clearSessionHoldingRefresh(cur.refreshToken);\n    throw new ApiError("AUTH_REQUIRED", 401, {}, "Phien dang nhap da het han");\n  }'],
'M-12':[S,'if (activeWiped) sessionEpoch += 1;','if (wiped) sessionEpoch += 1;'],
'M-13':[S,'if (read.kind !== "absent") return { kind, read };','if (read.kind === "ok") return { kind, read };'],
'M-14':[S,'prev.id === id ? prev.role','true ? prev.role'],
'M-15':[A,'if (res.ok || res.status === 400 || res.status === 401 || res.status === 403) markRefreshUsed','if (res.status === 400 || res.status === 401 || res.status === 403) markRefreshUsed'],
'M-16':[A,'      markRefreshUsed(refreshToken);\n      return failedRefresh(epoch, cur);','      return failedRefresh(epoch, cur);'],
'M-17':[A,'    stored.refreshToken !== cur.refreshToken &&\n',''],
'M-18':[A,'    if (controller.signal.aborted) throw err;\n    return failedRefresh(epoch, cur);','    return failedRefresh(epoch, cur);'],
'E-1':[S,'while (list.length > REFRESH_USED_MAX) list.shift();','while (list.length > REFRESH_USED_MAX) list.pop();'],
'E-2':[S,'const list = readRefreshUsed().filter((x) => x !== h);','const list = readRefreshUsed();'],
'W-1':[V,'                setCancelError(null);\n                setConfirmOpen(true);','                setCancelError(null);\n                void handleConfirmCancel();'],
'W-2':[V,'    if (!holdActive) {\n      setConfirmOpen(false);','    if (!holdActive) {'],
'W-2b':[V,['    if (!holdActive) {\n      setConfirmOpen(false);','    if (!holdActive) {'],['{confirmOpen && holdActive ? (','{confirmOpen ? (']],
'W-3':[R,'const authExpired = error?.type === "AUTH_REQUIRED";','const authExpired = error?.type === "AUTH_REQUIRED" && order !== null;'],
'W-4':[R,'const ticketsAuthRequired = issued.authRequired && !(showErrorCard && authExpired);','const ticketsAuthRequired = false;'],
'W-5':[H,'      await cancelHold(current.holdId);','      resetHold();\n      await cancelHold(current.holdId);'],
};
const m=M[id]; if(!m){console.error('no mutant');process.exit(2);}
const f=path.join(web,m[0]); let s=fs.readFileSync(f,'utf8'); const eol=s.includes('\r\n');
if(eol) s=s.replace(/\r\n/g,'\n');
const pairs=Array.isArray(m[1])?m.slice(1):[[m[1],m[2]]];
for(const [a,b] of pairs){ const n=s.split(a).length-1; if(n!==1){console.error(id+': match count '+n);process.exit(3);} s=s.replace(a,()=>b); }
if(eol) s=s.replace(/\n/g,'\r\n');
fs.writeFileSync(f,s); console.log('applied '+id);
