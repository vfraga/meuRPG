// Check 2 (precise): for every property of every generated message type, ask the TypeScript language service for its references
// in web/src/app (non-spec, non-testing). Properties with zero reads are listed with where the name still appears as text (templates).
// usage: node tsrefs.cjs <repo> <out.json>
const ts = require('/opt/node-tools/node_modules/typescript');
const fs = require('fs'), path = require('path'), cp = require('child_process');
const repo = path.resolve(process.argv[2]);
const sh = c => cp.execSync(c, {cwd: repo, maxBuffer: 1 << 28}).toString().trim().split('\n').filter(Boolean);
const gen = sh("find web/src/gen -name '*_pb.ts'").map(f => path.join(repo, f));
const app = sh("find web/src/app -name '*.ts' ! -name '*.spec.ts' ! -name '*-testing.ts'").map(f => path.join(repo, f));
const files = [...gen, ...app];
const opts = {target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext, moduleResolution: ts.ModuleResolutionKind.Bundler, strict: true, skipLibCheck: true, noResolve: false, experimentalDecorators: true, allowJs: false, types: []};
const host = {
  getScriptFileNames: () => files, getScriptVersion: () => '1', getCurrentDirectory: () => repo, getCompilationSettings: () => opts,
  getScriptSnapshot: f => fs.existsSync(f) ? ts.ScriptSnapshot.fromString(fs.readFileSync(f, 'utf8')) : undefined,
  fileExists: ts.sys.fileExists, readFile: ts.sys.readFile, readDirectory: ts.sys.readDirectory, directoryExists: ts.sys.directoryExists, getDefaultLibFileName: o => ts.getDefaultLibFilePath(o),
};
const ls = ts.createLanguageService(host, ts.createDocumentRegistry());
const prog = ls.getProgram();
const appSet = new Set(app);
const textApp = sh("find web/src/app -name '*.html' -o -name '*.ts' ! -name '*.spec.ts' ! -name '*-testing.ts'").map(f => [f, fs.readFileSync(path.join(repo, f), 'utf8')]);
const out = []; let total = 0;
for (const g of gen) {
  const sf = prog.getSourceFile(g); if (!sf) continue;
  ts.forEachChild(sf, node => {
    if (!ts.isTypeAliasDeclaration(node) || !/^[A-Z]/.test(node.name.text) || /Request$/.test(node.name.text)) return;
    // type X = Message<"..."> & { fields }
    const lit = (function find(n) { if (ts.isTypeLiteralNode(n)) return n; let r; ts.forEachChild(n, c => { r = r || find(c); }); return r; })(node.type);
    if (!lit) return;
    for (const m of lit.members) {
      if (!ts.isPropertySignature(m) || !m.name) continue;
      total++;
      const name = m.name.getText(sf);
      const refs = ls.findReferences(g, m.name.getStart(sf)) || [];
      let reads = 0;
      for (const r of refs) for (const ref of r.references) { if (ref.isDefinition) continue; if (appSet.has(ref.fileName)) reads++; }
      if (reads === 0) {
        const re = new RegExp('(\\.|\\?\\.|[\'"\\[{,]\\s*)' + name + '\\b');
        const hits = textApp.filter(([f, t]) => re.test(t)).map(([f]) => f.replace('web/src/app/', ''));
        out.push({msg: node.name.text, field: name, line: sf.getLineAndCharacterOfPosition(m.getStart(sf)).line + 1, file: path.relative(repo, g), textHits: hits.slice(0, 6), nHits: hits.length});
      }
    }
  });
}
fs.writeFileSync(process.argv[3], JSON.stringify(out, null, 1));
console.log('properties checked (non-request messages):', total, '; zero TS reads:', out.length, '; of which no text hit anywhere:', out.filter(o => !o.nHits).length);
