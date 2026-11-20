import { TestBed } from '@angular/core/testing';

import { create } from '@bufbuild/protobuf';

import { MountSavePromptSchema } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { MountSaveWait } from '../../../../core/combat/mount-save';
import { MountSaveWaitCard } from './mount-save-wait';

describe('MountSaveWaitCard', () => {
  const wait: MountSaveWait = {
    prompt: create(MountSavePromptSchema, { riderId: 'tor', mountId: 'h' }),
    title: 'Esperando o teste de Tor',
    detail: 'Teste de resistência de Destreza CD 10: o Cavalo de guerra sofreu um empurrão.',
    rollLabel: 'Rolar por Tor',
    why: 'Espera o teste de Tor',
  };

  function setup(value: MountSaveWait | null) {
    const fixture = TestBed.createComponent(MountSaveWaitCard);
    fixture.componentRef.setInput('wait', value);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  it('has its live region on the page before the card, which comes in with its own words', () => {
    const { fixture, el } = setup(null);
    const live = el.querySelector('.mr-live[role=status]')!;
    expect(live.textContent?.trim()).toBe('');
    fixture.componentRef.setInput('wait', wait);
    fixture.detectChanges();
    expect(el.querySelector('.mr-live[role=status]')).toBe(live);
    expect(live.querySelector('h3')?.textContent?.trim()).toBe('Esperando o teste de Tor');
    expect(live.querySelector('.card__detail')?.textContent?.trim()).toBe(wait.detail);
    expect(live.querySelector('.mr-notice')?.hasAttribute('role')).toBe(false);
  });

  it('asks to roll for the rider with the prompt it waits on', () => {
    const { fixture, el } = setup(wait);
    const rolled: unknown[] = [];
    fixture.componentInstance.roll.subscribe((p) => rolled.push(p));
    el.querySelector<HTMLButtonElement>('button')!.click();
    expect(el.querySelector('button')?.textContent?.trim()).toBe('Rolar por Tor');
    expect(rolled).toEqual([wait.prompt]);
  });
});
