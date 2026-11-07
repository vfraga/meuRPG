import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';

import type { GearVm } from '../../../../core/characters/gear';
import { CombatantSchema } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { TurnOptionsSchema } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { GearFlow } from '../../../../core/combat/gear-flow';
import { ActionGroups } from './action-groups';

const sword: GearVm = {
  key: 'equipment:longsword',
  namePt: 'Espada longa',
  kind: 'weapon',
  equipped: true,
  twoHanded: false,
  armorClassIfToggled: 0,
  count: 1,
};
const mace: GearVm = { ...sword, key: 'equipment:mace', namePt: 'Maça', equipped: false };
const shield: GearVm = {
  key: 'equipment:shield',
  namePt: 'Escudo',
  kind: 'shield',
  equipped: false,
  twoHanded: false,
  armorClassIfToggled: 17,
  count: 1,
};
const armor: GearVm = {
  key: 'equipment:chain-mail',
  namePt: 'Cota de malha',
  kind: 'armor',
  equipped: true,
  twoHanded: false,
  armorClassIfToggled: 10,
  count: 1,
};

describe('ActionGroups: "Sacar ou guardar" (#562)', () => {
  async function setup(
    over: {
      used?: boolean;
      actionUsed?: boolean;
      locked?: string;
      gear?: GearVm[];
      idle?: boolean;
      failRead?: boolean;
    } = {},
  ) {
    const fixture = TestBed.createComponent(ActionGroups);
    const asked: { key: string; equipped: boolean }[] = [];
    const flow = TestBed.runInInjectionContext(
      () =>
        new GearFlow({
          client: {
            read: () =>
              over.failRead
                ? Promise.reject(new Error('x'))
                : Promise.resolve({
                    gear: over.gear ?? [sword, mace, shield, armor],
                    armorClass: 16,
                    shieldIdle: over.idle ?? false,
                    revision: 1,
                  }),
            set: (_c, _ch, key, equipped) => {
              asked.push({ key, equipped });
              return Promise.resolve();
            },
          },
          campaignId: () => 'c1',
          characterId: () => 'ch1',
          signature: () => '',
        }),
    );
    await flow.load();
    fixture.componentRef.setInput('options', create(TurnOptionsSchema, {}));
    fixture.componentRef.setInput(
      'own',
      create(CombatantSchema, {
        movementLeftFt: 30,
        speedFt: 30,
        objectInteractionUsed: over.used ?? false,
        actionUsed: over.actionUsed ?? false,
      }),
    );
    fixture.componentRef.setInput('gearFlow', flow);
    fixture.componentRef.setInput('locked', over.locked ?? '');
    fixture.detectChanges();
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement, asked };
  }

  const buttonText = (b: HTMLElement | null): string => {
    const copy = b?.cloneNode(true) as HTMLElement | undefined;
    copy?.querySelectorAll('mat-icon').forEach((i) => i.remove());
    return copy?.textContent?.replace(/\s+/g, ' ').trim() ?? '';
  };
  const rows = (el: HTMLElement) =>
    Array.from(el.querySelectorAll('.gear-row')).map((r) => ({
      name: r.querySelector('.gear-row__name')!.textContent!.trim(),
      button: buttonText(r.querySelector('button')),
      disabled: r.querySelector('button')?.getAttribute('aria-disabled') === 'true',
      why: r.querySelector('.gear-row__line')?.textContent?.trim() ?? '',
    }));

  it('is a fold named "Sacar ou guardar" under the attacks, with the armor class in its header and the rule line', async () => {
    const { el } = await setup();
    const attacks = el.querySelector('[role="tabpanel"]')!;
    expect(attacks.textContent).toContain('Sacar ou guardar');
    expect(attacks.textContent).toContain('CA 16');
    expect(attacks.textContent).toContain(
      '1ª arma do turno: grátis; depois, a ação. Escudo: ação. Armadura: só fora do combate.',
    );
  });

  it('has the cost inside the button: "Guardar · grátis" for the first weapon, "Sacar · ação" for the shield', async () => {
    const { el } = await setup();
    const r = rows(el);
    expect(r.find((x) => x.name === 'Espada longa')!.button).toBe('Guardar grátis');
    expect(r.find((x) => x.name === 'Maça')!.button).toBe('Sacar grátis');
    expect(r.find((x) => x.name === 'Escudo')!.button).toContain('Sacar');
    expect(r.find((x) => x.name === 'Escudo')!.button).toContain('ação');
  });

  it('lists no armor: the rule line covers it', async () => {
    const { el } = await setup();
    expect(rows(el).map((x) => x.name)).not.toContain('Cota de malha');
    expect(el.querySelector('.gear-row__kind')).toBeNull();
    expect(el.querySelector('.gear-row__cost.mr-muted')).toBeNull();
  });

  it('says a second weapon uses the action once the free interaction is spent', async () => {
    const { el } = await setup({ used: true });
    expect(rows(el).find((x) => x.name === 'Maça')!.button).toBe('Sacar ação');
  });

  it('disables the button with the reason in words when the free interaction and the action are spent', async () => {
    const { el, asked } = await setup({ used: true, actionUsed: true });
    const mace = rows(el).find((x) => x.name === 'Maça')!;
    expect(mace.disabled).toBe(true);
    expect(mace.why).toBe('Você já usou a ação deste turno.');
    el.querySelector<HTMLButtonElement>('[data-gear-key="equipment:mace"]')!.click();
    expect(asked).toEqual([]);
  });

  it('keeps the button when a free interaction is left though the action is spent', async () => {
    const { el } = await setup({ actionUsed: true });
    expect(rows(el).find((x) => x.name === 'Maça')!.disabled).toBe(false);
  });

  it('asks for the opposite state of the piece', async () => {
    const { el, asked } = await setup();
    el.querySelector<HTMLButtonElement>('[data-gear-key="equipment:shield"]')!.click();
    expect(asked).toEqual([{ key: 'equipment:shield', equipped: true }]);
  });

  it('has no buttons while the turn is held for something else', async () => {
    const { el } = await setup({ locked: 'Aguardando o mestre' });
    expect(el.querySelector('.gear-row__button')).toBeNull();
  });

  it('shows no fold without gear', async () => {
    const { el } = await setup({ gear: [] });
    expect(el.textContent).not.toContain('Sacar ou guardar');
  });

  it('shows the error with "Tentar de novo" when the first read failed', async () => {
    const { el } = await setup({ failRead: true });
    expect(el.querySelector('[role="alert"]')!.textContent).toContain(
      'Não deu para ler o equipamento.',
    );
    expect(el.textContent).toContain('Tentar de novo');
  });

  describe('with a two-handed weapon in the pack and the shield in hand', () => {
    const hand: GearVm = { ...shield, equipped: true };
    const bow: GearVm = {
      ...sword,
      key: 'equipment:longbow',
      namePt: 'Arco longo',
      equipped: false,
      twoHanded: true,
      armorClassIfToggled: 14,
      count: 1,
    };

    it('warns before the drawing, marks the bow’s row and says the change in its label', async () => {
      const { el } = await setup({ gear: [hand, sword, bow] });
      expect(el.textContent).toContain(
        'Sacar Arco longo (duas mãos) deixa o escudo de lado: CA 16 → 14.',
      );
      const row = Array.from(el.querySelectorAll('.gear-row')).find((r) =>
        r.textContent?.includes('Arco longo'),
      )!;
      expect(row.querySelector('.gear-row__warn')).not.toBeNull();
      expect(row.querySelector('button')!.getAttribute('aria-label')).toContain(
        'A classe de armadura vai de 16 para 14',
      );
    });

    it('marks the shield "Parado" and the header "CA 18 → 16" once the bow is drawn', async () => {
      const drawn: GearVm = { ...bow, equipped: true, armorClassIfToggled: 18, count: 1 };
      const { el } = await setup({ gear: [hand, drawn], idle: true });
      const row = Array.from(el.querySelectorAll('.gear-row')).find((r) =>
        r.textContent?.includes('Escudo'),
      )!;
      expect(row.textContent).toContain('Parado');
      expect(el.textContent).toContain('CA 18 → 16');
    });
  });

  it('folds "Carregados (n)" when many weapons are carried', async () => {
    const many = Array.from({ length: 6 }, (_, i) => ({
      ...mace,
      key: `equipment:w${i}`,
      namePt: `Arma ${i}`,
    }));
    const { el } = await setup({ gear: [sword, ...many] });
    expect(el.textContent).toContain('Carregados (6)');
  });
});
