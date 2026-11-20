import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';

import { LevelUpSwapGroupSchema } from '../../../../gen/meurpg/characters/v1/characters_pb';
import { LevelUpSwap } from '../../../core/levelup/levelup-swap';
import { fighterOptions } from '../../../core/levelup/levelup-testing';
import type { LevelUpSession } from '../level-up-session';
import { PicksStep } from './picks-step';

const group = create(LevelUpSwapGroupSchema, {
  group: 'feature:infusoes@mesa',
  namePt: 'Infusões',
  picks: [{ key: 'a', namePt: 'Escudo' }],
  pool: [{ key: 'b', namePt: 'Radiante' }],
});

function render(abilityScoreImprovement: boolean) {
  const options = fighterOptions({ abilityScoreImprovement, swapGroups: [group] });
  const session = {
    options,
    draft: {
      swap: new LevelUpSwap(options),
      features: () => new Set<string>(),
      totals: () => ({ featureChoices: [] }),
      skillsAsked: () => 0,
      instrumentAsked: () => 0,
      expertiseAsked: () => 0,
    },
    groups: () => [],
  } as unknown as LevelUpSession;
  const fixture = TestBed.createComponent(PicksStep);
  fixture.componentRef.setInput('s', session);
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('PicksStep: "Trocar uma escolha"', () => {
  it('shows the swap panel, with the table feature group, at a level without the ability step', () => {
    const el = render(false);
    expect(el.querySelector('app-swap-picker')).not.toBeNull();
    expect(el.textContent).toContain('Trocar escolhas');
    expect(el.textContent).toContain('Infusões');
  });

  it('leaves the panel to the ability step where the level has one', () => {
    expect(render(true).querySelector('app-swap-picker')).toBeNull();
  });
});
