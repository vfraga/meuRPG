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
  ReactionKind,
  SpellTargetsSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { ActionEconomy, SpellAreaShape } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { TableRulesClient } from '../../../../core/campaigns/table-rules';
import { CombatClient } from '../../../../core/combat/combat-client';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';
import { combatant, encounter, reactionWindow } from '../../../../core/combat/combat-testing';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { AreaMap } from '../../../../shared/area-picker/area-map';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { MasterAreaCast, type MasterAreaCastData } from './master-area-cast';

const plain = (t: string | null | undefined) =>
  (t ?? '')
    .replace(/\u00a0/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();

const pen = combatant({
  id: 'pen',
  label: 'Pensantus',
  kind: CombatantKind.PLAYER,
  col: 2,
  row: 4,
});
const ogro = combatant({ id: 'og', label: 'Ogro', col: 6, row: 4 });
const live = reactionWindow({ id: 'w1', kind: ReactionKind.READIED, reactorId: 'pen' } as never);

async function open() {
  TestBed.resetTestingModule();
  const castSpell = vi.fn().mockResolvedValue({ encounter: encounter(), cast: { targets: [] } });
  const previewSpellArea = vi.fn().mockResolvedValue(
    create(PreviewSpellAreaResponseSchema, {
      origin: { col: 6, row: 4 },
      coverCounts: true,
      targets: [{ combatantId: 'og', label: 'Ogro' }],
    }),
  );
  const markReactionRolling = vi.fn();
  const close = vi.fn();
  const enc = signal(encounter({ combatants: [pen, ogro], reactionWindows: [live] } as never));
  const data: MasterAreaCastData = {
    campaignId: 'c',
    encounterId: 'enc',
    caster: pen,
    spellKey: 'spell:fireball',
    name: 'Bola de Fogo',
    level: 3,
    slot: null,
    economy: ActionEconomy.REACTION,
    targets: create(SpellTargetsSchema, {
      placement: AreaPlacement.POINT,
      areaShape: SpellAreaShape.SPHERE,
      areaSizeFt: 20,
      rangeFt: 150,
    }),
    map: {
      image: { url: 'c.png', width: 1200, height: 800 },
      mapName: 'Cripta',
      columns: 12,
      rows: 8,
      layers: null,
      fog: null,
    },
    state: { encounter: enc, apply: vi.fn((e) => enc.set(e)) } as never,
    release: {
      windowId: 'w1',
      readiedId: 'r1',
      reactorLabel: 'Pensantus',
      trigger: 'Quando o ogro passar pela porta',
      slotLevel: 3,
    },
  };
  TestBed.configureTestingModule({
    providers: [
      {
        provide: CombatClient,
        useValue: fakeCombatClient({ castSpell, previewSpellArea, markReactionRolling }),
      },
      { provide: SpellCatalog, useValue: { details: () => Promise.resolve(null) } },
      {
        provide: TableRulesClient,
        useValue: {
          get: () => Promise.resolve({ saved: { hiddenAreaHits: HiddenAreaHitRule.REVEAL } }),
        },
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
    );
  return {
    fixture,
    el,
    button,
    settle,
    castSpell,
    previewSpellArea,
    markReactionRolling,
    close,
    enc,
  };
}

describe('MasterAreaCast: the master releases a readied spell for the player (#857, 857-07)', () => {
  it('says "Soltar Bola de Fogo por Pensantus", the band, and that the player may still answer', async () => {
    const { el, button, markReactionRolling } = await open();
    expect(plain(el.querySelector('.frame__title')?.textContent)).toBe(
      'Soltar Bola de Fogo por Pensantus',
    );
    expect(plain(el.querySelector('.frame__sub')?.textContent)).toContain(
      'Reação · a ação preparada de Pensantus · você responde por Pensantus',
    );
    expect(plain(el.querySelector('[data-testid=release-band]')?.textContent)).toContain(
      'Preparada: “Quando o ogro passar pela porta”',
    );
    expect(plain(el.querySelector('[data-testid=release-foot]')?.textContent)).toBe(
      'Pensantus ainda pode responder no próprio aparelho: vale a primeira resposta.',
    );
    expect(button('Voltar')).toBeDefined();
    expect(button('Cancelar')).toBeUndefined();
    expect(markReactionRolling).not.toHaveBeenCalled();
  });

  it('draws the subtitle with the filled marker and a bold "Reação" (857-04)', async () => {
    const { el } = await open();
    const sub = el.querySelector('.frame__sub');
    expect(sub?.querySelector('.frame__mark')?.getAttribute('aria-hidden')).toBe('true');
    expect(plain(sub?.querySelector('b')?.textContent)).toBe('Reação');
    expect(plain(sub?.textContent)).toBe(
      'Reação · a ação preparada de Pensantus · você responde por Pensantus',
    );
  });

  it('shows the band under the header, and the body is not scrolled past it when the list opens', async () => {
    const { el, button, fixture, settle } = await open();
    const map = fixture.debugElement.query(By.directive(AreaMap)).componentInstance as AreaMap;
    map.place.emit({ kind: 'point', square: { col: 6, row: 4 } });
    await settle();
    button('Confirmar local')!.click();
    await settle();
    expect(el.querySelector('app-release-band')).not.toBeNull();
    const focus = vi.spyOn(HTMLElement.prototype, 'focus');
    const scrollTop = vi.spyOn(SheetFrame.prototype, 'scrollToTop');
    try {
      button('Mudar o local')!.click();
      await settle();
      button('Confirmar local')!.click();
      await settle();
      const heading = focus.mock.contexts.findIndex(
        (c, i) =>
          (c as HTMLElement).closest('app-area-step') !== null &&
          (focus.mock.calls[i]?.[0] as FocusOptions | undefined)?.preventScroll === true,
      );
      expect(heading).toBeGreaterThanOrEqual(0);
      expect(scrollTop).toHaveBeenCalled();
    } finally {
      focus.mockRestore();
      scrollTop.mockRestore();
    }
  });

  it('puts the back arrow on "Voltar" and none on "Mudar o local" in the release foot (857-04)', async () => {
    const { el, button, fixture, settle } = await open();
    const map = fixture.debugElement.query(By.directive(AreaMap)).componentInstance as AreaMap;
    map.place.emit({ kind: 'point', square: { col: 6, row: 4 } });
    await settle();
    button('Confirmar local')!.click();
    await settle();
    expect(button('Voltar')?.querySelector('mat-icon')?.textContent?.trim()).toBe('arrow_back');
    expect(button('Mudar o local')?.querySelector('mat-icon')).toBeNull();
    expect(el.querySelector('[data-testid=release-foot]')).not.toBeNull();
  });

  it('previews and casts with no slot, then closes with the cast', async () => {
    const { fixture, button, settle, castSpell, previewSpellArea, close } = await open();
    const map = fixture.debugElement.query(By.directive(AreaMap)).componentInstance as AreaMap;
    map.place.emit({ kind: 'point', square: { col: 6, row: 4 } });
    await settle();
    expect((previewSpellArea.mock.calls[0] as unknown[])[4]).toBeNull();
    button('Confirmar local')!.click();
    await settle();
    button('Conjurar Bola de Fogo')!.click();
    await settle();
    expect((castSpell.mock.calls[0] as unknown[])[4]).toBeNull();
    expect(close).toHaveBeenCalledWith(true);
  });

  it('drops the map when the player answered first: the notice and "Voltar à sessão" only', async () => {
    const { el, button, settle, enc } = await open();
    enc.set(encounter({ revision: 9, combatants: [pen, ogro], reactionWindows: [] } as never));
    await settle();
    expect(el.querySelector('[role="application"]')).toBeNull();
    expect(plain(el.querySelector('[data-testid=release-gone]')?.textContent)).toContain(
      'A janela fechou',
    );
    expect(button('Voltar à sessão')).toBeDefined();
    expect(button('Conjurar')).toBeUndefined();
  });
});
