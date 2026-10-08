// Check 2: every field of every message that is a RESPONSE or reachable from one: is its camelCase name read anywhere in web/src/app (non-spec, non-testing)?
// "read" = appears as `.name`, `?.name`, `['name']`, or destructured `{ name`. Request-only messages are skipped.
import fs from 'node:fs'; import {execSync} from 'node:child_process';
const protos = execSync(`find proto/meurpg -name '*.proto'`).toString().trim().split('\n').sort();
const camel = s=>s.replace(/_(\w)/g,(_,c)=>c.toUpperCase());
const msgs={}; // name -> {file, fields:[{name,type,line}] }
for (const f of protos){ const L=fs.readFileSync(f,'utf8').split('\n'); const stack=[];
  L.forEach((l,i)=>{ let m=l.match(/^\s*message\s+(\w+)\s*\{/); if(m){stack.push({name:m[1],depth:1}); msgs[m[1]]={file:f,fields:[],line:i+1}; return;}
    if(stack.length){ const top=stack[stack.length-1];
      const opens=(l.match(/\{/g)||[]).length, closes=(l.match(/\}/g)||[]).length;
      m=l.match(/^\s*(?:repeated\s+|optional\s+)?(?:map<[^>]+>|[\w.]+)\s+(\w+)\s*=\s*\d+/);
      if(m&&top.depth===1&&!/^\s*(rpc|enum|oneof|reserved|\/\/)/.test(l)) { const t=l.trim().split(/\s+/); msgs[top.name].fields.push({name:camel(m[1]),proto:m[1],line:i+1,decl:l.trim().slice(0,90)}); }
      top.depth+=opens-closes; if(top.depth<=0) stack.pop(); } });
}
const files=execSync(`find web/src/app \\( -name '*.ts' ! -name '*.spec.ts' ! -name '*-testing.ts' -o -name '*.html' \\)`).toString().trim().split('\n');
const all=files.map(f=>fs.readFileSync(f,'utf8')).join('\n');
const out=[]; let total=0;
for(const [mn,m] of Object.entries(msgs)){
  if(/Request$/.test(mn)) continue;
  for(const fd of m.fields){ total++;
    const re=new RegExp(`(\\.|\\?\\.|\\[['"]|[{,]\\s*)${fd.name}\\b`);
    if(!re.test(all)) out.push({msg:mn,field:fd.name,file:m.file,line:fd.line,decl:fd.decl});
  }
}
fs.writeFileSync(process.argv[2],JSON.stringify(out,null,1));
console.log('fields checked (non-request messages):',total,'; never read:',out.length);
