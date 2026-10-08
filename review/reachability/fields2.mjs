// Check 2, second pass: fields whose name is declared by several messages (so a textual read may belong to another message).
// Lists, for each such non-request field, how many web files read that name; the few-hit ones are the candidates to verify in context.
import fs from 'node:fs'; import {execSync} from 'node:child_process';
const protos = execSync(`find proto/meurpg -name '*.proto'`).toString().trim().split('\n').sort();
const camel = s=>s.replace(/_(\w)/g,(_,c)=>c.toUpperCase());
const msgs={};
for (const f of protos){ const L=fs.readFileSync(f,'utf8').split('\n'); const stack=[];
  L.forEach((l,i)=>{ let m=l.match(/^\s*message\s+(\w+)\s*\{/); if(m){stack.push({name:m[1],depth:1}); msgs[m[1]]={file:f,fields:[]}; return;}
    if(stack.length){ const top=stack[stack.length-1]; const o=(l.match(/\{/g)||[]).length,c=(l.match(/\}/g)||[]).length;
      m=l.match(/^\s*(?:repeated\s+|optional\s+)?(?:map<[^>]+>|[\w.]+)\s+(\w+)\s*=\s*\d+/);
      if(m&&top.depth===1&&!/^\s*(rpc|enum|oneof|reserved|\/\/)/.test(l)) msgs[top.name].fields.push({name:camel(m[1]),line:i+1});
      top.depth+=o-c; if(top.depth<=0) stack.pop(); } });
}
const files=execSync(`find web/src/app \\( -name '*.ts' ! -name '*.spec.ts' ! -name '*-testing.ts' -o -name '*.html' \\)`).toString().trim().split('\n');
const texts=files.map(f=>[f,fs.readFileSync(f,'utf8')]);
const owners={}; for(const [mn,m] of Object.entries(msgs)) for(const f of m.fields) (owners[f.name]=owners[f.name]||[]).push(mn);
const out=[];
for(const [mn,m] of Object.entries(msgs)){ if(/Request$/.test(mn)) continue;
  for(const f of m.fields){ if(owners[f.name].length<2) continue;
    const re=new RegExp(`(\\.|\\?\\.|\\[['"]|[{,]\\s*)${f.name}\\b`);
    const hits=texts.filter(([p,t])=>re.test(t)).map(([p])=>p.replace('web/src/app/',''));
    out.push({msg:mn,field:f.name,file:m.file.replace('proto/meurpg/',''),line:f.line,sharedBy:owners[f.name].length,nFiles:hits.length,files:hits.slice(0,4)});
  }}
fs.writeFileSync(process.argv[2],JSON.stringify(out,null,1));
console.log('shared-name fields',out.length,'; with <=2 reading files:',out.filter(o=>o.nFiles<=2).length,'; <=1:',out.filter(o=>o.nFiles<=1).length);
