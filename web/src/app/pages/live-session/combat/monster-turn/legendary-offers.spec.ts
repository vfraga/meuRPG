import { TestBed } from '@angular/core/testing';

import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { CreatureClient } from '../../../../core/combat/creature-client';
import { LegendaryOffers } from './legendary-offers';

const dragon = combatant({ id: 'dragon', label: 'Dragão vermelho adulto' });
const toren = combatant({ id: 'toren', label: 'Toren' });

const option = (key: string, name: string, cost: number, available: boolean) => ({
  key,
  name,
  namePt: name,
  cost,
  actionKey: '',
  text: 'texto',
  available,
});

function setup(client: Partial<Record<keyof CreatureClient, unknown>>, left = 1) {
  TestBed.resetTestingModule();
  TestBed.configureTestingModule({ providers: [{ provide: CreatureClient, useValue: client }] });
  const fixture = TestBed.createComponent(LegendaryOffers);
  const apply = vi.fn();
  fixture.componentRef.setInput('campaignId', 'c');
  fixture.componentRef.setInput('state', { apply } as unknown as CombatState);
  fixture.componentRef.setInput('encounter', {
    ...encounter({ combatants: [dragon, toren] }),
    legendaryOffers: [
      {
        combatantId: 'dragon',
        afterCombatantId: 'toren',
        left,
        perRound: 3,
        options: [
          option('detect', 'Detectar', 1, true),
          option('wing', 'Ataque de asa', 2, left >= 2),
        ],
      },
    ],
    legendaryResistancePrompts: [
      {
        combatantId: 'dragon',
        castId: 'cast-1',
        spellKey: 'spell:lightning-bolt',
        spellNamePt: 'Relâmpago',
        casterId: 'toren',
        ability: 2,
        dc: 15,
        d20: 6,
        bonus: 6,
        total: 12,
        usesLeft: 3,
        uses: 3,
      },
    ],
  } as never);
  fixture.detectChanges();
  return { fixture, apply, el: fixture.nativeElement as HTMLElement };
}

describe('LegendaryOffers', () => {
  it('offers the actions that fit and greys the one that does not, with its reason', () => {
    const { el } = setup({}, 1);
    expect(el.textContent).toContain('Fim da vez de Toren');
    expect(el.textContent).toContain('1 de 3');
    const wing = Array.from(el.querySelectorAll('.option button')).find((b) =>
      b.textContent?.includes('Usar (2)'),
    ) as HTMLButtonElement;
    expect(wing.getAttribute('aria-disabled')).toBe('true');
    expect(
      document.getElementById(wing.getAttribute('aria-describedby') ?? '')?.textContent,
    ).toContain('Resta só 1: não cabe');
  });

  it('uses an option and lets an offer pass', async () => {
    const useLegendary = vi.fn().mockResolvedValue({ encounter: { id: 'e1' } });
    const declineLegendary = vi.fn().mockResolvedValue({ id: 'e2' });
    const { fixture, apply } = setup({ useLegendary, declineLegendary }, 3);
    const offers = fixture.componentInstance as unknown as {
      use(o: unknown, key: string): Promise<void>;
      pass(o: unknown): Promise<void>;
    };
    const offer =
      fixture.componentRef.injector && (await Promise.resolve({ combatantId: 'dragon' }));
    await offers.use(offer, 'detect');
    expect(useLegendary.mock.calls[0].slice(2, 5)).toEqual(['dragon', 'detect', []]);
    await offers.pass(offer);
    expect(declineLegendary).toHaveBeenCalledTimes(1);
    expect(apply).toHaveBeenCalledTimes(2);
  });

  it('asks about Legendary Resistance and answers both ways', async () => {
    const answerResistance = vi.fn().mockResolvedValue({ id: 'e' });
    const { fixture, el } = setup({ answerResistance });
    expect(el.textContent).toContain('falhou no teste');
    expect(el.textContent).toContain('Relâmpago');
    expect(el.textContent).toContain('1d20 (6) + 6 = 12');
    const buttons = Array.from(el.querySelectorAll('.buttons button'));
    expect(buttons.map((b) => b.textContent?.trim())).toEqual([
      'Usar Resistência Lendária',
      'Deixar falhar',
    ]);
    (buttons[0] as HTMLButtonElement).click();
    await fixture.whenStable();
    expect(answerResistance.mock.calls[0].slice(2, 5)).toEqual(['dragon', 'cast-1', true]);
  });
});
