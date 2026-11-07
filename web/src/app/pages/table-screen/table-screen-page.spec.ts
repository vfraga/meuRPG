import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router, convertToParamMap } from '@angular/router';

import { TableScreenKind } from '../../../gen/meurpg/play/v1/table_screen_pb';
import { AuthService } from '../../core/auth/auth.service';
import { TableScreenClient } from '../../core/play/table-screen-client';
import type { TvPhase } from '../../core/play/table-screen-state';
import {
  FakeClock,
  FakeWatchCall,
  noOpenSession,
  screenResponse,
} from '../../core/play/table-screen-testing';
import { TV_CLOCK, TableScreenWatcher } from '../../core/play/table-screen-watcher';
import { nextTurn } from '../../../test-settle';
import { TableScreenPage } from './table-screen-page';
import {
  IDLE,
  combatBody,
  fogMapBody,
  plainMapBody,
  readyScreen,
  rollsOf,
} from './table-screen-testing';

function fullscreenSupport(enabled: boolean): void {
  Object.defineProperty(document, 'fullscreenEnabled', { value: enabled, configurable: true });
}

describe('TableScreenPage', () => {
  const phase = signal<TvPhase>({ kind: 'loading', text: 'Carregando a mesa…' });
  const watcher = { state: { phase }, start: vi.fn(), stop: vi.fn() };
  const auth = { signIn: vi.fn() };

  function render() {
    const fixture = TestBed.createComponent(TableScreenPage);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }
  const ready = (screen = readyScreen(), late: string | null = null): TvPhase => ({
    kind: 'ready',
    screen,
    late,
  });
  const stage = (kind: TableScreenKind, body: object, rolls: object[] = []) =>
    readyScreen({ kind, body }, rolls);

  beforeEach(() => {
    fullscreenSupport(false);
    phase.set({ kind: 'loading', text: 'Carregando a mesa…' });
    watcher.start.mockClear();
    watcher.stop.mockClear();
    auth.signIn.mockClear();
    TestBed.configureTestingModule({
      providers: [
        { provide: TableScreenWatcher, useValue: watcher },
        { provide: AuthService, useValue: auth },
        { provide: Router, useValue: { url: '/campaigns/c1/table-screen' } },
        {
          provide: ActivatedRoute,
          useValue: { snapshot: { paramMap: convertToParamMap({ id: 'c1' }) } },
        },
      ],
    });
  });

  it('opens the watcher on the campaign of the route and stops it when it leaves', () => {
    const { fixture } = render();
    expect(watcher.start).toHaveBeenCalledWith('c1');
    fixture.destroy();
    expect(watcher.stop).toHaveBeenCalled();
  });

  describe('phases', () => {
    it('loading: says it is loading', () => {
      const { el } = render();
      expect(el.querySelector('[role="status"] h1')?.textContent).toBe('Carregando a mesa…');
    });

    it('error: the title and the retry detail', () => {
      phase.set({
        kind: 'error',
        title: 'Não deu para abrir a Tela da mesa.',
        detail: 'Tentando de novo em 10 s.',
      });
      const { el } = render();
      expect(el.querySelector('h1')?.textContent).toBe('Não deu para abrir a Tela da mesa.');
      expect(el.textContent).toContain('Tentando de novo em 10 s.');
    });

    it('lost connection: the header with "Sem conexão" and the card, one page heading', () => {
      phase.set({
        kind: 'stale',
        title: 'Sem conexão desde 21:14',
        detail: 'Esta tela escondeu o que mostrava.',
        campaignName: 'Mirathel',
        sessionLabel: 'Sessão 3',
      });
      const { el } = render();
      expect(el.querySelectorAll('h1')).toHaveLength(1);
      expect(el.querySelector('h1')?.textContent).toBe('Sessão 3');
      expect(el.querySelector('.tv-head__kicker')?.textContent).toBe('Mirathel');
      expect(el.querySelector('h2')?.textContent).toBe('Sem conexão desde 21:14');
      expect(el.querySelector('[data-testid="tv-link-lost"]')).not.toBeNull();
      expect(el.querySelector('app-tv-idle')).toBeNull();
    });

    it('ended: the farewell and the hint', () => {
      phase.set({
        kind: 'ended',
        title: 'A sessão 3 terminou às 23:40.',
        farewell: 'Até a próxima sessão de Mirathel.',
        hint: 'O mestre pode fechar esta aba.',
        campaignName: 'Mirathel',
        sessionLabel: 'Sessão 3',
        endedAt: '23:40',
      });
      const { el } = render();
      expect(el.querySelector('h1')?.textContent).toBe('Sessão 3');
      expect(el.querySelector('.tv-head__kicker')?.textContent).toBe('Mirathel');
      expect(el.querySelector('[data-testid="tv-link-ended"]')?.textContent).toContain(
        'Encerrada às 23:40',
      );
      expect(el.querySelector('h2')?.textContent).toBe('A sessão 3 terminou às 23:40.');
      expect(el.textContent).toContain('Até a próxima sessão de Mirathel.');
      expect(el.textContent).toContain('O mestre pode fechar esta aba.');
    });

    it('ended before any read: no names, "Tela da mesa" as the title', () => {
      phase.set({
        kind: 'ended',
        title: 'Não há sessão aberta.',
        farewell: '',
        hint: 'O mestre pode fechar esta aba.',
        campaignName: '',
        sessionLabel: '',
        endedAt: '',
      });
      const { el } = render();
      expect(el.querySelector('h1')?.textContent).toBe('Tela da mesa');
      expect(el.querySelector('.tv-head__kicker')).toBeNull();
    });

    it('refused: the card "Só o mestre abre a Tela da mesa", with no table behind it', () => {
      phase.set({
        kind: 'no-access',
        title: 'Só o mestre abre a Tela da mesa.',
        detail: 'Ela é a tela da sala.',
      });
      const { el } = render();
      expect(el.querySelector('h1')?.textContent).toBe('Só o mestre abre a Tela da mesa.');
      expect(el.querySelector('app-tv-header')).toBeNull();
      expect(el.querySelector('[data-testid="tv-link-lost"]')).toBeNull();
    });

    it('signed out: the card, and the viewer goes to sign in and back to this page, once', () => {
      phase.set({
        kind: 'signed-out',
        title: 'Entre para abrir a Tela da mesa.',
        detail: 'Sua entrada expirou.',
      });
      const { fixture, el } = render();
      expect(el.querySelector('h1')?.textContent).toBe('Entre para abrir a Tela da mesa.');
      expect(auth.signIn).toHaveBeenCalledTimes(1);
      expect(auth.signIn).toHaveBeenCalledWith('/campaigns/c1/table-screen');
      fixture.detectChanges();
      expect(auth.signIn).toHaveBeenCalledTimes(1);
    });

    it('does not send anyone to sign in while the screen is fine', () => {
      phase.set(ready());
      render();
      expect(auth.signIn).not.toHaveBeenCalled();
    });
  });

  describe('stages', () => {
    it('idle: the session as the title, the waiting card and who is at the table', () => {
      phase.set(ready());
      const { el } = render();
      expect(el.querySelector('h1')?.textContent).toBe('Sessão 3');
      expect(el.querySelector('app-tv-idle h2')?.textContent).toBe('Nada à mostra ainda');
      expect(el.textContent).toContain('Tor');
      expect(el.querySelector('app-live-pill')).not.toBeNull();
    });

    it('map: the fog tiles with as=tv, the free rolls beside them', () => {
      phase.set(ready(stage(TableScreenKind.MAP, { case: 'map', value: fogMapBody() }, rollsOf)));
      const { el } = render();
      expect(el.querySelector('h1')?.textContent).toBe('Cripta do Corvo');
      expect(el.querySelector('.tv-head__kicker')?.textContent).toBe('Mapa · Mirathel · Sessão 3');
      expect(el.querySelector('img[src$="as=tv"]')).not.toBeNull();
      expect(el.querySelector('app-tv-rolls')?.textContent).toContain('Brisa rolou');
    });

    it('map without fog: the picture', () => {
      phase.set(ready(stage(TableScreenKind.MAP, { case: 'map', value: plainMapBody() })));
      const { el } = render();
      expect(el.querySelector('app-map-view')).not.toBeNull();
      expect(el.querySelector('app-fog-map')).toBeNull();
    });

    it('map hidden by the fog: the card "O mapa fica na tela de cada um"', () => {
      phase.set(ready(stage(TableScreenKind.MAP, { case: 'map', value: { hiddenByFog: true } })));
      const { el } = render();
      expect(el.querySelector('app-tv-fog-card h2')?.textContent).toBe(
        'O mapa fica na tela de cada um.',
      );
      expect(el.querySelector('img')).toBeNull();
    });

    it('image: the picture with the name as its alternative text', () => {
      phase.set(
        ready(
          stage(TableScreenKind.IMAGE, {
            case: 'image',
            value: { name: 'A carta de Bartolo', url: '/images/i1', width: 800, height: 600 },
          }),
        ),
      );
      const { el } = render();
      expect(el.querySelector('h1')?.textContent).toBe('A carta de Bartolo');
      expect(el.querySelector('img')?.getAttribute('alt')).toBe('A carta de Bartolo');
    });

    it('scene: the name, the description and who speaks', () => {
      phase.set(
        ready(
          stage(TableScreenKind.SCENE, {
            case: 'scene',
            value: {
              name: 'A taverna do Javali Cego',
              description: 'O salão está cheio.',
              stage: [{ name: 'Bartolo', speaking: true }],
            },
          }),
        ),
      );
      const { el } = render();
      expect(el.querySelector('h1')?.textContent).toBe('A taverna do Javali Cego');
      expect(el.textContent).toContain('O salão está cheio.');
      expect(el.textContent).toContain('Fala agora');
    });

    it('combat with a map: the map, the order and the log', () => {
      phase.set(ready(stage(TableScreenKind.COMBAT, combatBody(fogMapBody()), rollsOf)));
      const { el } = render();
      expect(el.querySelector('h1')?.textContent).toBe('Combate · Cripta do Corvo');
      expect(el.querySelector('app-tv-combat app-fog-map')).not.toBeNull();
      expect(el.querySelector('app-tv-order')?.textContent).toContain('Tor');
      expect(el.querySelectorAll('.tv-combat__line')).toHaveLength(2);
    });

    it('combat without a map: no map card, the whole log', () => {
      phase.set(ready(stage(TableScreenKind.COMBAT, combatBody())));
      const { el } = render();
      expect(el.querySelector('h1')?.textContent).toBe('Combate sem mapa');
      expect(el.querySelector('app-tv-map')).toBeNull();
      expect(el.querySelectorAll('.tv-combat__line')).toHaveLength(2);
    });

    it('combat with the fog hiding the map: the card beside the order', () => {
      phase.set(ready(stage(TableScreenKind.COMBAT, combatBody({ hiddenByFog: true }))));
      const { el } = render();
      expect(el.querySelector('app-tv-combat app-tv-fog-card')).not.toBeNull();
      expect(el.querySelector('app-tv-order')).not.toBeNull();
    });
  });

  describe('the connection', () => {
    it('late: "Reconectando…" and the warning, with the stage still drawn', () => {
      phase.set(
        ready(
          readyScreen(),
          'Esta tela pode estar atrasada desde 21:14. Ela continua assim por até 1 minuto.',
        ),
      );
      const { el } = render();
      expect(el.querySelector('[data-testid="tv-link-late"]')?.textContent).toContain(
        'Reconectando…',
      );
      const notice = el.querySelector('[data-testid="tv-late"]')!;
      expect(notice.textContent).toContain('pode estar atrasada desde 21:14');
      expect(notice.closest('.mr-live')?.getAttribute('role')).toBe('status');
      expect(el.querySelector('app-tv-idle')).not.toBeNull();
    });

    it('live: no warning, but the live region stays for the next one', () => {
      phase.set(ready());
      const { fixture, el } = render();
      expect(el.querySelector('[data-testid="tv-late"]')).toBeNull();
      expect(el.querySelector('.tv__late.mr-live')).not.toBeNull();
      phase.set(ready(readyScreen(), 'Esta tela pode estar atrasada.'));
      fixture.detectChanges();
      expect(el.querySelector('[data-testid="tv-late"]')).not.toBeNull();
    });
  });

  describe('the notice', () => {
    it('says why the TV is not showing what the master asked for, as a line under the header', () => {
      phase.set(
        ready({ ...readyScreen(), notice: 'O combate acabou. A TV foi para a tela de espera.' }),
      );
      const { el } = render();
      expect(el.querySelector('[data-testid="tv-notice"]')?.textContent).toBe(
        'O combate acabou. A TV foi para a tela de espera.',
      );
    });

    it('draws no line without one', () => {
      phase.set(ready());
      expect(render().el.querySelector('[data-testid="tv-notice"]')).toBeNull();
    });
  });

  describe('accessibility', () => {
    it('announces the stage in a polite live region, and again when it changes', () => {
      phase.set(ready());
      const { fixture, el } = render();
      const live = el.querySelector('[data-testid="tv-announce"]')!;
      expect(live.getAttribute('role')).toBe('status');
      expect(live.textContent).toBe('Na tela: Sessão 3');
      phase.set(ready(stage(TableScreenKind.COMBAT, combatBody())));
      fixture.detectChanges();
      expect(live.textContent).toBe('Na tela: Combate sem mapa');
    });

    it('has one h1 on every screen of a ready table', () => {
      const bodies: [TableScreenKind, object][] = [
        [TableScreenKind.MAP, { case: 'map', value: plainMapBody() }],
        [TableScreenKind.IMAGE, { case: 'image', value: { name: 'x', url: '/i' } }],
        [TableScreenKind.SCENE, { case: 'scene', value: { name: 'x' } }],
        [TableScreenKind.COMBAT, combatBody()],
      ];
      for (const [kind, body] of [
        [TableScreenKind.IDLE, IDLE.body] as [TableScreenKind, object],
        ...bodies,
      ]) {
        phase.set(ready(stage(kind, body)));
        const { fixture, el } = render();
        expect(el.querySelectorAll('h1')).toHaveLength(1);
        fixture.destroy();
      }
    });
  });

  describe('full screen', () => {
    it('hints at it where the browser can, and goes full screen on the click', () => {
      fullscreenSupport(true);
      const request = vi.fn(() => Promise.resolve());
      phase.set(ready());
      const { fixture, el } = render();
      (fixture.nativeElement as HTMLElement).requestFullscreen = request;
      const hint = el.querySelector<HTMLButtonElement>('app-tv-fullscreen-hint button')!;
      expect(hint.textContent).toContain('Pressione F11 ou clique na tela');
      hint.click();
      fixture.detectChanges();
      expect(request).toHaveBeenCalledTimes(1);
      expect(el.querySelector('app-tv-fullscreen-hint')).toBeNull();
    });

    it('puts the hint under the header, never above the title', () => {
      fullscreenSupport(true);
      phase.set(ready());
      const { el } = render();
      const head = el.querySelector('app-tv-header')!;
      const hint = el.querySelector('app-tv-fullscreen-hint')!;
      expect(head.compareDocumentPosition(hint) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    });

    it('says nothing where the browser has no full screen', () => {
      phase.set(ready());
      expect(render().el.querySelector('app-tv-fullscreen-hint')).toBeNull();
    });
  });
});

describe('TableScreenPage with the real watcher', () => {
  it('reads only TableScreenService: one stream and one read, and the TV’s own screen', async () => {
    vi.useFakeTimers();
    try {
      const calls: FakeWatchCall[] = [];
      const get = vi.fn((_id: string) => Promise.resolve(screenResponse()));
      TestBed.configureTestingModule({
        providers: [
          TableScreenWatcher,
          { provide: TV_CLOCK, useValue: new FakeClock(new Date(2026, 0, 5, 21, 10)) },
          {
            provide: TableScreenClient,
            useValue: {
              get,
              watch: (_: string, signal: AbortSignal) => {
                const call = new FakeWatchCall(signal);
                calls.push(call);
                return call.events();
              },
            },
          },
          { provide: AuthService, useValue: { signIn: vi.fn() } },
          { provide: Router, useValue: { url: '/campaigns/c1/table-screen' } },
          {
            provide: ActivatedRoute,
            useValue: { snapshot: { paramMap: convertToParamMap({ id: 'c1' }) } },
          },
        ],
      });
      const fixture = TestBed.createComponent(TableScreenPage);
      fixture.detectChanges();
      expect(fixture.nativeElement.textContent).toContain('Carregando a mesa…');
      calls[0].push('ready');
      await nextTurn();
      await nextTurn();
      TestBed.tick();
      fixture.detectChanges();
      expect(get).toHaveBeenCalledTimes(1);
      expect(get).toHaveBeenCalledWith('c1');
      expect(fixture.nativeElement.querySelector('h1')?.textContent).toBe('Sessão 3');
      fixture.destroy();
      expect(calls[0].aborted).toBe(true);
    } finally {
      vi.useRealTimers();
    }
  });

  it('shows the refusal when the server says there is no open session', async () => {
    vi.useFakeTimers();
    try {
      const calls: FakeWatchCall[] = [];
      TestBed.configureTestingModule({
        providers: [
          TableScreenWatcher,
          { provide: TV_CLOCK, useValue: new FakeClock(new Date(2026, 0, 5, 21, 10)) },
          {
            provide: TableScreenClient,
            useValue: {
              get: () => Promise.reject(noOpenSession()),
              watch: (_: string, signal: AbortSignal) => {
                const call = new FakeWatchCall(signal);
                calls.push(call);
                return call.events();
              },
            },
          },
          { provide: AuthService, useValue: { signIn: vi.fn() } },
          { provide: Router, useValue: { url: '/campaigns/c1/table-screen' } },
          {
            provide: ActivatedRoute,
            useValue: { snapshot: { paramMap: convertToParamMap({ id: 'c1' }) } },
          },
        ],
      });
      const fixture = TestBed.createComponent(TableScreenPage);
      fixture.detectChanges();
      calls[0].push('ready');
      await nextTurn();
      await nextTurn();
      TestBed.tick();
      fixture.detectChanges();
      expect(fixture.nativeElement.querySelector('h2')?.textContent).toBe('Não há sessão aberta.');
      fixture.destroy();
    } finally {
      vi.useRealTimers();
    }
  });
});
