import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { By } from '@angular/platform-browser';
import { create } from '@bufbuild/protobuf';

import { HiddenAreaHitRule } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AreaPlacement,
  CombatantKind,
  PreviewSpellAreaResponseSchema,
  SpellTargetsSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { ActionEconomy, SpellAreaShape } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { TableRulesClient } from '../../../../core/campaigns/table-rules';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { AreaMap } from '../../../../shared/area-picker/area-map';
import { MasterAreaCast, type MasterAreaCastData } from './master-area-cast';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const plain = (t: string | null | undefined) =>
  (t ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

const zuk = combatant({ id: 'z', label: 'Zuk', col: 8, row: 1 });
const toren = combatant({ id: 't', label: 'Toren', kind: CombatantKind.PLAYER, col: 5, row: 4 });
const g3 = combatant({ id: 'g3', label: 'Goblin 3', hidden: true, col: 6, row: 5 });

async function open(rule: HiddenAreaHitRule, hiddenInside: boolean) {
  TestBed.resetTestingModule();
  const castSpell = vi.fn().mockResolvedValue({ encounter: encounter(), cast: { targets: [] } });
  const previewSpellArea = vi.fn().mockResolvedValue(
    create(PreviewSpellAreaResponseSchema, {
      origin: { col: 6, row: 4 },
      coverCounts: true,
      targets: [
        { combatantId: 't', label: 'Toren' },
        ...(hiddenInside
          ? [{ combatantId: 'g3', label: 'Goblin 3', ally: true, hidden: true }]
          : []),
      ],
    }),
  );
  const close = vi.fn();
  const data: MasterAreaCastData = {
    campaignId: 'c',
    encounterId: 'enc',
    caster: zuk,
    spellKey: 'spell:fireball',
    name: 'Bola de Fogo',
    level: 3,
    slot: { level: 3, pact: false },
    economy: ActionEconomy.ACTION,
    targets: create(SpellTargetsSchema, {
      placement: AreaPlacement.POINT,
      areaShape: SpellAreaShape.SPHERE,
      areaSizeFt: 20,
      rangeFt: 150,
    }),
    map: {
      image: { url: 'cave.png', width: 1200, height: 800 },
      mapName: 'A caverna',
      columns: 12,
      rows: 8,
      layers: null,
      fog: null,
    },
    state: {
      encounter: signal(encounter({ combatants: [zuk, toren, g3] })),
      apply: vi.fn(),
    } as never,
  };
  TestBed.configureTestingModule({
    providers: [
      { provide: CombatClient, useValue: fakeCombatClient({ castSpell, previewSpellArea }) },
      { provide: SpellCatalog, useValue: { details: () => Promise.resolve(null) } },
      {
        provide: TableRulesClient,
        useValue: { get: () => Promise.resolve({ saved: { hiddenAreaHits: rule } }) },
      },
      { provide: MAT_DIALOG_DATA, useValue: data },
      { provide: MatDialogRef, useValue: { close } },
    ],
  });
  const fixture = TestBed.createComponent(MasterAreaCast);
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
    )!;
  const map = fixture.debugElement.query(By.directive(AreaMap)).componentInstance as AreaMap;
  map.place.emit({ kind: 'point', square: { col: 6, row: 4 } });
  await settle();
  button('Confirmar local').click();
  await settle();
  return { el, button, settle, castSpell, close };
}

describe("MasterAreaCast: the master casts an NPC's area spell", () => {
  it('names the caster, lists everyone with the hidden one marked, and starts the switch on for "Revelar"', async () => {
    const { el, button, settle, castSpell, close } = await open(HiddenAreaHitRule.REVEAL, true);
    expect(plain(el.querySelector('.frame__title')?.textContent)).toBe('Zuk conjura Bola de Fogo');
    expect(plain(el.textContent)).toContain('2 criaturas · 1 escondida');
    expect(plain(el.textContent)).toContain('Goblin 3 é aliado do Zuk.');
    expect(plain(el.querySelector('app-area-list')?.textContent)).toContain('Escondido');
    const sw = el.querySelector<HTMLButtonElement>('[role="switch"]')!;
    expect(sw.getAttribute('aria-checked')).toBe('true');
    expect(plain(el.textContent)).toContain(
      'Goblin 3 aparece para os jogadores depois da magia. Começa na regra da mesa (“Revelar”).',
    );
    button('Conjurar Bola de Fogo').click();
    await settle();
    expect(castSpell.mock.calls[0][12]).toEqual({
      area: { origin: { col: 6, row: 4 } },
      revealHidden: true,
    });
    expect(close).toHaveBeenCalledWith(true);
  });

  it('starts the switch off for "Manter escondidas", and sends what the master turned it to', async () => {
    const { el, button, settle, castSpell } = await open(HiddenAreaHitRule.KEEP_HIDDEN, true);
    const sw = el.querySelector<HTMLButtonElement>('[role="switch"]')!;
    expect(sw.getAttribute('aria-checked')).toBe('false');
    sw.click();
    await settle();
    expect(sw.getAttribute('aria-checked')).toBe('true');
    button('Conjurar Bola de Fogo').click();
    await settle();
    expect(castSpell.mock.calls[0][12].revealHidden).toBe(true);
  });

  it('starts the switch on for "Perguntar a cada vez": the master decides here', async () => {
    const { el } = await open(HiddenAreaHitRule.ASK, true);
    expect(el.querySelector('[role="switch"]')?.getAttribute('aria-checked')).toBe('true');
  });

  it('has no switch and leaves the reveal out when no hidden creature is inside', async () => {
    const { el, button, settle, castSpell } = await open(HiddenAreaHitRule.KEEP_HIDDEN, false);
    expect(el.querySelector('[role="switch"]')).toBeNull();
    button('Conjurar Bola de Fogo').click();
    await settle();
    expect(castSpell.mock.calls[0][12].revealHidden).toBeUndefined();
  });
});
