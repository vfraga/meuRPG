import { create } from '@bufbuild/protobuf';
import { Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter, Router } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import { Role } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  DungeonCorridorStyle,
  DungeonDoorMix,
  DungeonMask,
  DungeonOptionRefusedSchema,
} from '../../../../gen/meurpg/maps/v1/dungeons_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { DungeonsClient } from '../../../core/maps/dungeons-client';
import { FakeDungeonsClient } from '../../../core/maps/dungeons-testing';
import { DungeonNew, PREVIEW_DELAY_MS } from './dungeon-new';

@Component({ template: 'editor' })
class Stub {}

const refused = (option: string) =>
  new ConnectError('refused', Code.InvalidArgument, undefined, [
    { desc: DungeonOptionRefusedSchema, value: create(DungeonOptionRefusedSchema, { option }) },
  ]);

describe('DungeonNew ("Gerar masmorra", MR-010, E10-05 1 to 4)', () => {
  let api: FakeDungeonsClient;
  let router: Router;

  async function open(options: { role?: Role; phone?: boolean; campaignFails?: boolean } = {}) {
    api = new FakeDungeonsClient();
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: !!options.phone && query.includes('767'),
      media: query,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    }));
    TestBed.configureTestingModule({
      providers: [
        provideRouter([
          { path: 'campaigns/:id/maps/dungeon', component: DungeonNew },
          { path: 'campaigns/:id/maps/:mapId', component: Stub },
        ]),
        { provide: DungeonsClient, useValue: api },
        {
          provide: CampaignsService,
          useValue: {
            getCampaign: async () => {
              if (options.campaignFails) {
                throw new ConnectError('down', Code.Unavailable);
              }
              return {
                campaign: {
                  name: 'Mirathel',
                  myRole: options.role ?? Role.MASTER,
                  awaitingApproval: false,
                },
              };
            },
          },
        },
      ],
    });
    router = TestBed.inject(Router);
    const harness = await RouterTestingHarness.create();
    await harness.navigateByUrl('/campaigns/camp-1/maps/dungeon', DungeonNew);
    const settle = async () => {
      for (let i = 0; i < 4; i++) {
        harness.detectChanges();
        await harness.fixture.whenStable();
        await vi.advanceTimersByTimeAsync(0);
      }
      harness.detectChanges();
    };
    /** Time for the preview's pause (and its answer). */
    const debounced = async () => {
      await vi.advanceTimersByTimeAsync(PREVIEW_DELAY_MS + 10);
      await settle();
    };
    // The first preview waits on its own timer: nothing but the page's own promises run, so the loading state can be seen.
    for (let i = 0; i < 3; i++) {
      await harness.fixture.whenStable();
      harness.detectChanges();
    }
    const el = harness.routeNativeElement as HTMLElement;
    return { el, settle, debounced, harness };
  }

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  const text = (el: HTMLElement) => (el.textContent ?? '').replace(/\s+/g, ' ');
  const button = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      b.textContent?.trim().endsWith(label),
    )!;
  const radio = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLElement>('[role="radio"]')).find((b) =>
      b.textContent?.trim().endsWith(label),
    )!;
  const field = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll('mat-form-field'))
      .find((f) => f.querySelector('mat-label')?.textContent?.trim() === label)!
      .querySelector('input')!;
  function type(input: HTMLInputElement, value: string): void {
    input.value = value;
    input.dispatchEvent(new Event('input'));
  }

  describe('the page for the master', () => {
    it('asks the server for a preview of the default dungeon (no seed) and draws what it gets', async () => {
      const { el, debounced } = await open();
      expect(text(el)).toContain('Mirathel · O app desenha salas');
      expect(text(el)).toContain('Carregando a prévia...');
      await debounced();
      expect(api.previewRequests).toHaveLength(1);
      expect(api.previewRequests[0]!.seed).toBeUndefined();
      expect(api.previewRequests[0]!.options).toMatchObject({
        width: 31,
        height: 21,
        mask: DungeonMask.NONE,
        roomSideMin: 3,
        roomSideMax: 9,
        corridorStyle: DungeonCorridorStyle.MEANDERING,
        doorMix: DungeonDoorMix.TYPICAL,
        deadendRemoval: 60,
        stairs: 2,
      });
      expect(el.querySelector('app-dungeon-preview [role="img"]')).toBeTruthy();
      // The seed the server drew fills the field, and "Criar o mapa" is on.
      expect(field(el, 'Semente').value).toBe('48213');
      expect(button(el, 'Criar o mapa').getAttribute('aria-disabled')).not.toBe('true');
    });

    it('says under the preview that the map is born hidden, with the fog on and the base light "Clara"', async () => {
      const { el, debounced } = await open();
      await debounced();
      expect(text(el)).toContain(
        'O mapa nasce escondido dos jogadores, com a névoa ligada e a luz de base “Clara” (você muda no mapa).',
      );
    });

    it('draws the counts and the legend of what the preview shows, with the passages apart', async () => {
      const { el, debounced } = await open();
      await debounced();
      const words = text(el.querySelector('app-dungeon-preview')!);
      expect(words).toContain('2 salas');
      expect(words).toContain('4 portas e 1 passagem');
      expect(words).toContain('2 escadas');
      expect(words).toContain('11 × 9 quadrados');
      for (const entry of [
        'Parede',
        'Porta fechada',
        'Porta trancada (só você vê)',
        'Grade',
        'Porta secreta (só você vê)',
        'Escada para cima',
        'Escada para baixo',
      ]) {
        expect(words).toContain(entry);
      }
      // A passage is floor: no legend entry.
      expect(words).not.toContain('Passagem');
    });

    it('asks again a moment after the last change, and only once for a burst of changes', async () => {
      const { el, debounced, settle } = await open();
      await debounced();
      radio(el, 'Média (51)').click();
      await settle();
      radio(el, 'Anel').click();
      await settle();
      expect(api.previewRequests).toHaveLength(1);
      await debounced();
      expect(api.previewRequests).toHaveLength(2);
      expect(api.previewRequests[1]!.options).toMatchObject({
        width: 51,
        height: 35,
        mask: DungeonMask.DONUT,
      });
      // The seed on screen goes with the request: the same seed, the new options.
      expect(api.previewRequests[1]!.seed).toBe(48213n);
    });

    it('waits for a request that is on its way, then asks once more with the latest options (one preview at a time)', async () => {
      const { el, debounced, settle } = await open();
      await debounced();
      let release!: () => void;
      api.gate = new Promise<void>((r) => (release = r));
      radio(el, 'Média (51)').click();
      await debounced();
      expect(api.previewRequests).toHaveLength(2);
      radio(el, 'Grande (81)').click();
      await debounced();
      // The second request is still out: the third waits for it.
      expect(api.previewRequests).toHaveLength(2);
      api.gate = null;
      release();
      await settle();
      await vi.advanceTimersByTimeAsync(0);
      await settle();
      expect(api.previewRequests).toHaveLength(3);
      expect(api.previewRequests[2]!.options).toMatchObject({ width: 81 });
    });

    it('drops an answer made for older options: the button stays off and the seed the master is typing is kept', async () => {
      const { el, debounced, settle } = await open();
      await debounced();
      // A request is held on a gate; the master changes an option, then types a seed, before the pause is over.
      let release!: () => void;
      api.gate = new Promise<void>((r) => (release = r));
      radio(el, 'Média (51)').click();
      await debounced();
      expect(api.previewRequests).toHaveLength(2);
      radio(el, 'Grande (81)').click();
      await settle();
      type(field(el, 'Semente'), '777');
      await settle();
      api.gate = null;
      // The held answer (for 51) comes back before the debounce of the last change is over.
      release();
      await settle();
      expect(el.querySelector('.pv__draw--stale')).toBeTruthy();
      expect(button(el, 'Criar o mapa').getAttribute('aria-disabled')).toBe('true');
      expect(field(el, 'Semente').value).toBe('777');
      // The pause ends: one more request, with the last options and the typed seed, and then the button is on.
      await debounced();
      expect(api.previewRequests.at(-1)!.options).toMatchObject({ width: 81 });
      expect(api.previewRequests.at(-1)!.seed).toBe(777n);
      expect(button(el, 'Criar o mapa').getAttribute('aria-disabled')).not.toBe('true');
      expect(field(el, 'Semente').value).toBe('777');
    });

    it('"Outra semente" asks at once with no seed, and the new seed fills the field', async () => {
      const { el, settle } = await open();
      await vi.advanceTimersByTimeAsync(PREVIEW_DELAY_MS + 10);
      await settle();
      api.drawSeed = 777n;
      button(el, 'Outra semente').click();
      await vi.advanceTimersByTimeAsync(5);
      await settle();
      expect(api.previewRequests.at(-1)!.seed).toBeUndefined();
      expect(field(el, 'Semente').value).toBe('777');
    });

    it('sends the seed the master types', async () => {
      const { el, debounced } = await open();
      await debounced();
      type(field(el, 'Semente'), '12345');
      await debounced();
      expect(api.previewRequests.at(-1)!.seed).toBe(12345n);
    });

    it('says a seed that is not a number, in place, and does not ask', async () => {
      const { el, debounced } = await open();
      await debounced();
      const before = api.previewRequests.length;
      type(field(el, 'Semente'), '12a');
      await debounced();
      expect(text(el)).toContain('A semente é um número inteiro, só com dígitos.');
      expect(field(el, 'Semente').getAttribute('aria-invalid')).toBe('true');
      expect(api.previewRequests).toHaveLength(before);
      expect(button(el, 'Criar o mapa').getAttribute('aria-disabled')).toBe('true');
    });
  });

  describe('a refused option (E10-05 3)', () => {
    it('says a size out of range in place, before it asks, and turns "Criar o mapa" off with the reason', async () => {
      const { el, debounced, settle } = await open();
      await debounced();
      const asked = api.previewRequests.length;
      radio(el, 'Outro').click();
      await settle();
      type(field(el, 'Quadrados no lado maior'), '130');
      await debounced();
      expect(text(el)).toContain('O tamanho vai de 21 a 121 quadrados.');
      expect(field(el, 'Quadrados no lado maior').getAttribute('aria-invalid')).toBe('true');
      expect(text(el)).toContain('Corrija o tamanho para ver a masmorra.');
      expect(text(el)).toContain('Corrija o tamanho para criar o mapa.');
      expect(el.querySelector('app-dungeon-preview')).toBeNull();
      expect(api.previewRequests).toHaveLength(asked);
      expect(button(el, 'Criar o mapa').getAttribute('aria-disabled')).toBe('true');
      // A good size brings the preview back.
      type(field(el, 'Quadrados no lado maior'), '45');
      await debounced();
      expect(api.previewRequests.at(-1)!.options).toMatchObject({ width: 45, height: 31 });
      expect(el.querySelector('app-dungeon-preview')).toBeTruthy();
    });

    it("puts the server's refusal on the field it names", async () => {
      const { el, debounced, settle } = await open();
      await debounced();
      api.failWith.set('preview', refused('room_side_max'));
      radio(el, 'Sinuosos').click();
      await settle();
      radio(el, 'Retos').click();
      await debounced();
      expect(text(el)).toContain(
        'O maior lado das salas vai até 31 quadrados e não pode ser menor que o menor.',
      );
      expect(field(el, 'Maior lado').getAttribute('aria-invalid')).toBe('true');
      expect(text(el)).toContain('Corrija as opções marcadas para ver a masmorra.');
      // Changing an option clears the server's reason: the page asks again.
      api.failWith.delete('preview');
      radio(el, 'Labirinto').click();
      await debounced();
      expect(field(el, 'Maior lado').getAttribute('aria-invalid')).not.toBe('true');
      expect(el.querySelector('app-dungeon-preview')).toBeTruthy();
    });

    it('says a refusal that names no option (no room fits) over the empty preview', async () => {
      const { el, debounced } = await open();
      api.failWith.set('preview', refused(''));
      await debounced();
      radio(el, 'Anel').click();
      await debounced();
      expect(text(el)).toContain('não cabe nenhuma sala');
    });
  });

  describe('the preview loading and failing (E10-05 4)', () => {
    it('shows "Carregando a prévia..." as a status and explains why "Criar o mapa" is off', async () => {
      const { el } = await open();
      expect(el.querySelector('[role="status"]')?.textContent).toContain('Carregando a prévia...');
      expect(text(el)).toContain('Espere a prévia para criar o mapa.');
      expect(button(el, 'Criar o mapa').getAttribute('aria-disabled')).toBe('true');
    });

    it('fails with "Tentar de novo", keeps the options, and asks again with the same ones', async () => {
      const { el, debounced, settle } = await open();
      api.failWith.set('preview', new ConnectError('down', Code.Unavailable));
      await debounced();
      expect(el.querySelector('[role="alert"]')?.textContent).toContain(
        'Não deu para gerar a prévia.',
      );
      expect(text(el)).toContain('As opções continuam as mesmas.');
      expect(text(el)).toContain('Sem a prévia não dá para criar o mapa.');
      api.failWith.delete('preview');
      button(el, 'Tentar de novo').click();
      await vi.advanceTimersByTimeAsync(5);
      await settle();
      expect(el.querySelector('app-dungeon-preview')).toBeTruthy();
      expect(api.previewRequests.at(-1)!.options).toMatchObject({ width: 31 });
    });

    it('says a preview that took too long', async () => {
      const { el, debounced } = await open();
      api.failWith.set('preview', new ConnectError('slow', Code.DeadlineExceeded));
      await debounced();
      expect(text(el)).toContain('O gerador demorou demais com essas opções.');
    });

    it('asks again when the server says a preview of the campaign is already running', async () => {
      const { debounced, settle } = await open();
      api.failWith.set('preview', new ConnectError('busy', Code.ResourceExhausted));
      await debounced();
      const before = api.previewRequests.length;
      api.failWith.delete('preview');
      await vi.advanceTimersByTimeAsync(450);
      await settle();
      expect(api.previewRequests.length).toBeGreaterThan(before);
    });
  });

  describe('"Criar o mapa" (E10-05 2)', () => {
    it('sends the name, the options and the previewed seed, then opens the new map', async () => {
      const { el, debounced, settle } = await open();
      await debounced();
      const navigate = vi.spyOn(router, 'navigate');
      expect(field(el, 'Nome do mapa').value).toBe('Masmorra de Mirathel');
      type(field(el, 'Nome do mapa'), '  A cripta  ');
      button(el, 'Criar o mapa').click();
      await settle();
      expect(api.calls).toContain('create A cripta 48213');
      expect(api.createdOptions).toMatchObject({ width: 31, height: 21, stairs: 2 });
      expect(navigate).toHaveBeenCalledWith(['/campaigns', 'camp-1', 'maps', 'dungeon-1']);
    });

    it('shows one status line after a moment (no scripted steps), the bar and "Parar de esperar", with the options locked', async () => {
      const { el, debounced, settle } = await open();
      await debounced();
      let release!: () => void;
      api.gate = new Promise<void>((r) => (release = r));
      button(el, 'Criar o mapa').click();
      await settle();
      // The region is in the page from the start and empty: a quick creation does not flash a line.
      expect(el.querySelector('.work__status')?.textContent).toBe('');
      await vi.advanceTimersByTimeAsync(450);
      await settle();
      const words = text(el);
      expect(el.querySelector('.work__status')?.textContent).toBe(
        'Criando o mapa. Costuma levar poucos segundos.',
      );
      expect(el.querySelector('.work__status')?.getAttribute('role')).toBe('status');
      expect(el.querySelector('[aria-current]')).toBeNull();
      expect(words).not.toContain('Desenhar a imagem');
      expect(words).toContain('Pequena (31)');
      expect(words).toContain('Semente 48213');
      expect(words).toContain('As opções ficam travadas enquanto o mapa é criado.');
      expect(words).toContain('Se você sair desta tela, ele continua sendo criado');
      expect(el.querySelector('mat-progress-bar')?.getAttribute('aria-label')).toBe(
        'Criando o mapa',
      );
      expect(
        button(el, 'Criando o mapa...').disabled ||
          button(el, 'Criando o mapa...').getAttribute('aria-disabled') === 'true',
      ).toBe(true);
      expect(el.querySelector('app-dungeon-options')).toBeNull();
      release();
      await settle();
    });

    it('"Parar de esperar" stops waiting and says the server finishes the map anyway', async () => {
      const { el, debounced, settle } = await open();
      await debounced();
      let release!: () => void;
      api.gate = new Promise<void>((r) => (release = r));
      button(el, 'Criar o mapa').click();
      await settle();
      button(el, 'Parar de esperar').click();
      await settle();
      expect(text(el)).toContain('Você parou de esperar.');
      expect(text(el)).toContain('O servidor termina o mapa mesmo assim');
      // The options are back, with the same seed.
      expect(field(el, 'Semente').value).toBe('48213');
      release();
      await settle();
      expect(text(el)).not.toContain('Criando o mapa. Costuma');
    });

    it('says a failed creation above the buttons and unlocks the options with the same seed', async () => {
      const { el, debounced, settle } = await open();
      await debounced();
      api.failWith.set('create', new ConnectError('limit', Code.ResourceExhausted));
      button(el, 'Criar o mapa').click();
      await settle();
      expect(el.querySelector('[role="alert"]')?.textContent).toContain('Espere um pouco');
      expect(el.querySelector('app-dungeon-options')).toBeTruthy();
      expect(field(el, 'Semente').value).toBe('48213');
      // Trying again goes to the same seed.
      api.failWith.delete('create');
      button(el, 'Criar o mapa').click();
      await settle();
      expect(api.calls.filter((c) => c.startsWith('create '))).toHaveLength(2);
    });

    it('puts a refused option from the server on its field', async () => {
      const { el, debounced, settle } = await open();
      await debounced();
      api.failWith.set('create', refused('deadend_removal'));
      button(el, 'Criar o mapa').click();
      await settle();
      expect(text(el)).toContain('Os becos vão de 0 a 100 %.');
    });

    it('leaving the page while it creates never takes the master to the new map', async () => {
      const { el, debounced, settle, harness } = await open();
      await debounced();
      let release!: () => void;
      api.gate = new Promise<void>((r) => (release = r));
      const navigate = vi.spyOn(router, 'navigate');
      button(el, 'Criar o mapa').click();
      await settle();
      harness.fixture.destroy();
      release();
      await vi.advanceTimersByTimeAsync(10);
      expect(navigate).not.toHaveBeenCalledWith(['/campaigns', 'camp-1', 'maps', 'dungeon-1']);
    });

    it('sends the options and the seed of the preview on screen, not what the form says now', async () => {
      const { el, debounced, settle } = await open();
      await debounced();
      // The master changes an option; its preview has not come back yet.
      radio(el, 'Grande (81)').click();
      await settle();
      // "Criar o mapa" is off until the new preview is on screen.
      expect(button(el, 'Criar o mapa').getAttribute('aria-disabled')).toBe('true');
      await debounced();
      button(el, 'Criar o mapa').click();
      await settle();
      expect(api.createdOptions).toMatchObject({ width: 81, height: 55 });
    });

    it('asks for a name, in place, and sends nothing', async () => {
      const { el, debounced, settle } = await open();
      await debounced();
      type(field(el, 'Nome do mapa'), '   ');
      button(el, 'Criar o mapa').click();
      await settle();
      expect(el.querySelector('mat-error')?.textContent).toContain('Dê um nome ao mapa.');
      expect(api.calls.some((c) => c.startsWith('create'))).toBe(false);
    });
  });

  describe('who gets the page', () => {
    it('tells a player (or a non-member) that only the master generates dungeons, and asks nothing', async () => {
      const { el, debounced } = await open({ role: Role.PLAYER });
      await debounced();
      expect(text(el)).toContain('Só o mestre gera masmorras.');
      expect(api.calls).toEqual([]);
    });

    it('says the server did not answer, with a way to try again', async () => {
      const { el } = await open({ campaignFails: true });
      expect(text(el)).toContain('O servidor não respondeu.');
      expect(button(el, 'tente de novo')).toBeTruthy();
    });

    it('shows the shapes with "Em L" and the seed field takes up to 20 digits', async () => {
      const { el, debounced } = await open();
      await debounced();
      expect(radio(el, 'Em L')).toBeTruthy();
      expect(field(el, 'Semente').getAttribute('maxlength')).toBe('20');
    });

    it('on a phone only says that generating a dungeon is done on a computer', async () => {
      const { el, debounced } = await open({ phone: true });
      await debounced();
      expect(text(el)).toContain('Gerar masmorra é no notebook.');
      expect(el.querySelector('app-dungeon-options')).toBeNull();
      expect(api.previewRequests).toEqual([]);
    });
  });
});

describe('DungeonNew: a resposta atrasada de outra campanha', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('não troca o nome da campanha nem a fase da que a página mostra', async () => {
    const answers = new Map<string, (value: unknown) => void>();
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: query.includes('767'),
      media: query,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    }));
    TestBed.configureTestingModule({
      providers: [
        provideRouter([{ path: 'campaigns/:id/maps/dungeon', component: DungeonNew }]),
        { provide: DungeonsClient, useValue: new FakeDungeonsClient() },
        {
          provide: CampaignsService,
          useValue: {
            getCampaign: (id: string) =>
              new Promise((resolve) => {
                answers.set(id, resolve);
              }),
          },
        },
      ],
    });
    const harness = await RouterTestingHarness.create();
    await harness.navigateByUrl('/campaigns/camp-1/maps/dungeon', DungeonNew);
    await harness.navigateByUrl('/campaigns/camp-2/maps/dungeon', DungeonNew);
    const campaign = (name: string, myRole: Role) => ({
      campaign: { name, myRole, awaitingApproval: false },
    });
    answers.get('camp-2')!(campaign('Segunda', Role.MASTER));
    await harness.fixture.whenStable();
    // The first campaign answers last, and it would read "gone" for a player.
    answers.get('camp-1')!(campaign('Primeira', Role.PLAYER));
    await harness.fixture.whenStable();
    harness.detectChanges();
    const el = harness.routeNativeElement as HTMLElement;
    expect(el.textContent).toContain('Segunda');
    expect(el.textContent).not.toContain('Primeira');
  });
});
