import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import {
  GetTurnOptionsResponseSchema,
  ManeuverDieOptionSchema,
  OpportunityOfferSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { CombatState } from '../../../../core/combat/combat-state';
import { OpportunitySheet, type OpportunitySheetData } from './opportunity-sheet';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const plain = (t: string | null | undefined) =>
  (t ?? '')
    .replace(/\u00a0/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();

// Goblin 2 leaves Toren's reach (E9-13, frames C and G).
const offer = create(OpportunityOfferSchema, {
  id: 'o1',
  moverId: 'g2',
  moverLabel: 'Goblin 2',
  reactorId: 'toren',
  reactorLabel: 'Toren',
  forYou: true,
});

function setup(
  opts: {
    present?: boolean;
    fail?: boolean;
    dice?: { uses: number; name: string; pool?: string }[];
    attacks?: { key: string; name: string; detail: string; attack: null }[];
  } = {},
) {
  const state = new CombatState();
  state.apply(
    encounter({
      combatants: [combatant({ id: 'g2', label: 'Goblin 2' })],
      opportunityOffers: opts.present === false ? [] : [offer],
    }),
  );
  const declined: string[] = [];
  const closed: unknown[] = [];
  const data: OpportunitySheetData = {
    campaignId: 'c',
    encounterId: 'enc',
    offer,
    round: 2,
    load: async () => {
      if (opts.fail) {
        throw new Error('down');
      }
      return {
        attacks: opts.attacks ?? [
          {
            key: 'attack:longsword',
            name: 'Espada longa',
            detail: 'Espada longa +5 · 1d8 + 3 cortante',
            attack: null,
          },
        ],
        options: create(GetTurnOptionsResponseSchema, {
          maneuverDice: (opts.dice ?? []).map((d) =>
            create(ManeuverDieOptionSchema, {
              die: { namePt: d.name, sides: 8 },
              resourceNamePt: d.pool ?? 'Reserva de Golpes',
              usesLeft: d.uses,
            }),
          ),
        }),
      };
    },
    state,
  };
  TestBed.configureTestingModule({
    providers: [
      { provide: MAT_DIALOG_DATA, useValue: data },
      { provide: MatDialogRef, useValue: { close: (r: unknown) => closed.push(r) } },
      {
        provide: CombatClient,
        useValue: fakeCombatClient({
          declineOpportunity: async (_c: string, _e: string, id: string) => {
            declined.push(id);
            return encounter({ opportunityOffers: [] });
          },
        }),
      },
    ],
  });
  const fixture = TestBed.createComponent(OpportunitySheet);
  fixture.detectChanges();
  return { fixture, el: fixture.nativeElement as HTMLElement, declined, closed };
}

describe('OpportunitySheet', () => {
  it('asks the player, names the weapon with its numbers and the cost, and never shows an armor class', async () => {
    const { fixture, el } = setup();
    await fixture.whenStable();
    fixture.detectChanges();
    const text = plain(el.textContent);
    expect(text).toContain('Ataque de oportunidade');
    expect(text).toContain('Goblin 2 · Rodada 2');
    expect(text).toContain('O Goblin 2 está saindo do seu alcance. Ataque de oportunidade?');
    expect(text).toContain('Gasta a sua reação: ela só volta no começo do seu próximo turno.');
    expect(plain(el.querySelector('.weapons li')?.textContent)).toBe(
      'Espada longa +5 · 1d8 + 3 cortante',
    );
    expect(text).not.toMatch(/CA \d/);
  });

  it('stacks the answers, "Não atacar" first and marked for the initial focus, one filled button, and no close button', async () => {
    const { fixture, el } = setup({
      attacks: [
        {
          key: 'a',
          name: 'Espada longa',
          detail: 'Espada longa +5 · 1d8 + 3 cortante',
          attack: null,
        },
        { key: 'b', name: 'Adaga', detail: 'Adaga +5 · 1d4 + 3 perfurante', attack: null },
      ],
    });
    await fixture.whenStable();
    fixture.detectChanges();
    const buttons = Array.from(el.querySelectorAll<HTMLButtonElement>('.stack .btn'));
    expect(buttons.map((b) => plain(b.textContent))).toEqual([
      'Não atacar',
      'Atacar com Espada longa',
      'Atacar com Adaga',
    ]);
    expect(buttons[0].hasAttribute('data-initial-focus')).toBe(true);
    // The same width (full) for all, and one filled button: the first weapon's.
    expect(buttons.every((b) => b.classList.contains('btn'))).toBe(true);
    expect(el.querySelectorAll('.stack .mat-mdc-unelevated-button').length).toBe(1);
    expect(buttons[1].classList.contains('mat-mdc-unelevated-button')).toBe(true);
    expect(el.querySelector('[aria-label="Fechar"]')).toBeNull();
  });

  it('keeps the attack buttons off until the attacks are read, and says why', () => {
    const { el } = setup();
    expect(plain(el.textContent)).toContain('Lendo os seus ataques…');
    const attack = Array.from(el.querySelectorAll<HTMLButtonElement>('.stack .btn')).find(
      (b) => plain(b.textContent) === 'Atacar',
    )!;
    expect(attack.disabled || attack.getAttribute('aria-disabled') === 'true').toBe(true);
  });

  it('never leaves the offer stuck when the read fails: "Tentar de novo", and "Não atacar" still works', async () => {
    const { fixture, el, declined } = setup({ fail: true });
    await fixture.whenStable();
    fixture.detectChanges();
    expect(plain(el.textContent)).toContain('Não deu para ler os seus ataques.');
    const names = Array.from(el.querySelectorAll('.stack .btn'), (b) => plain(b.textContent));
    expect(names).toEqual(['Não atacar', 'Tentar de novo']);
    el.querySelector<HTMLButtonElement>('.stack .btn')!.click();
    await fixture.whenStable();
    expect(declined).toEqual(['o1']);
  });

  it('turns the offer down, and hands the attack over on "Atacar"', async () => {
    const { fixture, el, declined, closed } = setup();
    await fixture.whenStable();
    fixture.detectChanges();
    const [no, yes] = Array.from(el.querySelectorAll<HTMLButtonElement>('.stack .btn'));
    yes.click();
    expect(closed).toEqual([
      { attackKey: 'attack:longsword', options: create(GetTurnOptionsResponseSchema) },
    ]);
    no.click();
    await fixture.whenStable();
    expect(declined).toEqual(['o1']);
    expect(closed[1]).toBeNull();
  });

  it('says so when the master answered first, and only closes', () => {
    const { el } = setup({ present: false });
    expect(plain(el.textContent)).toContain('O mestre respondeu por você');
    expect(Array.from(el.querySelectorAll('.btn'), (b) => plain(b.textContent))).toEqual([
      'Fechar',
    ]);
  });

  describe('the table die line', () => {
    const line = (el: HTMLElement) => plain(el.querySelector('.mr-die-note')?.textContent);
    async function opened(dice?: { uses: number; name: string; pool?: string }[]) {
      const r = setup({ dice });
      await r.fixture.whenStable();
      r.fixture.detectChanges();
      return r;
    }

    it('says under the weapons that the die may join this attack, with the uses left, the icon hidden', async () => {
      const { el } = await opened([{ uses: 2, name: 'Golpe Certeiro' }]);
      expect(line(el)).toContain(
        'Golpe Certeiro (d8) pode entrar neste ataque: você marca ao rolar. Restam 2.',
      );
      expect(el.querySelector('.mr-die-note mat-icon')?.getAttribute('aria-hidden')).toBe('true');
      expect(el.querySelector('.mr-die-note button, .mr-die-note input')).toBeNull();
    });

    it('says "Resta 1." in the singular for one use', async () => {
      const { el } = await opened([{ uses: 1, name: 'Golpe Certeiro' }]);
      expect(line(el)).toContain('você marca ao rolar. Resta 1.');
    });

    it('says "Sem usos." at zero, without claiming a recharge', async () => {
      const { el } = await opened([{ uses: 0, name: 'Golpe Certeiro' }]);
      expect(line(el)).toContain('você marca ao rolar. Sem usos.');
      expect(line(el)).not.toMatch(/descanso/);
    });

    it('names both dice in the plural', async () => {
      const { el } = await opened([
        { uses: 1, name: 'Golpe Certeiro' },
        { uses: 1, name: 'Ataque Firme' },
      ]);
      expect(line(el)).toContain(
        'Golpe Certeiro (d8) e Ataque Firme (d8) podem entrar neste ataque',
      );
    });

    it('is absent without dice (the master answering for an NPC), and "Não atacar" stays first', async () => {
      const { el } = await opened();
      expect(el.querySelector('.mr-die-note')).toBeNull();
      expect(plain(el.querySelector('.stack .btn')?.textContent)).toBe('Não atacar');
    });
  });
});
