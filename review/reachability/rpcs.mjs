// Inventory: every RPC in proto/meurpg/**, with callers in web/src/app (non-spec).
import fs from 'node:fs'; import path from 'node:path'; import {execSync} from 'node:child_process';
const root = process.argv[2] ?? '.';
const protos = execSync(`find ${root}/proto/meurpg -name '*.proto'`).toString().trim().split('\n').sort();
const rows = [];
for (const f of protos) {
  const lines = fs.readFileSync(f,'utf8').split('\n'); let svc=null;
  lines.forEach((l,i)=>{
    let m=l.match(/^service\s+(\w+)/); if(m){svc=m[1];}
    m=l.match(/^\s*rpc\s+(\w+)\s*\(\s*(stream\s+)?([\w.]+)\s*\)\s*returns\s*\(\s*(stream\s+)?([\w.]+)/);
    if(m&&svc) rows.push({svc,rpc:m[1],req:m[3],res:m[5],stream:!!m[4],file:path.relative(root,f),line:i+1});
  });
}
const src = execSync(`grep -rn --include=*.ts -E '.' ${root}/web/src/app | grep -v '\\.spec\\.ts' | grep -v -- '-testing\\.ts'`,{maxBuffer:1<<28}).toString().split('\n');
const specSrc = execSync(`grep -rln --include=*.spec.ts -E '.' ${root}/web/src/app`,{maxBuffer:1<<28}).toString();
for (const r of rows) {
  const lc = r.rpc[0].toLowerCase()+r.rpc.slice(1);
  const re = new RegExp(`\\b(${lc}|${r.rpc})\\b`);
  r.callers = src.filter(l=>re.test(l)).map(l=>l.replace(root+'/web/src/app/','')).filter(l=>!/^\S+?:\d+:\s*(\/\/|\*)/.test(l)).slice(0,6);
}
fs.writeFileSync(process.argv[3] ?? 'rpcs.json', JSON.stringify(rows,null,1));
console.log(rows.length,'rpcs;', rows.filter(r=>!r.callers.length).length,'with no textual caller');
for (const r of rows.filter(r=>!r.callers.length)) console.log(r.svc, r.rpc, r.file+':'+r.line);
