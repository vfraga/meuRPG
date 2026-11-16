import json,collections,re,sys
mo=json.load(open(sys.argv[1]))
inv=collections.defaultdict(list)
def add(kind,m): inv[kind].append(m['key'])
def usage(u):
  u=str(u).lower()
  if u.startswith('recharge') and 'rest' not in u: return u
  if 'rest' in u: return 'recharge-after-rest'
  return 'per-day'
def norm(n): return re.sub(r'\s*\(.*?\)','',n).strip()
for m in mo:
  for a in m.get('special_abilities') or []:
    add('trait:'+norm(a['name']).lower(),m)
    if a.get('usage'): add('usage:'+usage(a['usage']),m)
  for a in m.get('actions') or []:
    if a.get('multiattack'): add('action:multiattack',m); continue
    ut=usage(a['usage']) if a.get('usage') else None
    if a.get('has_attack') or a.get('attack_bonus') is not None:
      d=a['desc'].lower()
      shape='action:'+('melee' if 'melee' in d else 'ranged')+'-attack'
      if a.get('save'): shape+='+save'
      if 'grappled' in d: shape+='+grapple'
      if 'swallow' in d: shape+='+swallow'
      if 'prone' in d and 'knocked prone' in d: shape+='+prone'
    elif a.get('save'): shape='action:save-effect'+('+damage' if a.get('damage') else '')
    else: shape='action:other:'+norm(a['name']).lower()
    add(shape,m)
    if ut: add('action-usage:'+ut,m)
  for a in m.get('reactions') or []: add('reaction:'+norm(a['name']).lower(),m)
  if m.get('legendary_actions'):
    add('legendary:actions',m)
    for a in m['legendary_actions']:
      c=re.search(r'Costs (\d) Actions',a['name']); add('legendary:cost-'+(c.group(1) if c else '1'),m)
  for f in ('resistances','immunities','vulnerabilities'):
    for r in m.get(f) or []:
      s=json.dumps(r).lower()
      add(f'{f}:'+('nonmagical-clause' if 'nonmagical' in s else 'plain'),m)
  if m.get('condition_immunities'): add('condition_immunities',m)
  for s in ('blindsight','tremorsense','truesight','darkvision'):
    if m.get(s): add('sense:'+s,m)
  for k in (m.get('speed') or {}):
    if k!='walk': add('speed:'+k,m)
  if m.get('armor_class_alts'): add('ac:alternate',m)
out={k:{'count':len(set(v)),'examples':sorted(set(v))[:3]} for k,v in sorted(inv.items())}
json.dump(out,open(sys.argv[2],'w'),indent=1)
print(len(out))
