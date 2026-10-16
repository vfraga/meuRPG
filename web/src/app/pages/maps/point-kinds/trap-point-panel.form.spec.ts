import { create } from '@bufbuild/protobuf';
import { type ComponentFixture, TestBed } from '@angular/core/testing';
import { describe, expect, it } from 'vitest';

import { MapPointKind, MapPointSchema, TrapState } from '../../../../gen/meurpg/maps/v1/maps_pb';
import {
  ListTrapPresetsResponseSchema,
  TrapSaveApplies,
  TrapTrigger,
} from '../../../../gen/meurpg/rules/v1/rules_pb';
import { MapsClient } from '../../../core/maps/maps-client';
import { FakeMapsClient } from '../../../core/maps/maps-testing';
import { SceneChecks } from '../../../core/maps/scene-actions';
import { TrapPresets } from '../../../core/traps/trap-presets';
import { nextTurn } from '../../../../test-settle';
import type { AttackDraft, ConditionDraft, DamageDraft, SaveDraft, TrapDraft } from './trap-draft';
import { TrapPointPanel } from './trap-point-panel';

const point = create(MapPointSchema, {
  id: 'trap-1',
  mapId: 'map-1',
  kind: MapPointKind.TRAP,
  name: 'Fosso',
  description: 'No corredor.',
  trap: {
    presetKey: '',
    noticeDc: 15,
    findDc: 15,
    areaSize: 1,
    trigger: TrapTrigger.ENTER,
    state: TrapState.ARMED,
    effect: { damage: [{ dice: '2d6', damageTypeKey: 'damage-type:bludgeoning' }] },
  },
});

const SKILLS = ['acrobatics', 'arcana', 'athletics', 'perception', 'religion'].map((k) => ({
  key: `skill:${k}`,
  label: k,
}));

/** The calls the template makes on the panel, one by one: each edit of a part, the keys of the skill list, the radio arrows. */
interface Panel {
  draft(): TrapDraft;
  alsoOpen(): boolean;
  alsoCursor(): number;
  noticersFailed(): boolean;
  presetsFailed(): boolean;
  areaWords(): string;
  saveSeverity(): string;
  attackSeverity(): string;
  show(): boolean;
  patch(c: Partial<TrapDraft>): void;
  text(field: 'name' | 'description' | 'noticeDc' | 'findDc', e: Event): void;
  alsoName(key: string): string;
  toggleSkill(key: string): void;
  removeSkill(key: string): void;
  openAlso(): void;
  closeAlso(focus?: boolean): void;
  onAlsoClick(e: MouseEvent): void;
  onAlsoKey(e: KeyboardEvent): void;
  addAttack(): void;
  removeAttack(): void;
  patchAttack(c: Partial<AttackDraft>): void;
  patchAttackDamage(c: Partial<DamageDraft>): void;
  addDamage(): void;
  removeDamage(i: number): void;
  patchDamage(i: number, c: Partial<DamageDraft>): void;
  addCondition(): void;
  removeCondition(i: number): void;
  patchCondition(i: number, c: Partial<ConditionDraft>): void;
  addSave(): void;
  removeSave(): void;
  patchSave(c: Partial<SaveDraft>): void;
  addFailDamage(): void;
  removeFailDamage(i: number): void;
  patchFailDamage(i: number, c: Partial<DamageDraft>): void;
  patchFailCondition(c: Partial<ConditionDraft> | null): void;
  changes(): unknown;
  discard(): void;
  onKey(e: KeyboardEvent): void;
  readNoticers(): Promise<void>;
  val(e: Event): string;
  num(e: Event): number;
  presetSub(p: unknown): string;
}

describe('TrapPointPanel, the parts of the form', () => {
  let fixture: ComponentFixture<TrapPointPanel>;
  let api: FakeMapsClient;
  const panel = () => fixture.componentInstance as unknown as Panel;
  const key = (k: string): KeyboardEvent => {
    const e = new KeyboardEvent('keydown', { key: k, cancelable: true });
    panel().onAlsoKey(e);
    return e;
  };

  async function setup(over: { presetsFail?: boolean; noticersFail?: boolean } = {}) {
    api = new FakeMapsClient();
    if (over.noticersFail) {
      api.failWith = new Error('down');
    }
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: MapsClient, useValue: api },
        {
          provide: TrapPresets,
          useValue: {
            list: () =>
              over.presetsFail
                ? Promise.reject(new Error('down'))
                : Promise.resolve(
                    create(ListTrapPresetsResponseSchema, {
                      severities: [
                        {
                          key: 'a',
                          namePt: 'Revés',
                          saveDcMin: 10,
                          saveDcMax: 11,
                          attackBonusMin: 3,
                          attackBonusMax: 5,
                        },
                      ],
                    }),
                  ),
          },
        },
        { provide: SceneChecks, useValue: { skills: () => Promise.resolve(SKILLS) } },
      ],
    });
    fixture = TestBed.createComponent(TrapPointPanel);
    fixture.componentRef.setInput('point', point);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.detectChanges();
    await nextTurn();
    await fixture.whenStable();
    fixture.detectChanges();
  }

  it('says that the presets could not be read, and that the noticers could not', async () => {
    await setup({ presetsFail: true, noticersFail: true });
    expect(panel().presetsFailed()).toBe(true);
    expect(panel().noticersFailed()).toBe(true);
  });

  it('writes the severity tables of the SRD and the words about the area', async () => {
    await setup();
    expect(panel().saveSeverity()).toBe('revés 10 a 11');
    expect(panel().attackSeverity()).toBe('revés +3 a +5');
    expect(panel().areaWords()).toBe('Só o quadrado do ponto.');
    panel().patch({ areaSize: 3 });
    expect(panel().areaWords()).toContain('Um bloco de 3 × 3 quadrados');
  });

  it('types into the text fields and reads the number of a select', async () => {
    await setup();
    const input = document.createElement('input');
    input.value = 'Novo nome';
    panel().text('name', { target: input } as unknown as Event);
    expect(panel().draft().name).toBe('Novo nome');
    expect(panel().val({ target: input } as unknown as Event)).toBe('Novo nome');
    input.value = '2';
    expect(panel().num({ target: input } as unknown as Event)).toBe(2);
  });

  it('edits the attack, and ignores an edit when there is none', async () => {
    await setup();
    panel().patchAttack({ bonus: '5' });
    panel().patchAttackDamage({ dice: '1d4' });
    expect(panel().draft().attack).toBeNull();
    panel().addAttack();
    panel().patchAttack({ bonus: '5' });
    panel().patchAttackDamage({ dice: '1d4' });
    expect(panel().draft().attack).toMatchObject({ bonus: '5', damage: { dice: '1d4' } });
    panel().removeAttack();
    expect(panel().draft().attack).toBeNull();
  });

  it('edits the damage and the condition parts by their place', async () => {
    await setup();
    panel().addDamage();
    panel().patchDamage(1, { dice: '1d8' });
    panel().patchDamage(0, { typeKey: 'damage-type:fire' });
    expect(panel().draft().damage).toEqual([
      { dice: '2d6', typeKey: 'damage-type:fire' },
      { dice: '1d8', typeKey: 'damage-type:bludgeoning' },
    ]);
    panel().removeDamage(0);
    expect(panel().draft().damage).toHaveLength(1);
    panel().addCondition();
    panel().addCondition();
    panel().patchCondition(1, { duration: '1 hora' });
    expect(
      panel()
        .draft()
        .conditions.map((c) => c.duration),
    ).toEqual(['', '1 hora']);
    panel().removeCondition(0);
    expect(panel().draft().conditions).toEqual([{ key: 'condition:prone', duration: '1 hora' }]);
  });

  it('edits the save and what a failure does, and ignores it all when there is no save', async () => {
    await setup();
    panel().patchSave({ dc: '12' });
    panel().addFailDamage();
    panel().removeFailDamage(0);
    panel().patchFailDamage(0, { dice: '1' });
    panel().patchFailCondition({});
    expect(panel().draft().save).toBeNull();
    panel().addSave();
    panel().patchSave({ dc: '12', appliesTo: TrapSaveApplies.HIT });
    panel().addFailDamage();
    panel().addFailDamage();
    panel().patchFailDamage(1, { dice: '2d6' });
    panel().patchFailDamage(0, { typeKey: 'damage-type:fire' });
    panel().removeFailDamage(0);
    panel().patchFailCondition({ key: 'condition:blinded' });
    panel().patchFailCondition({ duration: '1 min' });
    expect(panel().draft().save).toMatchObject({
      dc: '12',
      appliesTo: TrapSaveApplies.HIT,
      failDamage: [{ dice: '2d6' }],
      failCondition: { key: 'condition:blinded', duration: '1 min' },
    });
    panel().patchFailCondition(null);
    expect(panel().draft().save?.failCondition).toBeNull();
    panel().removeSave();
    expect(panel().draft().save).toBeNull();
  });

  it('does not offer a form that breaks a rule, shows the errors, and discards back to the saved point', async () => {
    await setup();
    panel().patch({ name: '' });
    expect(panel().changes()).toBeNull();
    expect(panel().show()).toBe(true);
    panel().discard();
    expect(panel().draft().name).toBe('Fosso');
    expect(panel().show()).toBe(false);
    expect(panel().changes()).toBeNull();
  });

  it('moves the skill list cursor within its bounds, with Home and End', async () => {
    await setup();
    panel().openAlso();
    expect(panel().alsoCursor()).toBe(0);
    key('ArrowUp');
    expect(panel().alsoCursor()).toBe(0);
    key('End');
    expect(panel().alsoCursor()).toBe(3);
    key('ArrowDown');
    expect(panel().alsoCursor()).toBe(3);
    key('ArrowUp');
    key('Home');
    expect(panel().alsoCursor()).toBe(0);
  });

  it('picks the skill under the cursor with Enter, and leaves other keys alone', async () => {
    await setup();
    panel().openAlso();
    expect(key('Tab').defaultPrevented).toBe(false);
    const enter = key('Enter');
    expect(enter.defaultPrevented).toBe(true);
    expect(panel().draft().alsoFind).toEqual(['skill:acrobatics']);
    expect(panel().alsoName('skill:acrobatics')).toBe('acrobatics');
    expect(panel().alsoName('skill:unknown')).toBe('skill:unknown');
    panel().removeSkill('skill:acrobatics');
    expect(panel().draft().alsoFind).toEqual([]);
  });

  it('opens and closes the list with its button, and picks by a tap on an option', async () => {
    await setup();
    panel().openAlso();
    fixture.detectChanges();
    expect(panel().alsoOpen()).toBe(true);
    const row = fixture.nativeElement.querySelectorAll('[role="option"]')[2] as HTMLElement;
    const tap = new MouseEvent('click', { bubbles: true });
    Object.defineProperty(tap, 'target', { value: row });
    panel().onAlsoClick(tap);
    expect(panel().draft().alsoFind).toEqual(['skill:athletics']);
    expect(panel().alsoCursor()).toBe(2);
    const off = new MouseEvent('click', { bubbles: true });
    Object.defineProperty(off, 'target', { value: fixture.nativeElement });
    panel().onAlsoClick(off);
    expect(panel().draft().alsoFind).toEqual(['skill:athletics']);
    panel().openAlso();
    expect(panel().alsoOpen()).toBe(false);
  });

  it('moves the choice of a segmented control with the arrows, wrapping, and ignores other keys', async () => {
    await setup();
    const group = document.createElement('div');
    const picked: number[] = [];
    for (let i = 0; i < 3; i++) {
      const b = document.createElement('button');
      b.setAttribute('role', 'radio');
      b.addEventListener('click', () => picked.push(i));
      group.append(b);
    }
    document.body.append(group);
    const buttons = Array.from(group.querySelectorAll('button'));
    const press = (k: string) => {
      const e = new KeyboardEvent('keydown', { key: k, cancelable: true });
      Object.defineProperty(e, 'currentTarget', { value: group });
      panel().onKey(e);
      return e;
    };
    expect(press('a').defaultPrevented).toBe(false);
    buttons[0].focus();
    press('ArrowRight');
    expect(document.activeElement).toBe(buttons[1]);
    press('ArrowDown');
    press('ArrowDown');
    expect(picked).toEqual([1, 2, 0]);
    press('ArrowUp');
    expect(picked.at(-1)).toBe(2);
    group.remove();
  });

  it('reads the noticers again, and keeps only the last answer', async () => {
    await setup();
    await panel().readNoticers();
    expect(panel().noticersFailed()).toBe(false);
    api.failWith = new Error('down');
    await panel().readNoticers();
    expect(panel().noticersFailed()).toBe(true);
  });
});
