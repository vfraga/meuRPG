// Finding U13-07 (review/unit-13-web-live-rest.md): "Voltar" then asking again after a lost answer makes a new idempotency key, so the first (already applied) grant is granted twice.
import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import { SceneClient } from '../../../../core/play/scene-client';
import { SceneState } from '../../../../core/play/scene-state';
import { FakeSceneClient, masterScene, sceneRoll } from '../../../../core/play/scene-testing';
import { SceneRollLine } from './scene-roll-line';

const failed = sceneRoll('r1', 'a2', 'Toren', 7, { passed: false, attemptsLeft: 0 });

describe('Review13 U13-07: a grant sent again after Voltar keeps the key of the lost one', () => {
  async function setup() {
    const api = new FakeSceneClient();
    api.granted = masterScene([failed], [], {});
    const scene = masterScene([failed]);
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
    fixture.componentRef.setInput('roll', failed);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      for (let i = 0; i < 4; i++) {
        await fixture.whenStable();
        fixture.detectChanges();
      }
    };
    const opener = () => el.querySelector<HTMLButtonElement>('.rl__grant')!;
    const confirm = () =>
      Array.from(el.querySelectorAll('[role="alertdialog"] button')).find((b) =>
        b.textContent?.includes('Dar mais uma tentativa'),
      ) as HTMLButtonElement;
    const voltar = () =>
      Array.from(el.querySelectorAll('[role="alertdialog"] button')).find((b) =>
        b.textContent?.includes('Voltar'),
      ) as HTMLButtonElement;
    return { api, el, settle, opener, confirm, voltar };
  }
  const keyOf = (call: string) => call.split(' ').pop();

  it('control: retrying inside the same open question reuses the key', async () => {
    const { api, settle, opener, confirm } = await setup();
    opener().click();
    await settle();
    api.failWith = new ConnectError('lost', Code.Unavailable);
    confirm().click();
    await settle();
    api.failWith = null;
    confirm().click();
    await settle();
    expect(api.calls).toHaveLength(2);
    expect(keyOf(api.calls[1])).toBe(keyOf(api.calls[0]));
  });

  it('keeps the same key when the master goes back and asks again after a lost answer', async () => {
    const { api, settle, opener, confirm, voltar } = await setup();
    opener().click();
    await settle();
    api.failWith = new ConnectError('lost', Code.Unavailable);
    confirm().click();
    await settle();
    voltar().click();
    await settle();
    api.failWith = null;
    opener().click();
    await settle();
    confirm().click();
    await settle();
    expect(api.calls).toHaveLength(2);
    expect(keyOf(api.calls[1])).toBe(keyOf(api.calls[0]));
  });
});
