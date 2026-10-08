#!/usr/bin/env python3
"""Merges spells-machine.json (what the cast pipeline reads, from overlay/zz_spells_dump_test.go) with
the haiku tags (raw/spell-tags-b*.jsonl) and the hand overrides below, and writes spells-states.json + spells-states.csv.

State rules (judged by hand against play/combat_spells.go, see the report):
  absent   : the cast RPC refuses it (casting time of 1 minute or more, not one of the 3 summons) or a reaction other than Shield.
  built    : the cast resolves everything the text asks of a fight (attack / save+damage / heal / HP effect / summon).
  partial  : the cast rolls something real (attack, save, damage, heal, HP effect) but the tags show a rider the app does not apply.
  reminder : slot, action and concentration flag are spent and the log says "cast"; nothing is computed ("o mestre resolve o efeito").
"""
import json, glob, csv, os, collections
root = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..')
mach = {r['key']: r for r in json.load(open(os.path.join(root, 'spells-machine.json')))}
tags = {}
for f in sorted(glob.glob(os.path.join(root, 'raw/spell-tags-b*.jsonl'))):
    for l in open(f):
        if l.strip():
            r = json.loads(l); tags[r['key']] = r
data = {s['key']: s for s in json.load(open('/home/user/meuRPG/backend/internal/rules/srd51/data/spells.json'))}
names = json.load(open('/home/user/meuRPG/backend/internal/rules/srd51/effects/names_pt.json'))['names']
missing_tags = [k for k in mach if k not in tags]
if missing_tags: print('NO TAGS for', len(missing_tags), missing_tags[:5])

# needs that count as "a rider the app does not apply"
GAP = {'repeat_save': 'a later saving throw is not run', 'ongoing_damage': 'ongoing / later damage is not run',
       'movement': 'movement, speed or teleport is not applied', 'stat_change': 'bonus, penalty, advantage or resistance is not applied',
       'zone': 'the area / wall / object is not put on the map', 'rider': 'a second effect on hit or failed save is not applied'}
OVERRIDES = json.load(open(os.path.join(root, 'scripts/spell-overrides.json'))) if os.path.exists(os.path.join(root, 'scripts/spell-overrides.json')) else {}

rows = []
for k in sorted(mach):
    m, t = mach[k], tags.get(k, {'kind': '?', 'conditions': [], 'needs': [], 'note': '', 'evidence': ''})
    d = data[k]
    fight = m['castable_in_fight']
    what = []   # what the cast really computes
    if m['summon']: what.append('summon')
    if m['hp_effect']: what.append('hp_effect:' + m['hp_effect'])
    if m['darts']: what.append('darts')
    if fight and not m['caster_only']:
        if m['attack_type']: what.append('attack_roll')
        if m['save_ability']: what.append('save_roll')
    if (m['damage_parsed'] or m['darts']) and (m['attack_type'] or (m['save_ability'] and not m['caster_only']) or m['darts']): what.append('damage')
    elif m['damage_parsed'] and not m['hp_effect']: what.append('damage_table_only')  # shown to the master, rolled by nobody
    if m['heal'] and not m['hp_effect']: what.append('heal')
    if m['damage_choice']: what.append('damage_type_choice')
    if m['damage_upcasts'] or m['heal_upcasts']: what.append('upcast_dice')
    if m['concentration']: what.append('concentration_flag')
    if m['area_any_number'] and not m['caster_only']: what.append('area_targets_picked_by_hand')
    real = [w for w in what if w in ('summon', 'darts', 'attack_roll', 'save_roll', 'damage', 'heal') or w.startswith('hp_effect')]
    gaps = []
    for n, txt in GAP.items():
        if n in t['needs']: gaps.append(n)
    if t['conditions'] and not m['hp_effect']:
        gaps.append('condition:' + '+'.join(t['conditions']))
    if 'damage_table_only' in what: gaps.append('damage_not_rolled')
    if 'creates_creature_or_object' in t['needs'] and not m['summon'] and 'zone' not in gaps: gaps.append('creation')
    if 'upcast_extra' in t['needs']: gaps.append('upcast_extra')
    timed = m['duration_kind'] in ('timed', 'special') and not m['concentration'] or m['concentration']
    if timed and (t['conditions'] or 'stat_change' in t['needs'] or 'zone' in t['needs'] or t['kind'] in ('buff', 'condition', 'debuff', 'zone')):
        gaps.append('duration_not_tracked')
    # state
    if k == 'spell:shield':
        state, why = 'built', ''
    elif m['economy'] == 'reaction':
        state, why = 'absent', 'reaction spell: the cast RPC refuses it (CASTING reason REACTION_ONLY)'
    elif not fight and not m['summon']:
        state, why = 'absent', 'casting time of %s: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat' % d['casting_time']
    else:
        material = [g for g in gaps if g not in ('duration_not_tracked', 'upcast_extra', 'creation')]
        if real and not material and not ('creation' in gaps): state, why = 'built', ''
        elif real: state, why = 'partial', ''
        else: state, why = 'reminder', ''
        if not real and not gaps and t['kind'] == 'utility': state = 'reminder'
    o = OVERRIDES.get(k)
    if o: 
        state = o.get('state', state); 
        if 'gaps' in o: gaps = o['gaps']
        if 'why' in o: why = o['why']
    HIGH = set('bless bane hold-person haste slow spirit-guardians web entangle faerie-fire hunters-mark mage-armor shield-of-faith misty-step invisibility fear hypnotic-pattern fly polymorph banishment guidance heroism counterspell hellish-rebuke darkness fog-cloud sleet-storm grease command charm-person hideous-laughter dominate-person hold-monster confusion stinking-cloud mirror-image blur barkskin protection-from-evil-and-good enlarge-reduce blindness-deafness cloudkill wall-of-force wall-of-fire dispel-magic greater-invisibility death-ward revivify lesser-restoration spiritual-weapon flaming-sphere moonbeam call-lightning heat-metal acid-arrow guiding-bolt shatter thunderwave sunbeam animate-objects conjure-woodland-beings conjure-minor-elementals conjure-fey conjure-elemental healing-word cure-wounds raise-dead resurrection identify alarm tiny-hut'.split())
    kk = k[6:]
    combat_kind = t['kind'] in ('condition', 'buff', 'debuff', 'zone', 'summon', 'save_damage', 'attack_damage', 'auto_damage', 'heal', 'other')
    if state == 'built': impact = 'low'
    elif kk in HIGH: impact = 'high'
    elif state == 'absent': impact = 'medium' if (m['level'] <= 5 and kk in ('feather-fall', 'mending', 'augury', 'commune', 'magic-circle', 'find-steed', 'glyph-of-warding', 'clairvoyance', 'create-undead', 'phantom-steed', 'contingency', 'magic-mouth', 'planar-ally', 'prayer-of-healing', 'regenerate', 'scrying', 'heroes-feast', 'reincarnate', 'commune-with-nature')) else 'low'
    elif combat_kind and m['level'] <= 3: impact = 'high' if state == 'partial' and gaps else 'medium'
    elif combat_kind and m['level'] <= 5: impact = 'medium'
    elif t['kind'] == 'utility' and m['level'] <= 2: impact = 'medium' if kk in ('disguise-self', 'knock', 'detect-magic', 'comprehend-languages', 'speak-with-animals', 'light', 'mage-hand', 'prestidigitation', 'message', 'minor-illusion') else 'low'
    else: impact = 'low'
    rows.append({'key': k, 'impact': impact, 'name_pt': names.get(k, ''), 'level': m['level'], 'state': state, 'kind': t['kind'], 'engine_does': what,
                 'missing': gaps, 'note': t['note'], 'why': why, 'conditions': t['conditions'], 'needs': t['needs'],
                 'casting_time': d['casting_time'], 'concentration': m['concentration'], 'ritual': m['ritual'], 'classes': d['classes'],
                 'override': bool(o)})
json.dump(rows, open(os.path.join(root, 'spells-states.json'), 'w'), indent=1)
with open(os.path.join(root, 'spells-states.csv'), 'w', newline='') as f:
    w = csv.writer(f); w.writerow(['key', 'name_pt', 'level', 'state', 'kind', 'engine_does', 'missing', 'impact', 'casting_time', 'concentration', 'note'])
    for r in rows: w.writerow([r['key'], r['name_pt'], r['level'], r['state'], r['kind'], ' '.join(r['engine_does']), ' '.join(r['missing']), r['impact'], r['casting_time'], r['concentration'], r['note']])
c = collections.Counter(r['state'] for r in rows); print(c)
