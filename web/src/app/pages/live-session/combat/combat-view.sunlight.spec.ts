// While the master paints the sun on the map (#704), "Próximo turno" waits until "Pronto".
import { CombatantKind } from '../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import { settle } from '../../../../test-settle';
import { mountCombatView, type ViewApi } from './combat-view-testing';

describe('CombatView, painting the sun holds the turn', () => {
  async function mount() {
    const enc = encounter({
      combatants: [
        combatant({ id: 'g1', label: 'Goblin 1', initiative: 14 }),
        combatant({
          id: 'pen',
          label: 'Pensantus',
          kind: CombatantKind.PLAYER,
          characterId: 'p',
          initiative: 12,
        }),
      ],
      currentCombatantId: 'g1',
      round: 1,
      sunlightOn: true,
    });
    const mounted = mountCombatView({ enc, master: true, matches: ['(min-width: 1024px)'] });
    await settle(mounted.fixture);
    return mounted;
  }
  const next = (el: HTMLElement) =>
    el.querySelector<HTMLButtonElement>('app-next-turn button.next')!;

  it('is not held while no paint mode is open', async () => {
    const { fixture } = await mount();
    expect(next(fixture.nativeElement).disabled).toBe(false);
  });

  it('disables the turn button and says why until "Pronto"', async () => {
    const { fixture } = await mount();
    const view = fixture.componentInstance as ViewApi;
    view.sunPainting.set(true);
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(next(el).getAttribute('aria-disabled') === 'true' || next(el).disabled).toBe(true);
    expect(el.querySelector('app-next-turn .why')?.textContent).toContain('Pronto');
    view.sunPainting.set(false);
    await settle(fixture);
    expect(next(el).disabled).toBe(false);
    expect(next(el).getAttribute('aria-disabled')).not.toBe('true');
  });

  it('puts the one sun switch in the combat header on a phone, not by "Encerrar combate"', async () => {
    const enc = encounter({
      combatants: [combatant({ id: 'g1', label: 'Goblin 1', initiative: 14 })],
      currentCombatantId: 'g1',
      round: 2,
      sunlightOn: true,
    });
    const { fixture } = mountCombatView({ enc, master: true, matches: ['(max-width: 767.98px)'] });
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelectorAll('[role="switch"]').length).toBe(1);
    expect(el.querySelector('.phone__line [role="switch"]')).not.toBeNull();
    expect(el.querySelector('.end-only [role="switch"]')).toBeNull();
  });

  it('keeps it by "Encerrar combate" on a tablet', async () => {
    const enc = encounter({
      combatants: [combatant({ id: 'g1', label: 'Goblin 1', initiative: 14 })],
      currentCombatantId: 'g1',
      round: 2,
    });
    const { fixture } = mountCombatView({ enc, master: true, matches: [] });
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelectorAll('[role="switch"]').length).toBe(1);
    expect(el.querySelector('.end-only [role="switch"]')).not.toBeNull();
  });
});
