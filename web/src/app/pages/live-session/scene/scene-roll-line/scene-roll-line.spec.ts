import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import { SceneClient } from '../../../../core/play/scene-client';
import { SceneState } from '../../../../core/play/scene-state';
import { FakeSceneClient, masterScene, sceneRoll } from '../../../../core/play/scene-testing';
import { SceneRollLine } from './scene-roll-line';

const flat = (e: Element | null | undefined) => e?.textContent?.replace(/\s+/g, ' ').trim();
/** The words of an element, without its icons' ligature names. */
const words = (e: Element | null | undefined) => {
  if (!e) {
    return undefined;
  }
  const copy = e.cloneNode(true) as Element;
  copy.querySelectorAll('mat-icon').forEach((i) => i.remove());
  return flat(copy);
};

/** Toren failed Seguir os rastros (CD 13, 1 attempt): he has none left. */
const failed = sceneRoll('r1', 'a2', 'Toren', 7, { passed: false, attemptsLeft: 0 });

describe('SceneRollLine, "Dar mais uma tentativa" (MR-015, question 55)', () => {
  async function setup(scene = masterScene([failed]), roll = failed) {
    const api = new FakeSceneClient();
    api.granted = masterScene([failed], [], {});
    const state = new SceneState(
      () => api.get(),
      () => true,
    );
    state.apply(scene);
    TestBed.configureTestingModule({ providers: [{ provide: SceneClient, useValue: api }] });
    const fixture = TestBed.createComponent(SceneRollLine);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('state', state);
    fixture.componentRef.setInput('scene', scene);
    fixture.componentRef.setInput('roll', roll);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      for (let i = 0; i < 4; i++) {
        await fixture.whenStable();
        fixture.detectChanges();
      }
    };
    return { fixture, api, state, el, settle };
  }
  const opener = (el: HTMLElement) => el.querySelector<HTMLButtonElement>('.rl__grant');
  const button = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll('button')).find((b) => words(b) === label)!;

  it('puts the action on a failed roll of a player with no attempts left, named for the player and the action', async () => {
    const { el } = await setup();
    expect(words(opener(el))).toBe('Dar mais uma tentativa');
    expect(opener(el)?.getAttribute('aria-label')).toBe(
      'Dar mais uma tentativa a Toren em Seguir os rastros dos goblins',
    );
    expect(words(el.querySelector('.rl__line'))).toBe('Não passou · CD 13 Tentativa 1 de 1');
  });

  it('is not there for a roll that passed with the DC shown, nor for one with attempts left', async () => {
    const passed = sceneRoll('r2', 'a1', 'Pensantus', 17, { passed: true, attemptsLeft: 0 });
    expect(
      opener((await setup(masterScene([passed], [], { showDc: true }), passed)).el),
    ).toBeNull();
    TestBed.resetTestingModule();
    const left = sceneRoll('r3', 'a4', 'Pensantus', 9, { attemptsLeft: 2 });
    expect(opener((await setup(masterScene([left]), left)).el)).toBeNull();
  });

  it('asks in place first: a warm question that names who and which action, "Voltar" first and focused, the only filled button second', async () => {
    const { fixture, el, settle } = await setup();
    opener(el)!.click();
    await settle();
    const ask = el.querySelector('[role="alertdialog"]')!;
    expect(words(ask.querySelector('h4'))).toBe('Dar mais uma tentativa a Toren?');
    expect(flat(ask.querySelector('.rl__ask-text'))).toContain(
      'Em “Seguir os rastros dos goblins”. Passa de 1 para 2 tentativas',
    );
    expect(ask.getAttribute('aria-labelledby')).toBe(ask.querySelector('h4')!.id);
    expect(ask.getAttribute('aria-describedby')).toBe(ask.querySelector('.rl__ask-text')!.id);
    const buttons = Array.from(ask.querySelectorAll('button'));
    expect(buttons.map((b) => words(b))).toEqual(['Voltar', 'Dar mais uma tentativa']);
    expect(buttons[0].classList.contains('mat-mdc-outlined-button')).toBe(true);
    expect(buttons[1].classList.contains('mat-mdc-unelevated-button')).toBe(true);
    expect(document.activeElement).toBe(buttons[0]);
    // The card's own button is gone while the question is open, and the host is the warm box.
    expect(opener(el)).toBeNull();
    expect((fixture.nativeElement as HTMLElement).classList.contains('rl--asking')).toBe(true);
    expect(fixture.nativeElement.querySelectorAll('.mat-mdc-unelevated-button')).toHaveLength(1);
  });

  it('"Voltar" closes the question without calling the server and hands focus back to the action', async () => {
    const { api, el, settle } = await setup();
    opener(el)!.click();
    await settle();
    button(el, 'Voltar').click();
    await settle();
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
    expect(api.calls).toEqual([]);
    expect(document.activeElement).toBe(opener(el));
  });

  it('gives the attempt once, with one key, and says it in a status line with the time and the count', async () => {
    const { api, state, el, settle } = await setup();
    opener(el)!.click();
    await settle();
    button(el, 'Dar mais uma tentativa').click();
    button(el, 'Dar mais uma tentativa')?.click();
    await settle();
    expect(api.calls).toHaveLength(1);
    expect(api.calls[0]).toMatch(/^grant a2 toren [0-9a-f-]{36}$/);
    expect(state.scene()).toBe(api.granted);
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
    const status = el.querySelector('[role="status"]')!;
    expect(flat(status)).toMatch(
      /Mais uma tentativa dada a Toren às \d\d:\d\d\. Agora são 2 tentativas em “Seguir os rastros dos goblins”; 1 já usada\./,
    );
    expect(document.activeElement).toBe(status);
  });

  it('says why when the server refuses, and keeps the question open to try again', async () => {
    const { api, el, settle } = await setup();
    api.failWith = new ConnectError('gone', Code.NotFound);
    opener(el)!.click();
    await settle();
    button(el, 'Dar mais uma tentativa').click();
    await settle();
    expect(el.querySelector('[role="alert"]')?.textContent).toContain('Essa cena não existe mais');
    expect(el.querySelector('[role="alertdialog"]')).not.toBeNull();
  });

  it('clears the status line once the player rolls again (the count it spoke of changed)', async () => {
    const { fixture, state, el, api, settle } = await setup();
    opener(el)!.click();
    await settle();
    button(el, 'Dar mais uma tentativa').click();
    await settle();
    expect(el.querySelector('[role="status"]')).not.toBeNull();
    // Still there while nothing else happens (the master keeps the confirmation).
    state.apply(api.granted!);
    fixture.detectChanges();
    expect(el.querySelector('[role="status"]')).not.toBeNull();
    // The player rolled again: the scene now has a second roll of Toren at the action.
    const again = sceneRoll('r9', 'a2', 'Toren', 12, {
      passed: false,
      attemptsLeft: 0,
      rolledAt: new Date(2026, 9, 3, 21, 30),
    });
    const scene = masterScene([again, failed]);
    fixture.componentRef.setInput('scene', scene);
    fixture.detectChanges();
    expect(el.querySelector('[role="status"]')).toBeNull();
  });
});
