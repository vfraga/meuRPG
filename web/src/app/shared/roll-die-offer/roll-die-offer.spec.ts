import { ChangeDetectionStrategy, Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';

import { type RollDie, RollDieSchema } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { DieUses } from '../../core/effects/roll-die';
import { RollDieOffer } from './roll-die-offer';

const die = (key: string, name: string, sides: number, resource = 'dados_de_vigor') =>
  create(RollDieSchema, { key, namePt: name, applies: 'attack', sides, resource });
const certeiro = die('golpe', 'Golpe Certeiro', 8);
const firme = die('firme', 'Ataque Firme', 8);

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RollDieOffer],
  template: `<app-roll-die-offer
    [dice]="dice()"
    [uses]="uses()"
    [exclusive]="exclusive()"
    [legend]="legend()"
    [(picked)]="picked"
  />`,
})
class Host {
  readonly dice = signal<readonly RollDie[]>([certeiro]);
  readonly uses = signal<Readonly<Record<string, DieUses>>>({});
  readonly exclusive = signal(false);
  readonly legend = signal('');
  readonly picked = signal('');
}

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();

function mount() {
  const f = TestBed.createComponent(Host);
  f.detectChanges();
  const el = f.nativeElement as HTMLElement;
  return {
    f,
    el,
    host: f.componentInstance,
    inputs: () => Array.from(el.querySelectorAll<HTMLInputElement>('input')),
  };
}

describe('RollDieOffer', () => {
  it('keeps the checkboxes of the scene and the save sheets: the cost, and one die at a time', () => {
    const { f, el, host, inputs } = mount();
    host.dice.set([certeiro, firme]);
    f.detectChanges();
    expect(inputs().map((i) => i.type)).toEqual(['checkbox', 'checkbox']);
    expect(plain(el.textContent)).toContain('d8 · gasta 1 uso de dados de vigor');
    expect(el.querySelector('legend')?.classList.contains('mr-visually-hidden')).toBe(true);
    inputs()[0].click();
    f.detectChanges();
    expect(host.picked()).toBe('golpe');
    inputs()[1].click();
    f.detectChanges();
    expect(host.picked()).toBe('firme');
    inputs()[1].click();
    f.detectChanges();
    expect(host.picked()).toBe('');
  });

  it('draws nothing without a die', () => {
    const { f, el, host } = mount();
    host.dice.set([]);
    f.detectChanges();
    expect(el.querySelector('fieldset')).toBeNull();
  });

  it('says the pool and the uses left on one line, under a visible legend', () => {
    const { f, el, host } = mount();
    host.uses.set({
      golpe: { resource: 'dados_de_vigor', left: 2, resourceNamePt: 'Reserva de Golpes' },
    });
    host.legend.set('Dados das suas características');
    f.detectChanges();
    const text = plain(el.textContent);
    expect(text).toContain('Dados das suas características');
    expect(text).toContain('Usar Golpe Certeiro');
    expect(text).toContain('d8 somado ao ataque · gasta 1 de Reserva de Golpes · restam 2');
    expect(el.querySelector('legend')?.classList.contains('mr-visually-hidden')).toBe(false);
    expect(el.querySelector('.offer__none')).toBeNull();
  });

  it('turns a die with no use left off, and says why under the group', () => {
    const { f, el, host, inputs } = mount();
    host.uses.set({
      golpe: { resource: 'dados_de_vigor', left: 0, resourceNamePt: 'Reserva de Golpes' },
    });
    f.detectChanges();
    expect(inputs()[0].disabled).toBe(true);
    expect(el.querySelector('.opt--off')).not.toBeNull();
    expect(plain(el.textContent)).toContain('sem usos');
    expect(plain(el.querySelector('.offer__none')?.textContent)).toContain(
      'blockSem usos de Reserva de Golpes.',
    );
    const why = el.querySelector('.offer__none')!;
    expect(inputs()[0].getAttribute('aria-describedby')).toContain(why.id);
    inputs()[0].click();
    f.detectChanges();
    expect(host.picked()).toBe('');
  });

  it('keeps two pools of the same name apart, each with its own line and its own "why"', () => {
    const { f, el, host, inputs } = mount();
    const outro = die('outro', 'Ataque Outro', 6, 'dados_de_posto');
    host.dice.set([certeiro, outro]);
    host.uses.set({
      golpe: { resource: 'dados_de_vigor', left: 0, resourceNamePt: 'Reserva' },
      outro: { resource: 'dados_de_posto', left: 0, resourceNamePt: 'Reserva' },
    });
    f.detectChanges();
    const lines = [...el.querySelectorAll('.offer__none')];
    expect(lines).toHaveLength(2);
    expect(inputs()[0].getAttribute('aria-describedby')).toContain(lines[0].id);
    expect(inputs()[1].getAttribute('aria-describedby')).toContain(lines[1].id);
  });

  it('is a radio group with "Nenhum" first and chosen when exclusive with two dice', () => {
    const { f, el, host, inputs } = mount();
    host.dice.set([certeiro, firme]);
    host.exclusive.set(true);
    host.uses.set({
      golpe: { resource: 'dados_de_vigor', left: 2, resourceNamePt: 'Reserva de Golpes' },
      firme: { resource: 'dados_de_vigor', left: 2, resourceNamePt: 'Reserva de Golpes' },
    });
    f.detectChanges();
    expect(inputs().map((i) => i.type)).toEqual(['radio', 'radio', 'radio']);
    expect(new Set(inputs().map((i) => i.name)).size).toBe(1);
    expect(plain(el.querySelector('.opt')?.textContent)).toContain('Nenhum');
    // One line on the card; the meaning of "Nenhum" is read from the group.
    const hint = el.querySelector('.mr-visually-hidden[id$="-nenhum"]')!;
    expect(plain(hint.textContent)).toBe('O ataque sai sem dado de manobra.');
    expect(el.querySelector('fieldset')?.getAttribute('aria-describedby')).toBe(hint.id);
    expect(plain(el.querySelector('.opt')?.textContent)).toBe('Nenhum');
    expect(inputs().map((i) => i.checked)).toEqual([true, false, false]);
    inputs()[2].click();
    f.detectChanges();
    expect(host.picked()).toBe('firme');
    // A radio stays on its die: ticking it again does not clear it; "Nenhum" does.
    inputs()[0].click();
    f.detectChanges();
    expect(host.picked()).toBe('');
    expect(inputs().map((i) => i.checked)).toEqual([true, false, false]);
  });

  it('keeps one checkbox when exclusive with a single die', () => {
    const { f, host, inputs } = mount();
    host.exclusive.set(true);
    f.detectChanges();
    expect(inputs().map((i) => i.type)).toEqual(['checkbox']);
  });
});
