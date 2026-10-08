// Second hop: for each RPC call site `<x>.rpcName(` find the enclosing wrapper method and who calls that wrapper.
import fs from 'node:fs'; import {execSync} from 'node:child_process';
const rpcs = JSON.parse(fs.readFileSync(process.argv[2],'utf8'));
const files = execSync(`find web/src/app -name '*.ts' ! -name '*.spec.ts' ! -name '*-testing.ts'`).toString().trim().split('\n');
const text = Object.fromEntries(files.map(f=>[f,fs.readFileSync(f,'utf8').split('\n')]));
const out=[];
for (const r of rpcs) {
  const lc=r.rpc[0].toLowerCase()+r.rpc.slice(1);
  const re=new RegExp(`\\.${lc}\\s*\\(`);
  const sites=[];
  for (const f of files) text[f].forEach((l,i)=>{ if(re.test(l)&&!/^\s*(\/\/|\*)/.test(l)) {
    // enclosing method: nearest previous line matching class-method def at 2-space indent
    let name=null; for(let j=i;j>=0;j--){const m=text[f][j].match(/^  (?:private |protected |public |static |readonly |async )*(\w+)\s*(?:<[^>]*>)?\s*\(.*\)?.*[{]?\s*$/) ; const m2=text[f][j].match(/^  (?:async )?(\w+)\(/); if(m2&&!['if','for','while','switch','constructor'].includes(m2[1])){name=m2[1];break;} const m3=text[f][j].match(/^  (?:readonly |private )?(\w+)\s*=\s*(?:async )?\(/); if(m3){name=m3[1];break;} if(/^export (async )?function (\w+)/.test(text[f][j])){name=text[f][j].match(/function (\w+)/)[1];break;}}
    sites.push({file:f,line:i+1,wrapper:name});
  }});
  const users=[];
  for (const s of sites){ if(!s.wrapper){users.push('?'); continue;}
    const re2=new RegExp(`\\b${s.wrapper}\\b`);
    let n=0; const where=new Set();
    for(const f of files){ text[f].forEach((l,i)=>{ if(re2.test(l)&&!(f===s.file&&i+1===s.line)&&!/^\s*(\/\/|\*|\/\*)/.test(l)&&!(f===s.file&&new RegExp(`^  (async )?${s.wrapper}\\(`).test(l))){n++;where.add(f+':'+(i+1));}});}
    users.push({wrapper:s.wrapper,n,where:[...where].slice(0,4)});
  }
  out.push({rpc:`${r.svc}.${r.rpc}`,sites,users});
}
fs.writeFileSync(process.argv[3],JSON.stringify(out,null,1));
const weak=out.filter(o=>!o.sites.length||o.users.every(u=>u==='?'||u.n===0));
console.log(out.length,'rpcs; weak:',weak.length);
for(const w of weak) console.log(w.rpc, JSON.stringify(w.sites.map(s=>s.file.replace('web/src/app/','')+':'+s.line+' '+s.wrapper)), JSON.stringify(w.users.map(u=>u.n)));
