import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { describe, expect, it, vi } from 'vitest';

import {
  CombatantSide,
  CreatureActionResultSchema,
  PendingDamageStatus,
  SaveOutcome,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  Ability,
  CreatureAreaShape,
  CreatureSchema,
  SpellSaveSuccess,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';
import type { CombatState } from '../../../../core/combat/combat-state';
import {
  combatant,
  creatureAction,
  creaturesProvider,
  encounter,
} from '../../../../core/combat/combat-testing';
import { settle } from '../../../../../test-settle';
import { NpcCard } from './npc-card';

const breath = creatureAction({
  key: 'monster:hell-hound#fire-breath',
  name: 'Fire Breath',
  namePt: 'Sopro de fogo',
  area: { shape: CreatureAreaShape.CONE, sizeFt: 15, widthFt: 0 },
  save: {
    ability: Ability.DEXTERITY,
    dc: 13,
    onSuccess: SpellSaveSuccess.HALF,
    damage: [{ dice: '6d6', damageTypeKey: 'damage:fire', damageTypePt: 'fogo' }],
  },
} as never);
const hound = create(CreatureSchema, { actions: [breath] as never });

const pending = {
  id: 'pd1',
  attackerId: 'hound',
  targetId: 'toren',
  status: PendingDamageStatus.AWAITING_ROLL,
  diceCount: 6,
  diceSides: 6,
  bonus: 0,
} as never;

async function open(blocks: Record<string, ReturnType<typeof create<typeof CreatureSchema>>>) {
  TestBed.resetTestingModule();
  const npc = combatant({
    id: 'hound',
    label: 'Cão Infernal 1',
    bestiaryCreatureKey: 'monster:hell-hound',
    side: CombatantSide.ENEMY,
  });
  const toren = combatant({ id: 'toren', label: 'Toren', side: CombatantSide.PARTY });
  const enc = encounter({ combatants: [npc, toren] });
  const useCreatureAction = vi.fn(async () => ({
    encounter: enc,
    result: create(CreatureActionResultSchema, {
      castId: 'cast',
      targets: [
        {
          combatantId: 'toren',
          save: { outcome: SaveOutcome.FAILED, dc: 13 },
          pendingDamageId: 'pd1',
        },
      ],
      pendingDamages: [{ id: 'pd1' }],
    }),
    area: null,
  }));
  TestBed.configureTestingModule({
    providers: [
      provideRouter([]),
      { provide: CombatClient, useValue: fakeCombatClient({ useCreatureAction }) },
      creaturesProvider(blocks),
    ],
  });
  const fixture = TestBed.createComponent(NpcCard);
  fixture.componentRef.setInput('campaignId', 'c');
  fixture.componentRef.setInput('encounter', enc);
  fixture.componentRef.setInput('subject', npc);
  fixture.componentRef.setInput('state', {
    apply: vi.fn(),
    encounter: () => enc,
  } as unknown as CombatState);
  fixture.componentRef.setInput('theatre', true);
  fixture.componentRef.setInput('options', {
    options: { attacks: [], spells: [] },
    attackTargets: [],
    pendingDamages: [pending],
    spellTargets: [],
  } as never);
  await settle(fixture);
  await settle(fixture);
  return { fixture, root: fixture.nativeElement as HTMLElement, useCreatureAction };
}

const rollButtons = (root: HTMLElement) =>
  [...root.querySelectorAll('button')].filter((b) => (b.textContent ?? '').includes('Rolar dano'));

describe("NpcCard: the creature's other actions", () => {
  it('lists the breath from the stat block the card already reads, with the creatures client stubbed', async () => {
    const { root } = await open({ 'monster:hell-hound': hound });
    expect(root.querySelector('app-creature-actions')?.textContent).toContain('Sopro de fogo');
  });

  it('says the stat block could not be read and offers "Tentar de novo", the rest of the card working', async () => {
    const { root } = await open({});
    expect(root.querySelector('app-creature-actions .acts__live')?.textContent).toContain(
      'Não deu para ler as outras ações da criatura.',
    );
    const retry = [...root.querySelectorAll('app-creature-actions button')].find((b) =>
      (b.textContent ?? '').includes('Tentar de novo'),
    );
    expect(retry).toBeDefined();
  });

  it('applies the damage the action opened inside its result, once, not again in the card list', async () => {
    const { fixture, root } = await open({ 'monster:hell-hound': hound });
    expect(rollButtons(root)).toHaveLength(1); // before the use: the card's list holds it
    root.querySelector<HTMLButtonElement>('button[aria-label="Usar Sopro de fogo"]')!.click();
    await settle(fixture);
    await settle(fixture);
    expect(root.querySelector('[data-testid="creature-action-result"]')).not.toBeNull();
    expect(rollButtons(root)).toHaveLength(1); // after: inside the result, not twice
    expect(root.querySelector('[data-testid="creature-action-result"] button')).not.toBeNull();
  });
});
