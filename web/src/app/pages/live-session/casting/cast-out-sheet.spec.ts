import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CastingEffectKind,
  CastingReachSchema,
  GetCastOptionsResponseSchema,
  OutsideCastStatus,
} from '../../../../gen/meurpg/play/v1/casting_pb';
import { CastingClient } from '../../../core/casting/casting-client';
import {
  FakeCastingClient,
  castingSpell,
  castingTarget,
  outsideCast,
} from '../../../core/casting/casting-testing';
import { CastOutSheet, type CastOutData } from './cast-out-sheet';
import { nextTurn } from '../../../../test-settle';

const plain = (s: string | null | undefined) => (s ?? '').replace(/\s+/g, ' ').trim();

describe('CastOutSheet (MR-048)', () => {
  async function setup(build: (api: FakeCastingClient) => void, data: Partial<CastOutData> = {}) {
    const api = new FakeCastingClient();
    build(api);
    const close = vi.fn();
    TestBed.configureTestingModule({
      providers: [
        { provide: CastingClient, useValue: api },
        { provide: MatDialogRef, useValue: { close } },
        {
          provide: MAT_DIALOG_DATA,
          useValue: {
            campaignId: 'c',
            casterId: 'ilaria',
            casterName: 'Ilaria',
            master: false,
            diceMode: DiceMode.APP,
            preference: DicePreference.APP,
            ...data,
          } satisfies CastOutData,
        },
      ],
    });
    const fixture = TestBed.createComponent(CastOutSheet);
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      for (let i = 0; i < 4; i++) {
        await fixture.whenStable();
        await nextTurn();
        fixture.detectChanges();
      }
    };
    fixture.detectChanges();
    await settle();
    const click = async (selector: string, text?: string) => {
      const all = Array.from(el.querySelectorAll<HTMLElement>(selector));
      const target = text ? all.find((e) => plain(e.textContent).includes(text)) : all[0];
      expect(target, `${selector} ${text ?? ''}`).toBeTruthy();
      target!.click();
      await settle();
    };
    return { el, api, close, settle, click };
  }

  const cure = castingSpell('spell:cure-wounds', 'Curar Ferimentos', 1, {
    effect: CastingEffectKind.HEAL,
    rollsDice: true,
    rollDice: '1d8',
    maxTargets: 1,
    rangeKind: 'touch',
    reach: [
      create(CastingReachSchema, { characterId: 'toren', inRange: true }),
      create(CastingReachSchema, {
        characterId: 'brisa',
        inRange: false,
        reasonPt: 'Fora do alcance do toque (1,5 m).',
      }),
    ],
  });
  const alarm = castingSpell('spell:alarm', 'Alarme', 1, {
    ritualAllowed: true,
    ritualMinutes: 11,
    castingMinutes: 1,
    castingTimePt: '1 minuto',
    effect: CastingEffectKind.NARRATED,
  });
  const people = [
    castingTarget('toren', 'Toren'),
    castingTarget('brisa', 'Brisa'),
    castingTarget('ilaria', 'Ilaria'),
  ];

  function withSpells(...spells: ReturnType<typeof castingSpell>[]) {
    return (api: FakeCastingClient) => {
      api.options_ = create(GetCastOptionsResponseSchema, { spells, targets: people });
    };
  }

  it('lists the spells and, once one is picked, asks for a slot and one target as a radio group', async () => {
    const { el, click } = await setup(withSpells(cure, alarm));
    expect(plain(el.querySelector('.cap')?.textContent)).toBe('Magia');
    await click('label.row', 'Curar Ferimentos');
    expect(plain(el.querySelector('.frame__title')?.textContent)).toBe('Curar Ferimentos');
    expect(el.querySelectorAll('input[type="radio"]').length).toBeGreaterThan(1);
    expect(plain(el.textContent)).toContain('Quem você toca (uma criatura, até 1,5 m)');
  });

  it('disables a target out of reach and says why', async () => {
    const { el, click } = await setup(withSpells(cure));
    await click('label.row', 'Curar Ferimentos');
    const brisa = Array.from(el.querySelectorAll('label.row')).find((r) =>
      r.textContent?.includes('Brisa'),
    );
    expect(brisa?.classList.contains('row--off')).toBe(true);
    expect(plain(brisa?.textContent)).toContain('Fora do alcance do toque');
  });

  it('casts with the slot and the target under one key, and shows what it did', async () => {
    const { el, api, click } = await setup(withSpells(cure), {});
    api.cast_ = outsideCast({
      spellNamePt: 'Curar Ferimentos',
      status: OutsideCastStatus.ENDED,
      targets: [{ characterId: 'toren', name: 'Toren', effect: 2, amount: 9 }],
    });
    await click('label.row', 'Curar Ferimentos');
    await click('label.row', '1º nível');
    await click('label.row', 'Toren');
    expect(plain(el.querySelector('.missing')?.textContent ?? '')).toBe('');
    await click('.roll-picker, button', 'Conjurar e rolar no app');
    expect(api.casts).toHaveLength(1);
    const { req, key } = api.casts[0];
    expect(req).toMatchObject({
      spellKey: 'spell:cure-wounds',
      asRitual: false,
      slot: { level: 1, pact: false },
      targetIds: ['toren'],
      dice: { inApp: true },
    });
    expect(key).toBeTruthy();
    expect(plain(el.textContent)).toContain('Toren recuperou 9 PV.');
  });

  it('a summoning spell (Convocar Familiar) asks for no target and hands over to the creature sheet', async () => {
    const familiar = castingSpell('spell:find-familiar', 'Convocar Familiar', 1, {
      summons: true,
      effect: CastingEffectKind.SUMMON,
      ritualAllowed: true,
      ritualMinutes: 70,
      castingMinutes: 60,
      castingTimePt: '1 hora',
      maxTargets: 1,
      rangeKind: 'ranged',
      rangeFt: 10,
    });
    const { el, api, click } = await setup(withSpells(familiar));
    await click('label.row', 'Convocar Familiar');
    const text = plain(el.textContent);
    expect(text).toContain('não tem alvo');
    expect(text).not.toContain('Quem recebe');
    expect(el.querySelector('input[type="radio"][name]')).toBeNull();
    expect(plain(el.querySelector('button.cast')?.textContent)).toBe('Escolher a criatura');
    expect(api.casts).toHaveLength(0);
  });

  it('casts a ritual with no slot, 10 minutes more', async () => {
    const { el, api, click } = await setup(withSpells(alarm));
    await click('label.row', 'Alarme');
    expect(plain(el.textContent)).toContain('Como ritual');
    await click('label.row', 'Como ritual');
    expect(plain(el.textContent)).toContain('1 minuto + 10 = 11 minutos');
    expect(el.querySelector('app-slot-picker')).toBeNull();
    await click('button.cast');
    expect(api.casts[0].req).toMatchObject({ asRitual: true, slot: null });
  });

  it('a spell that is only a ritual for this caster says so, with its time, and spends no slot', async () => {
    const book = castingSpell('spell:alarm', 'Alarme', 1, {
      canCast: false,
      ritualAllowed: true,
      ritualMinutes: 11,
      castingMinutes: 1,
      castingTimePt: '1 minuto',
      slots: [],
    });
    const { el, api, click } = await setup(withSpells(book));
    await click('label.row', 'Alarme');
    const way = plain(el.querySelector('app-choice-cards:nth-of-type(1)')?.textContent);
    expect(way).toContain('Como ritual');
    expect(plain(el.textContent)).toContain('1 minuto + 10 = 11 minutos');
    expect(el.querySelector('app-slot-picker')).toBeNull();
    await click('button.cast');
    expect(api.casts[0].req).toMatchObject({ asRitual: true, slot: null });
  });

  it('asks before a concentration ends another one, and casts nothing if cancelled', async () => {
    const bless = castingSpell('spell:bless', 'Bênção', 1, {
      lasts: true,
      durationSeconds: 60,
      maxTargets: 3,
      targetsPerLevel: 1,
    });
    bless.spell!.concentration = true;
    const { api, click, settle } = await setup((a) => {
      withSpells(bless)(a);
      a.options_.concentrating = outsideCast({ spellNamePt: 'Escudo da Fé', concentrating: true });
    });
    await click('label.row', 'Bênção');
    await click('label.row', '1º nível');
    await click('label.row', 'Toren');
    await click('button.cast');
    const dialog = document.querySelector('app-cast-confirm-sheet');
    expect(plain(dialog?.textContent)).toContain('Isso encerra Escudo da Fé');
    expect(api.casts).toHaveLength(0);
    (
      Array.from(dialog!.querySelectorAll('button')).find((b) =>
        b.textContent?.includes('Cancelar'),
      ) as HTMLElement
    ).click();
    await settle();
    expect(api.casts).toHaveLength(0);
  });

  it('says a refusal in words and keeps the sheet open', async () => {
    const { el, api, click } = await setup(withSpells(alarm));
    api.failWith = new ConnectError('boom', Code.Internal);
    await click('label.row', 'Alarme');
    await click('label.row', '1º nível');
    await click('button.cast');
    const alert = plain(el.querySelector('[role="alert"]')?.textContent);
    expect(alert).not.toBe('');
    expect(alert).not.toContain('boom');
  });

  it('a master picks who casts before anything else', async () => {
    const { el, api, click } = await setup(withSpells(alarm), {
      master: true,
      casterId: '',
      casterName: '',
      npcCasters: [{ id: 'lich', name: 'Lich', detail: 'Mago 18' }],
    });
    expect(plain(el.querySelector('.cap')?.textContent)).toBe('Quem conjura');
    await click('label.row', 'Lich');
    expect(plain(el.querySelector('.cap')?.textContent)).toBe('Magia');
    expect(api.casts).toHaveLength(0);
  });

  it('asks which ability Aprimorar Habilidade is for and casts with it', async () => {
    const enhance = castingSpell('spell:enhance-ability', 'Aprimorar Habilidade', 2, {
      lasts: true,
      durationSeconds: 3600,
      maxTargets: 1,
      rangeKind: 'touch',
    });
    enhance.spell!.concentration = true;
    const { el, api, click } = await setup(withSpells(enhance));
    await click('label.row', 'Aprimorar Habilidade');
    await click('label.row', '2º nível');
    await click('label.row', 'Toren');
    expect(plain(el.querySelector('.missing')?.textContent)).toContain('Escolha a habilidade.');
    expect(plain(el.querySelector('app-ability-picker')?.textContent)).toContain('Águia');
    expect(el.querySelector('button.cast')?.getAttribute('aria-disabled')).toBe('true');
    await click('app-ability-picker label.row', 'Carisma');
    expect(el.querySelector('.missing')).toBeNull();
    await click('button.cast');
    expect(api.casts[0].req).toMatchObject({
      spellKey: 'spell:enhance-ability',
      abilityKey: 'cha',
    });
  });

  it('sends no ability for a spell that does not ask for one', async () => {
    const { el, api, click } = await setup(withSpells(alarm));
    await click('label.row', 'Alarme');
    await click('label.row', '1º nível');
    expect(el.querySelector('app-ability-picker')).toBeNull();
    await click('button.cast');
    expect(api.casts[0].req.abilityKey).toBeUndefined();
  });
});
