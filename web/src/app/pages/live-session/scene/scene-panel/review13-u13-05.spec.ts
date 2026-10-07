// Finding U13-05 (review/unit-13-web-live-rest.md): ScenePanel.open() does not await map.refresh(), so the picker gets the pre-refresh points.
import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';

import { SceneActionSchema } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import { MapState } from '../../../../core/maps/map-state';
import { mapMessage, mapPoint, mapResponse } from '../../../../core/maps/maps-testing';
import { SceneClient } from '../../../../core/play/scene-client';
import { SceneState } from '../../../../core/play/scene-state';
import { FakeSceneClient } from '../../../../core/play/scene-testing';
import { ScenePanel } from './scene-panel';

const actions = (n: number) =>
  Array.from({ length: n }, (_, i) =>
    create(SceneActionSchema, { id: `a${i}`, key: 'skill:arcana', checkName: 'Arcanismo' }),
  );

describe('Review13 U13-05: scene picker shows the pre-refresh points', () => {
  async function setup() {
    const api = new FakeSceneClient();
    const state = new SceneState(
      () => api.get(),
      () => true,
    );
    let resolveSecond!: () => void;
    let calls = 0;
    const mapState = new MapState(() => {
      calls++;
      if (calls === 1) {
        return Promise.resolve(
          mapResponse(mapMessage('m1', 'Estrada'), [
            mapPoint('p1', 'Carroça', { sceneActions: actions(1) }),
            mapPoint('p2', 'Apagada', { sceneActions: actions(1) }),
          ]),
        );
      }
      return new Promise((resolve) => {
        resolveSecond = () =>
          resolve(
            mapResponse(mapMessage('m1', 'Estrada'), [
              mapPoint('p1', 'Carroça', { sceneActions: actions(4) }),
            ]),
          );
      });
    });
    await mapState.open('m1');
    TestBed.configureTestingModule({ providers: [{ provide: SceneClient, useValue: api }] });
    const fixture = TestBed.createComponent(ScenePanel);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('state', state);
    fixture.componentRef.setInput('mapState', mapState);
    fixture.detectChanges();
    const settle = async () => {
      for (let i = 0; i < 4; i++) {
        await fixture.whenStable();
        fixture.detectChanges();
      }
    };
    const rows = () =>
      Array.from(document.querySelectorAll('.pk__row')).map((r) =>
        (r.textContent ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim(),
      );
    return { fixture, mapState, settle, rows, finish: () => resolveSecond() };
  }

  it('control: the picker lists the points the page had', async () => {
    const { fixture, settle, rows } = await setup();
    (fixture.nativeElement as HTMLElement).querySelector('button')!.click();
    await settle();
    expect(rows()).toHaveLength(2);
    expect(rows()[0]).toContain('1 ação');
  });

  it('shows the refreshed points once the refresh answers', async () => {
    const { fixture, mapState, settle, rows, finish } = await setup();
    (fixture.nativeElement as HTMLElement).querySelector('button')!.click();
    await settle();
    finish();
    await settle();
    expect(mapState.points()).toHaveLength(1); // the refresh did land
    expect(rows()).toHaveLength(1);
    expect(rows()[0]).toContain('4 ações');
  });
});
