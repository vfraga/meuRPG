import { create } from '@bufbuild/protobuf';
import { ComponentFixture, TestBed } from '@angular/core/testing';

import {
  type GetTrapNoticersResponse,
  GetTrapNoticersResponseSchema,
  MapPointKind,
  MapPointSchema,
  TrapNoticerSchema,
  TrapState,
} from '../../../../gen/meurpg/maps/v1/maps_pb';
import {
  Ability,
  ListTrapPresetsResponseSchema,
  TrapPassOutcome,
  TrapPresetSchema,
  TrapSaveApplies,
  TrapTargets,
  TrapTrigger,
} from '../../../../gen/meurpg/rules/v1/rules_pb';
import { MapsClient } from '../../../core/maps/maps-client';
import { FakeMapsClient } from '../../../core/maps/maps-testing';
import { SceneChecks } from '../../../core/maps/scene-actions';
import { TrapPresets } from '../../../core/traps/trap-presets';
import { TrapPointPanel } from './trap-point-panel';
import { nextTurn } from '../../../../test-settle';

const presets = create(ListTrapPresetsResponseSchema, {
  presets: [
    create(TrapPresetSchema, {
      key: 'trap:simple-pit',
      namePt: 'Fosso simples',
      descriptionPt: 'Um fosso.',
      noticeDc: 10,
      findDc: 10,
      trigger: TrapTrigger.ENTER,
      areaSize: 1,
      fallFt: 10,
      effect: {
        damage: [
          { dice: '1d6', damageTypeKey: 'damage-type:bludgeoning', damageTypePt: 'concussão' },
        ],
      },
    }),
    create(TrapPresetSchema, {
      key: 'trap:fire-statue',
      namePt: 'Estátua que cospe fogo',
      kind: 'magic',
      descriptionPt: 'Uma estátua.',
      noticeDc: 15,
      findDc: 15,
      trigger: TrapTrigger.ENTER,
      areaSize: 1,
    }),
    create(TrapPresetSchema, {
      key: 'trap:poison-needle',
      namePt: 'Agulha envenenada',
      descriptionPt: 'Uma agulha na fechadura.',
      findDc: 20,
      trigger: TrapTrigger.MANUAL,
      areaSize: 1,
      effect: {
        targets: TrapTargets.MANUAL,
        damage: [
          { dice: '1', damageTypeKey: 'damage-type:piercing', damageTypePt: 'perfurante' },
          { dice: '2d10', damageTypeKey: 'damage-type:poison', damageTypePt: 'veneno' },
        ],
        save: {
          ability: Ability.CONSTITUTION,
          dc: 15,
          appliesTo: TrapSaveApplies.CAUGHT,
          onFail: { condition: { conditionKey: 'condition:poisoned', durationPt: '1 hora' } },
          onPass: TrapPassOutcome.NONE,
        },
      },
    }),
  ],
  severities: [
    {
      key: 'setback',
      namePt: 'Revés',
      saveDcMin: 10,
      saveDcMax: 11,
      attackBonusMin: 3,
      attackBonusMax: 5,
    },
    {
      key: 'dangerous',
      namePt: 'Perigosa',
      saveDcMin: 12,
      saveDcMax: 15,
      attackBonusMin: 6,
      attackBonusMax: 8,
    },
    {
      key: 'deadly',
      namePt: 'Mortal',
      saveDcMin: 16,
      saveDcMax: 20,
      attackBonusMin: 9,
      attackBonusMax: 12,
    },
  ],
});

const trapPoint = create(MapPointSchema, {
  id: 'trap-1',
  mapId: 'map-1',
  kind: MapPointKind.TRAP,
  name: 'Fosso escondido',
  description: 'No corredor.',
  trap: {
    presetKey: 'trap:simple-pit',
    noticeDc: 15,
    findDc: 15,
    areaSize: 2,
    trigger: TrapTrigger.ENTER,
    state: TrapState.ARMED,
    effect: { damage: [{ dice: '2d6', damageTypeKey: 'damage-type:bludgeoning' }] },
  },
});

// The rules' skills as `SceneChecks` sends them, alphabetical: the 18 of the SRD (the panel offers 16 of them).
const SKILLS = [
  ['acrobatics', 'Acrobacia'],
  ['animal-handling', 'Adestrar Animais'],
  ['arcana', 'Arcanismo'],
  ['athletics', 'Atletismo'],
  ['performance', 'Atuação'],
  ['deception', 'Enganação'],
  ['stealth', 'Furtividade'],
  ['history', 'História'],
  ['intimidation', 'Intimidação'],
  ['investigation', 'Investigação'],
  ['medicine', 'Medicina'],
  ['nature', 'Natureza'],
  ['perception', 'Percepção'],
  ['insight', 'Intuição'],
  ['persuasion', 'Persuasão'],
  ['religion', 'Religião'],
  ['sleight-of-hand', 'Prestidigitação'],
  ['survival', 'Sobrevivência'],
]
  .map(([key, label]) => ({ key: `skill:${key}`, label }))
  .sort((a, b) => a.label.localeCompare(b.label, 'pt-BR'));

const noticers = create(GetTrapNoticersResponseSchema, {
  noticeDc: 15,
  noticers: [
    create(TrapNoticerSchema, {
      characterId: 'c1',
      characterName: 'Sálvia',
      passivePerception: 16,
      lightPenalty: 0,
      onMap: true,
      inRange: true,
      sees: true,
      wouldNotice: true,
      passesDc: true,
    }),
    create(TrapNoticerSchema, {
      characterId: 'c2',
      characterName: 'Toren',
      passivePerception: 11,
      lightPenalty: -5,
      onMap: true,
      inRange: false,
      passesDc: false,
    }),
  ],
});

describe('TrapPointPanel', () => {
  let fixture: ComponentFixture<TrapPointPanel>;
  let el: HTMLElement;
  let api: FakeMapsClient;

  async function setup(point = trapPoint, answer: GetTrapNoticersResponse | null = noticers) {
    api = new FakeMapsClient();
    api.noticers = answer;
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: MapsClient, useValue: api },
        { provide: TrapPresets, useValue: { list: () => Promise.resolve(presets) } },
        { provide: SceneChecks, useValue: { skills: () => Promise.resolve(SKILLS) } },
      ],
    });
    fixture = TestBed.createComponent(TrapPointPanel);
    fixture.componentRef.setInput('point', point);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.detectChanges();
    el = fixture.nativeElement;
    await settle();
  }
  const settle = async () => {
    fixture.detectChanges();
    await nextTurn();
    await fixture.whenStable();
    fixture.detectChanges();
  };
  const text = () => (el.textContent ?? '').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ');
  const button = (t: string) =>
    Array.from(el.querySelectorAll<HTMLElement>('button')).find((b) =>
      b.textContent?.trim().endsWith(t),
    )!;
  const radio = (t: string) =>
    Array.from(el.querySelectorAll<HTMLElement>('[role="radio"]')).find((b) =>
      b.textContent?.trim().includes(t),
    )!;
  const field = (label: string) => {
    const f = Array.from(el.querySelectorAll('mat-form-field')).find(
      (x) => x.querySelector('mat-label')?.textContent?.trim() === label,
    )!;
    return f.querySelector('input, textarea, select') as HTMLInputElement;
  };
  function type(label: string, value: string): void {
    const input = field(label);
    input.value = value;
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
  }
  const panel = () => fixture.componentInstance;

  it('lists the eight presets with a line each, and "Começar do zero"', async () => {
    await setup();
    expect(text()).toContain('Predefinições do SRD');
    expect(radio('Fosso simples').textContent).toContain('Queda de 3\u00a0m, 1d6');
    expect(radio('Agulha envenenada').textContent).toContain(
      '1 perfurante, 2d10 veneno · resistência de Constituição',
    );
    expect(button('Começar do zero')).toBeTruthy();
    expect(radio('Fosso simples').getAttribute('aria-checked')).toBe('true');
  });

  it('fills the form from a preset: the numbers, the trigger and the parts, all editable', async () => {
    await setup();
    radio('Agulha envenenada').click();
    await settle();
    expect(field('Nome').value).toBe('Agulha envenenada');
    expect(field('CD para notar (Percepção)').value).toBe('');
    expect(field('CD para achar (Investigação)').value).toBe('20');
    expect(text()).toContain('Dano que sempre acontece');
    expect(text()).toContain('Teste de resistência');
    expect(text()).toContain(
      'CDs de resistência do SRD: revés 10 a 11 · perigosa 12 a 15 · mortal 16 a 20.',
    );
    // The severity words judge nothing: there is no "A sua é perigosa".
    expect(text()).not.toMatch(/a sua,? .* é/i);
    expect(radio('Manual').getAttribute('aria-checked')).toBe('true');
    type('CD para achar (Investigação)', '17');
    expect(field('CD para achar (Investigação)').value).toBe('17');
  });

  it('says the image is visible to the players and keeps the DC labels of the ruling', async () => {
    await setup();
    expect(text()).toContain('Não desenhe a armadilha na imagem: os jogadores veem a imagem.');
    expect(text()).toContain('CD para notar (Percepção)');
    expect(text()).toContain('CD para achar (Investigação)');
  });

  it('adds and removes the parts of the effect as text actions, each part on its own', async () => {
    await setup(trapPoint);
    expect(el.querySelectorAll('[role="group"][aria-labelledby^="part-damage-"]')).toHaveLength(1);
    expect(text()).toContain('Acrescentar ao efeito');
    el.querySelector<HTMLElement>(
      'button[aria-label="Acrescentar dano que sempre acontece"]',
    )!.click();
    await settle();
    expect(el.querySelectorAll('[role="group"][aria-labelledby^="part-damage-"]')).toHaveLength(2);
    button('Ataque').click();
    el.querySelector<HTMLElement>('button[aria-label="Acrescentar teste de resistência"]')!.click();
    await settle();
    expect(el.querySelector('#part-attack')).not.toBeNull();
    expect(el.querySelector('#part-save')).not.toBeNull();
    expect(text()).toContain(
      'Bônus de ataque do SRD: revés +3 a +5 · perigosa +6 a +8 · mortal +9 a +12.',
    );
    el.querySelector<HTMLElement>('button[aria-label="Remover a parte Ataque"]')!.click();
    await settle();
    expect(el.querySelector('#part-attack')).toBeNull();
    el.querySelector<HTMLElement>(
      'button[aria-label="Remover a parte Dano que sempre acontece 2"]',
    )!.click();
    await settle();
    expect(el.querySelectorAll('[role="group"][aria-labelledby^="part-damage-"]')).toHaveLength(1);
  });

  it('is clean until something changes, then offers the whole spec', async () => {
    const dirty: boolean[] = [];
    await setup();
    panel().dirtyChange.subscribe((d) => dirty.push(d));
    expect(panel().changes()).toBeNull();
    type('CD para achar (Investigação)', '12');
    await settle();
    expect(dirty.at(-1)).toBe(true);
    const changes = panel().changes();
    expect(changes?.trap).toMatchObject({
      findDc: 12,
      noticeDc: 15,
      areaSize: 2,
      presetKey: 'trap:simple-pit',
    });
    panel().discard();
    await settle();
    expect(field('CD para achar (Investigação)').value).toBe('15');
  });

  it('says what is wrong in the field, after the first try to save', async () => {
    await setup();
    type('CD para achar (Investigação)', '40');
    type('Nome', '   ');
    expect(text()).not.toContain('Use uma CD de 1 a 30.');
    expect(panel().changes()).toBeNull();
    await settle();
    expect(text()).toContain('Use uma CD de 1 a 30.');
    expect(text()).toContain('Dê um nome ao ponto.');
  });

  it('says a dice that is not a die', async () => {
    await setup();
    const dice = field('Dano');
    dice.value = '2d20';
    dice.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(panel().changes()).toBeNull();
    await settle();
    expect(text()).toContain('Use dados como 2d6');
  });

  it('shows "Quem notaria" from the server\'s numbers (the 9.14 table)', async () => {
    await setup();
    expect(api.calls).toContain('getTrapNoticers map-1 trap-1');
    expect(text()).toContain('Quem notaria');
    expect(text()).toContain('Sálvia');
    expect(text()).toContain('Se chegar a 3 m: não nota');
  });

  it('a trap with no DC to notice says nobody notices it alone', async () => {
    await setup(trapPoint, create(GetTrapNoticersResponseSchema, { noticeDc: 0, noticers: [] }));
    expect(text()).toContain('Ninguém nota esta armadilha sozinho');
  });

  it('has the state as three choices', async () => {
    await setup();
    expect(radio('Armada').getAttribute('aria-checked')).toBe('true');
    radio('Desarmada').click();
    await settle();
    expect(panel().changes()?.trap?.state).toBe(TrapState.DISARMED);
  });

  describe('"Também acham com" (PM-03c)', () => {
    const also = () => el.querySelector('.tp__also')!;
    const options = () =>
      Array.from(el.querySelectorAll<HTMLElement>('[role="option"]'), (o) =>
        o.textContent?.replace(/^check/, '').trim(),
      );
    const chips = () => Array.from(el.querySelectorAll('.tp__chip b'), (b) => b.textContent);
    const open = async () => {
      (el.querySelector('.tp__add') as HTMLButtonElement).click();
      await settle();
    };

    it('says "Nenhuma perícia a mais" by default, and sends an empty list', async () => {
      await setup();
      expect(also().textContent).toContain('Também acham com');
      expect(also().textContent).toContain('Nenhuma perícia a mais');
      expect(el.querySelector('.tp__add')?.textContent?.replace(/\s+/g, ' ').trim()).toContain(
        'Acrescentar perícia',
      );
      expect(text()).toContain(
        'Usam a CD para achar. O jogador procura com qualquer perícia; só as daqui (e Percepção e Investigação) acham esta armadilha. Você decide: o app não presume nada.',
      );
      type('CD para achar (Investigação)', '16');
      expect(panel().changes()?.trap?.alsoFindSkillKeys).toEqual([]);
    });

    it('opens a multi-select list of the 16 skills that are not Percepção or Investigação, alphabetical', async () => {
      await setup();
      await open();
      const list = el.querySelector('[role="listbox"]')!;
      expect(list.getAttribute('aria-multiselectable')).toBe('true');
      expect(options()).toHaveLength(16);
      expect(options()[0]).toBe('Acrobacia');
      expect(options()).not.toContain('Percepção');
      expect(options()).not.toContain('Investigação');
      expect(text()).toContain('Percepção e Investigação já valem');
      expect(el.querySelector('.tp__add')?.getAttribute('aria-expanded')).toBe('true');
    });

    it('picks and puts back a skill as a chip with "×", and sends the list in the whole spec', async () => {
      await setup();
      await open();
      const option = (name: string) =>
        Array.from(el.querySelectorAll<HTMLElement>('[role="option"]')).find((o) =>
          o.textContent?.includes(name),
        )!;
      option('Religião').click();
      option('Arcanismo').click();
      await settle();
      // Kept in the list's order, whichever was picked first.
      expect(chips()).toEqual(['Arcanismo', 'Religião']);
      expect(option('Arcanismo').getAttribute('aria-selected')).toBe('true');
      expect(el.querySelector('.tp__none')).toBeNull();
      expect(panel().changes()?.trap?.alsoFindSkillKeys).toEqual([
        'skill:arcana',
        'skill:religion',
      ]);
      (el.querySelector('[aria-label="Tirar Arcanismo"]') as HTMLButtonElement).click();
      await settle();
      expect(chips()).toEqual(['Religião']);
      expect(panel().changes()?.trap?.alsoFindSkillKeys).toEqual(['skill:religion']);
    });

    it('moves with the arrows, picks with Space and closes with Escape, giving the focus back to the button', async () => {
      await setup();
      await open();
      const list = el.querySelector<HTMLElement>('[role="listbox"]')!;
      const key = (k: string) => {
        list.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true }));
        fixture.detectChanges();
      };
      key('ArrowDown');
      key('ArrowDown');
      expect(list.getAttribute('aria-activedescendant')).toBe('tf-also-2');
      key(' ');
      expect(chips()).toEqual(['Arcanismo']);
      key('ArrowUp');
      key('Escape');
      expect(el.querySelector('[role="listbox"]')).toBeNull();
      expect(document.activeElement).toBe(el.querySelector('.tp__add'));
    });

    it('reads the saved list into the chips', async () => {
      await setup(
        create(MapPointSchema, {
          ...trapPoint,
          trap: {
            presetKey: 'trap:simple-pit',
            noticeDc: 15,
            findDc: 15,
            areaSize: 2,
            alsoFindSkillKeys: ['skill:arcana', 'skill:religion'],
          } as never,
        }),
      );
      expect(chips()).toEqual(['Arcanismo', 'Religião']);
      expect(panel().changes()).toBeNull();
    });

    it('reminds a magic preset of Arcanismo without marking anything, and drops the line once it is a chip', async () => {
      await setup();
      radio('Estátua que cospe fogo').click();
      await settle();
      expect(text()).toContain(
        'O SRD diz que qualquer personagem pode tentar um teste de Inteligência (Arcanismo)',
      );
      expect(chips()).toEqual([]);
      await open();
      (
        Array.from(el.querySelectorAll<HTMLElement>('[role="option"]')).find((o) =>
          o.textContent?.includes('Arcanismo'),
        ) as HTMLElement
      ).click();
      await settle();
      expect(text()).not.toContain('O SRD diz que qualquer personagem');
    });

    it('has no reminder for a mechanical preset', async () => {
      await setup();
      radio('Agulha envenenada').click();
      await settle();
      expect(text()).not.toContain('O SRD diz que qualquer personagem');
    });
  });
});
