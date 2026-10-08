// Third pass: resolve wrapper -> class; callers = files importing/injecting that class (or its interface) and calling `.wrapper(`.
import fs from 'node:fs'; import {execSync} from 'node:child_process';
const wr = JSON.parse(fs.readFileSync(process.argv[2],'utf8'));
const files = execSync(`find web/src/app -name '*.ts' ! -name '*.spec.ts' ! -name '*-testing.ts'`).toString().trim().split('\n');
const text = Object.fromEntries(files.map(f=>[f,fs.readFileSync(f,'utf8')]));
const res=[];
for (const w of wr) {
  const entry={rpc:w.rpc,sites:[]};
  for (const s of w.sites){
    const t=text[s.file]; const lines=t.split('\n');
    // enclosing class/function-factory: nearest previous `export class X` or `export function X`/`export const X`
    let cls=null; for(let j=s.line-1;j>=0;j--){const m=lines[j].match(/^export (?:abstract )?class (\w+)/); if(m){cls=m[1];break;}}
    const callers=new Set();
    const re=new RegExp(`\\.${s.wrapper}\\s*\\(|\\b${s.wrapper}\\s*[:=(]\\s*`);
    const reCall=new RegExp(`\\.${s.wrapper}\\s*\\(`);
    for(const f of files){ if(f===s.file) { // same-file use other than definition
        lines.forEach((l,i)=>{ if(reCall.test(l)&&i!==s.line-1&&!/^\s*(\/\/|\*)/.test(l)) callers.add(f+':'+(i+1)+' (same file)');}); continue;}
      const tx=text[f]; if(cls && !tx.includes(cls) && !/Port|Source|Api|Fetcher|Store/.test('')) {/* keep strict */}
      const lns=tx.split('\n');
      lns.forEach((l,i)=>{ if(reCall.test(l)&&!/^\s*(\/\/|\*)/.test(l)) { if(!cls||tx.includes(cls)||/Source|Api|port|Port/.test(tx)) callers.add(f+':'+(i+1)); }});
    }
    entry.sites.push({wrapper:s.wrapper,cls,file:s.file,line:s.line,callers:[...callers].slice(0,5),n:callers.size});
  }
  res.push(entry);
}
fs.writeFileSync(process.argv[3],JSON.stringify(res,null,1));
const weak=res.filter(r=>r.sites.length&&r.sites.every(s=>s.n===0));
console.log('weak (no caller of wrapper in a file mentioning its class):',weak.length);
for(const w of weak) console.log(w.rpc,w.sites.map(s=>`${s.cls}.${s.wrapper} @${s.file.replace('web/src/app/','')}:${s.line}`).join(' | '));
