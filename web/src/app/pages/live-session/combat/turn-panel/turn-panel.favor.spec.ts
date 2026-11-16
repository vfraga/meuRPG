import { TestBed } from '@angular/core/testing';
import { afterEach, vi } from 'vitest';

import { CombatantKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { FavorClient } from '../../../../core/play/favor-client';
import { FavorState } from '../../../../core/play/favor-state';
import { FakeFavorClient, favorPass, openFavor } from '../../../../core/play/favor-testing';
import { TurnPanel } from './turn-panel';

// The read-only chip "★ Inspiração" in the player's band (RN-43, board 581-03): only for the owner's own character (RN-10).

const brisa = combatant({
  id: 'bri',
  label: 'Brisa',
  kind: CombatantKind.PLAYER,
  mine: true,
  characterId: 'bri-c',
} as never);
const tor = combatant({
  id: 'tor',
  label: 'Tor',
  kind: CombatantKind.PLAYER,
  characterId: 'tor-c',
} as never);

function panel(
  holders: Record<string, string> | null,
  compact: boolean,
  current = 'bri',
  asks: { master?: boolean; from?: string } | null = null,
) {
  // From 1024 px the own turn is the compact band; below it, the card.
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: compact && query.includes('min-width: 1024'),
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  }));
  TestBed.configureTestingModule({
    providers: [{ provide: FavorClient, useValue: new FakeFavorClient().asClient() }],
  });
  if (holders) {
    const favor = TestBed.inject(FavorState);
    openFavor(favor, holders);
    if (asks) {
      favor.master.set(!!asks.master);
      favor.passes.set([favorPass({ fromId: asks.from ?? 'bri-c' })]);
    }
  }
  const fixture = TestBed.createComponent(TurnPanel);
  fixture.componentRef.setInput(
    'encounter',
    encounter({ currentCombatantId: current, combatants: [brisa, tor] }),
  );
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('TurnPanel: the Inspiração chip', () => {
  afterEach(() => vi.unstubAllGlobals());

  it.each([
    ['the card of a phone', false],
    ['the band of a computer', true],
  ])('shows "Inspiração" in %s when the own character has it', (_name, compact) => {
    const el = panel({ 'bri-c': 'Brisa' }, compact);
    const chips = el.querySelectorAll('app-favor-chip');
    expect(chips).toHaveLength(1);
    expect((chips[0].textContent ?? '').replace(/\s+/g, ' ').trim()).toBe('starInspiração');
    expect(chips[0].querySelector('button')).toBeNull();
  });

  it.each([false, true])(
    'shows nothing without it, or when the read failed (compact %s)',
    (compact) => {
      expect(panel({}, compact).querySelector('app-favor-chip')).toBeNull();
    },
  );

  it('shows nothing outside an open session', () => {
    expect(panel(null, false).querySelector('app-favor-chip')).toBeNull();
  });

  it("shows nothing on another player's turn, whoever the state knows", () => {
    const el = panel({ 'bri-c': 'Brisa', 'tor-c': 'Tor' }, false, 'tor');
    expect(el.querySelector('app-favor-chip')).toBeNull();
  });

  it.each([
    ['the card of a phone', false],
    ['the band of a computer', true],
  ])('reads "Inspiração · esperando o mestre" in %s while the own ask waits', (_name, compact) => {
    const el = panel({ 'bri-c': 'Brisa' }, compact, 'bri', {});
    const chip = el.querySelector('app-favor-chip')!;
    expect((chip.textContent ?? '').replace(/\s+/g, ' ').trim()).toBe(
      'starInspiração · esperando o mestre',
    );
    expect(chip.classList.contains('favor--waiting')).toBe(true);
  });

  it("reads the plain chip when the ask is another character's, and for the master", () => {
    expect(
      panel({ 'bri-c': 'Brisa' }, false, 'bri', { from: 'tor-c' }).querySelector('app-favor-chip')!
        .textContent,
    ).not.toContain('esperando');
    TestBed.resetTestingModule();
    expect(
      panel({ 'bri-c': 'Brisa' }, false, 'bri', { master: true }).querySelector('app-favor-chip')!
        .textContent,
    ).not.toContain('esperando');
  });
});
