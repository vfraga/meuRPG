import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { By } from '@angular/platform-browser';
import { create } from '@bufbuild/protobuf';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AreaPlacement,
  CombatantKind,
  PreviewSpellAreaResponseSchema,
  SpellTargetsSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  Ability,
  ActionEconomy,
  SpellAreaShape,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { AreaMap } from '../../../../shared/area-picker/area-map';
import { CastSheet, type CastSheetData } from './cast-sheet';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();

const pensantus = combatant({
  id: 'p',
  label: 'Pensantus',
  kind: CombatantKind.PLAYER,
  mine: true,
  col: 2,
  row: 4,
});
const toren = combatant({ id: 't', label: 'Toren', kind: CombatantKind.PLAYER, col: 5, row: 4 });
const goblin = combatant({ id: 'g1', label: 'Goblin 1', col: 6, row: 3 });
const goblin2 = combatant({ id: 'g2', label: 'Goblin 2', col: 6, row: 5 });

const handsTargets = create(SpellTargetsSchema, {
  spellKey: 'spell:burning-hands',
  placement: AreaPlacement.DIRECTION,
  areaShape: SpellAreaShape.CONE,
  areaSizeFt: 15,
  targets: [],
});

const burningHands = {
  spell: { level: 1 },
  save: { ability: Ability.DEXTERITY },
  damage: [],
  healBySlotLevel: {},
  higherLevel: [],
};

function preview() {
  return create(PreviewSpellAreaResponseSchema, {
    origin: { col: 2, row: 4 },
    squares: [{ col: 3, row: 4 }],
    coverCounts: true,
    targets: [
      { combatantId: 'p', label: 'Pensantus', self: true, ally: true, distanceFt: 0 },
      { combatantId: 'g1', label: 'Goblin 1', distanceFt: 5 },
      { combatantId: 't', label: 'Toren', ally: true, distanceFt: 10 },
      { combatantId: 'g2', label: 'Goblin 2', distanceFt: 10 },
    ],
  });
}

async function open(sculptSpells: boolean) {
  TestBed.resetTestingModule();
  const previewSpellArea = vi.fn().mockResolvedValue(preview());
  const castSpell = vi.fn().mockResolvedValue({
    encounter: encounter({ combatants: [pensantus, toren, goblin, goblin2] }),
    cast: { targets: [], pendingDamages: [], metamagicKeys: [], slot: { level: 1, pact: false } },
    summoned: [],
  });
  const data = {
    campaignId: 'c',
    encounterId: 'enc',
    casterId: 'p',
    round: 2,
    spellKey: 'spell:burning-hands',
    name: 'Mãos Flamejantes',
    level: 1,
    concentration: false,
    cantripDice: '',
    economy: ActionEconomy.ACTION,
    slots: [{ level: 1, free: 3, pact: false }],
    usage: [{ level: 1, total: 4, used: 1 }],
    pact: null,
    targets: handsTargets,
    shieldFree: null,
    shieldName: '',
    attackBonus: 0,
    diceMode: DiceMode.PLAYERS_CHOOSE,
    preference: DicePreference.APP,
    sculptSpells,
    state: {
      encounter: signal(encounter({ combatants: [pensantus, toren, goblin, goblin2] })),
      apply: vi.fn(),
    },
    map: {
      image: { url: 'cave.png', width: 1200, height: 800 },
      mapName: 'A caverna',
      columns: 12,
      rows: 8,
      layers: null,
      fog: null,
    },
  } as unknown as CastSheetData;
  TestBed.configureTestingModule({
    providers: [
      { provide: CombatClient, useValue: fakeCombatClient({ castSpell, previewSpellArea }) },
      { provide: SpellCatalog, useValue: { details: () => Promise.resolve(burningHands) } },
      { provide: MAT_DIALOG_DATA, useValue: data },
      { provide: MatDialogRef, useValue: { close: vi.fn() } },
    ],
  });
  const fixture = TestBed.createComponent(CastSheet);
  const settle = async () => {
    for (let i = 0; i < 3; i++) {
      fixture.detectChanges();
      await fixture.whenStable();
    }
    fixture.detectChanges();
  };
  await settle();
  const el = fixture.nativeElement as HTMLElement;
  const button = (label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      plain(b.textContent).includes(label),
    );
  const toList = async () => {
    const map = fixture.debugElement.query(By.directive(AreaMap)).componentInstance as AreaMap;
    map.place.emit({ kind: 'direction', direction: { dx: 1, dy: 0 } });
    await settle();
    button('Confirmar direção')!.click();
    await settle();
  };
  const boxes = () =>
    Array.from(el.querySelectorAll<HTMLInputElement>('app-sculpt-picker input[type="checkbox"]'));
  return { el, button, toList, settle, boxes, castSpell };
}

describe('CastSheet: Esculpir Magias on an area spell', () => {
  it('offers nothing to a caster without the feature', async () => {
    const { el, toList } = await open(false);
    await toList();
    expect(plain(el.textContent)).toContain('Quem está na área');
    expect(el.querySelector('app-sculpt-picker')).toBeNull();
  });

  it('lists the creatures but the caster, allies first, nothing marked, with the count "0 de 2"', async () => {
    const { el, toList, boxes } = await open(true);
    await toList();
    const text = plain(el.querySelector('app-sculpt-picker')?.textContent);
    expect(text).toContain('Esculpir Magias');
    expect(text).toContain('0 de 2');
    expect(text).toContain('Escolha até 2 criaturas que passam automaticamente e não sofrem dano.');
    const rows = Array.from(el.querySelectorAll('app-sculpt-picker label')).map((l) =>
      plain(l.textContent),
    );
    expect(rows).toEqual(['Toren · aliado', 'Goblin 1', 'Goblin 2']);
    expect(boxes().every((b) => !b.checked)).toBe(true);
  });

  it('stops at 1 + the spell level and sends the marked creatures with the cast', async () => {
    const { el, button, toList, boxes, settle, castSpell } = await open(true);
    await toList();
    boxes()[0].click();
    boxes()[1].click();
    await settle();
    expect(plain(el.querySelector('app-sculpt-picker')?.textContent)).toContain('2 de 2');
    expect(boxes()[2].disabled).toBe(true);
    button('Conjurar Mãos Flamejantes')!.click();
    await settle();
    const call = castSpell.mock.calls[0] as unknown[];
    expect(call[12]).toEqual({ area: { direction: { dx: 1, dy: 0 } }, sculptedIds: ['t', 'g1'] });
  });

  it('sends no list when nobody is marked', async () => {
    const { button, toList, settle, castSpell } = await open(true);
    await toList();
    button('Conjurar Mãos Flamejantes')!.click();
    await settle();
    const call = castSpell.mock.calls[0] as unknown[];
    expect(call[12]).toEqual({ area: { direction: { dx: 1, dy: 0 } } });
  });
});
