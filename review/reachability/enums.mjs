// Checks 4+6: every enum value in proto/meurpg/**; is `EnumTsName.VALUE` referenced in web/src/app (non-spec, non-testing)?
// Output per enum: values with no reference. Whole-enum zero refs is listed separately (enum maybe handled via string keys / numeric maps).
import fs from 'node:fs'; import {execSync} from 'node:child_process';
const protos = execSync(`find proto/meurpg -name '*.proto'`).toString().trim().split('\n').sort();
const files=execSync(`find web/src/app -name '*.ts' ! -name '*.spec.ts' ! -name '*-testing.ts'`).toString().trim().split('\n');
const all=files.map(f=>fs.readFileSync(f,'utf8')).join('\n');
const enums=[];
for(const f of protos){ const L=fs.readFileSync(f,'utf8').split('\n'); let cur=null;
  L.forEach((l,i)=>{ let m=l.match(/^\s*enum\s+(\w+)\s*\{/); if(m){cur={name:m[1],file:f,line:i+1,values:[]};enums.push(cur);return;}
    if(cur){ if(/^\s*\}/.test(l)){cur=null;return;} m=l.match(/^\s*([A-Z][A-Z0-9_]*)\s*=\s*(\d+)/); if(m) cur.values.push({proto:m[1],num:+m[2],line:i+1}); } });
}
const strip=(e,v)=>{ // protobuf-es strips the enum-name prefix (SNAKE of enum name)
  const pre=e.replace(/([a-z0-9])([A-Z])/g,'$1_$2').toUpperCase()+'_'; return v.startsWith(pre)?v.slice(pre.length):v; };
const res=[]; let nv=0;
for(const e of enums){ const out={enum:e.name,file:e.file.replace('proto/meurpg/',''),line:e.line,total:e.values.length,refs:0,missing:[]};
  const tsRe=new RegExp(`\\b${e.name}\\b`); out.mentioned=tsRe.test(all);
  for(const v of e.values){ nv++; const n=strip(e.name,v.proto); if(n==='UNSPECIFIED') continue;
    const re=new RegExp(`\\w*${e.name}\\.${n}\\b`); if(re.test(all)) out.refs++; else out.missing.push(n); }
  res.push(out);
}
fs.writeFileSync(process.argv[2],JSON.stringify(res,null,1));
console.log('enums',enums.length,'values',nv);
console.log('enums never mentioned in web:',res.filter(r=>!r.mentioned).map(r=>r.enum).join(', '));
