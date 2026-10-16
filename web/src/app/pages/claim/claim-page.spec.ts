import { Injectable, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  CharacterBlockedReason,
  CharacterBlockedSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { AuthService, type AuthState } from '../../core/auth/auth.service';
import { ClaimsService } from '../../core/characters/claims.service';
import { ClaimPage } from './claim-page';
import { nextTurn } from '../../../test-settle';

@Injectable()
class FakeAuthService {
  private readonly stateSignal = signal<AuthState>({ status: 'unknown' });
  readonly state = this.stateSignal.asReadonly();
  readonly refresh = vi.fn(() => Promise.resolve());
  readonly endSessionHere = vi.fn(() => Promise.resolve());

  set(state: AuthState): void {
    this.stateSignal.set(state);
  }
}

@Injectable()
class FakeClaimsService {
  readonly preview = vi.fn();
  readonly claim = vi.fn();
}

const SIGNED_IN: AuthState = {
  status: 'signed-in',
  user: { id: 'u1', displayName: 'Lia' },
  sessionExpiresAt: null,
};

const CARD = {
  ownLink: false,
  card: {
    characterName: 'Kai',
    raceNamePt: 'Humano',
    classSummary: 'Monge 5',
    campaignName: 'Mirathel',
    sentByDisplayName: 'Vinicius',
  },
};

function blocked(reason: CharacterBlockedReason, extra: Record<string, string> = {}): ConnectError {
  return new ConnectError('blocked', Code.FailedPrecondition, undefined, [
    { desc: CharacterBlockedSchema, value: { reason, ...extra } },
  ]);
}

function text(fixture: { nativeElement: unknown }): string {
  return ((fixture.nativeElement as HTMLElement).textContent ?? '').replace(/\s+/g, ' ');
}

function button(fixture: { nativeElement: unknown }, label: string): HTMLElement {
  const el = Array.from(
    (fixture.nativeElement as HTMLElement).querySelectorAll<HTMLElement>('button, a'),
  ).find((b) => b.textContent?.includes(label));
  if (!el) {
    throw new Error(`no button "${label}" in: ${text(fixture)}`);
  }
  return el;
}

describe('ClaimPage (MR-049)', () => {
  let auth: FakeAuthService;
  let claims: FakeClaimsService;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [ClaimPage],
      providers: [
        provideRouter([]),
        { provide: AuthService, useClass: FakeAuthService },
        { provide: ClaimsService, useClass: FakeClaimsService },
      ],
    });
    auth = TestBed.inject(AuthService) as unknown as FakeAuthService;
    claims = TestBed.inject(ClaimsService) as unknown as FakeClaimsService;
  });

  afterEach(() => {
    window.location.hash = '';
    document.querySelectorAll('form[action="/auth/login"]').forEach((f) => f.remove());
  });

  async function open(hash: string, state: AuthState) {
    window.location.hash = hash;
    const fixture = TestBed.createComponent(ClaimPage);
    fixture.detectChanges();
    auth.set(state);
    await nextTurn();
    fixture.detectChanges();
    return fixture;
  }

  it('reads the secret from the fragment and strips it at once, never touching storage', () => {
    window.location.hash = '#t=super-secret-token';
    const replaceState = vi.spyOn(history, 'replaceState');
    const setItem = vi.spyOn(Storage.prototype, 'setItem');

    TestBed.createComponent(ClaimPage);

    expect(replaceState).toHaveBeenCalledTimes(1);
    expect(window.location.hash).toBe('');
    expect(setItem).not.toHaveBeenCalled();
  });

  it('another link opened in the same tab starts the page over with the new secret', async () => {
    claims.preview.mockResolvedValueOnce(CARD);
    const fixture = await open('#t=first', SIGNED_IN);
    expect(claims.preview).toHaveBeenLastCalledWith('first');
    expect(text(fixture)).toContain('Kai');

    claims.preview.mockResolvedValueOnce({
      ...CARD,
      card: { ...CARD.card, characterName: 'Ragna' },
    });
    window.location.hash = '#t=second';
    await nextTurn();
    fixture.detectChanges();

    expect(claims.preview).toHaveBeenLastCalledWith('second');
    expect(text(fixture)).toContain('Ragna');
    expect(window.location.hash).toBe('');
  });

  describe('signed out', () => {
    it('is the same page for every link, valid or not, and asks the server nothing', async () => {
      const withLink = await open('#t=link-one', { status: 'signed-out' });
      const first = text(withLink);
      TestBed.resetTestingModule();
      TestBed.configureTestingModule({
        imports: [ClaimPage],
        providers: [
          provideRouter([]),
          { provide: AuthService, useClass: FakeAuthService },
          { provide: ClaimsService, useClass: FakeClaimsService },
        ],
      });
      auth = TestBed.inject(AuthService) as unknown as FakeAuthService;
      claims = TestBed.inject(ClaimsService) as unknown as FakeClaimsService;
      const withoutLink = await open('', { status: 'signed-out' });

      expect(text(withoutLink)).toBe(first);
      expect(first).toContain('Assumir um personagem');
      expect(first).toContain('Entrar com Google');
      expect(first).toContain('Entrar não assume nada');
      expect(claims.preview).not.toHaveBeenCalled();
      expect(claims.claim).not.toHaveBeenCalled();
      // No campaign, no character: nothing about this link is on the page.
      expect(first).not.toContain('Campanha');
    });

    it('"Entrar com Google" signs in with the claim intent and the secret in the form body, and never claims', async () => {
      const submit = vi
        .spyOn(HTMLFormElement.prototype, 'submit')
        .mockImplementation(() => undefined);
      const fixture = await open('#t=super-secret-token', { status: 'signed-out' });

      button(fixture, 'Entrar com Google').click();

      const form = document.querySelector('form[action="/auth/login"]') as HTMLFormElement;
      expect(form.method).toBe('post');
      const value = (name: string) =>
        (form.querySelector(`input[name="${name}"]`) as HTMLInputElement | null)?.value;
      expect(value('return_to')).toBe('/claim');
      expect(value('intent')).toBe('character_claim');
      expect(value('intent_payload')).toBe('super-secret-token');
      expect(value('prompt')).toBeUndefined();
      expect(submit).toHaveBeenCalled();
      expect(claims.claim).not.toHaveBeenCalled();
    });

    it('with no secret in the address, signs in plainly: there is nothing to bring back', async () => {
      vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(() => undefined);
      const fixture = await open('', { status: 'signed-out' });

      button(fixture, 'Entrar com Google').click();

      const form = document.querySelector('form[action="/auth/login"]') as HTMLFormElement;
      expect(form.querySelector('input[name="intent"]')).toBeNull();
      expect(form.querySelector('input[name="intent_payload"]')).toBeNull();
    });
  });

  describe('signed in', () => {
    it('shows the public card with who sent it, and claims only on "Assumir este personagem"', async () => {
      claims.preview.mockResolvedValue(CARD);
      claims.claim.mockResolvedValue({
        campaignId: 'camp-1',
        campaignName: 'Mirathel',
        characterId: 'char-1',
        characterName: 'Kai',
      });
      const fixture = await open('#t=abc', SIGNED_IN);

      expect(claims.preview).toHaveBeenCalledWith('abc');
      const card = text(fixture);
      expect(card).toContain('Este personagem é seu?');
      expect(card).toContain('Campanha Mirathel');
      expect(card).toContain('Kai');
      expect(card).toContain('Humano · Monge 5');
      expect(card).toContain('Enviado por Vinicius (mestre de Mirathel).');
      expect(card).toContain('Você entrou como Lia.');
      expect(card).toContain('Não é você? Entrar com outra conta');
      expect(card).not.toMatch(/RN-\d/);
      expect(claims.claim).not.toHaveBeenCalled();

      button(fixture, 'Assumir este personagem').click();
      await nextTurn();
      fixture.detectChanges();

      expect(claims.claim).toHaveBeenCalledWith('abc');
      expect(text(fixture)).toContain('Pronto: Kai é seu');
      expect(text(fixture)).toContain('Você agora é membro de Mirathel e o dono de Kai.');
      expect(button(fixture, 'Abrir a ficha de Kai').getAttribute('href')).toBe(
        '/campaigns/camp-1/characters/char-1',
      );
      expect(button(fixture, 'Ir para a campanha').getAttribute('href')).toBe('/campaigns/camp-1');
    });

    it('back from the sign-in there is no secret, and the page asks with an empty one', async () => {
      claims.preview.mockResolvedValue(CARD);
      await open('', SIGNED_IN);
      expect(claims.preview).toHaveBeenCalledWith('');
    });

    it('says who sent it in words that do not need a name', async () => {
      claims.preview.mockResolvedValue({
        ...CARD,
        card: { ...CARD.card, sentByDisplayName: '' },
      });
      const fixture = await open('#t=abc', { ...SIGNED_IN, user: { id: 'u1', displayName: null } });

      expect(text(fixture)).toContain('Enviado pelo mestre de Mirathel.');
      expect(text(fixture)).not.toContain('Você entrou como');
    });

    it('every link that cannot be used is one page: invalid, expired, used, revoked, lost race', async () => {
      claims.preview.mockRejectedValue(
        new ConnectError('this claim link cannot be used', Code.NotFound),
      );
      const fixture = await open('#t=abc', SIGNED_IN);

      expect(text(fixture)).toContain('Este link não pode ser usado');
      expect(text(fixture)).toContain('Este link não existe ou não vale mais.');
      expect(text(fixture)).toContain('Peça ao mestre um link novo.');
      expect(button(fixture, 'Ir para minhas campanhas').getAttribute('href')).toBe('/campaigns');
    });

    it('a claim that loses a race shows the same page', async () => {
      claims.preview.mockResolvedValue(CARD);
      claims.claim.mockRejectedValue(new ConnectError('gone', Code.NotFound));
      const fixture = await open('#t=abc', SIGNED_IN);

      button(fixture, 'Assumir este personagem').click();
      await nextTurn();
      fixture.detectChanges();

      expect(text(fixture)).toContain('Este link não pode ser usado');
    });

    it('the master opening their own link is told it is for a player', async () => {
      claims.preview.mockResolvedValue({ ownLink: true, card: undefined });
      const fixture = await open('#t=abc', SIGNED_IN);

      expect(text(fixture)).toContain('Este link é para um jogador. Copie e envie para ele.');
      expect(text(fixture)).not.toContain('Assumir este personagem');
    });

    it('RN-03 is the one specific refusal: it names the living character and never shows a rule id', async () => {
      claims.preview.mockResolvedValue(CARD);
      claims.claim.mockRejectedValue(
        blocked(CharacterBlockedReason.LIVING_CHARACTER_EXISTS, {
          characterId: 'char-9',
          characterName: 'Ícaro',
          campaignId: 'camp-1',
        }),
      );
      const fixture = await open('#t=abc', SIGNED_IN);

      button(fixture, 'Assumir este personagem').click();
      await nextTurn();
      fixture.detectChanges();

      const page = text(fixture);
      expect(page).toContain('Você já tem um personagem vivo aqui');
      expect(page).toContain('Você já tem um personagem vivo nesta campanha: Ícaro.');
      expect(page).toContain('Cada jogador tem um personagem por campanha.');
      expect(page).toContain(
        'fale com o mestre: dá para devolver Ícaro à reserva ou mandar o link de Kai para outra pessoa.',
      );
      expect(page).not.toMatch(/RN-\d/);
      expect(button(fixture, 'Abrir a ficha de Ícaro').getAttribute('href')).toBe(
        '/campaigns/camp-1/characters/char-9',
      );
    });

    it('a server failure offers "Tentar de novo" with the secret kept in memory', async () => {
      claims.preview.mockRejectedValueOnce(new ConnectError('down', Code.Unavailable));
      claims.preview.mockResolvedValueOnce(CARD);
      const fixture = await open('#t=abc', SIGNED_IN);

      expect(text(fixture)).toContain('Não foi possível falar com o servidor agora');
      button(fixture, 'Tentar de novo').click();
      await nextTurn();
      fixture.detectChanges();

      expect(claims.preview).toHaveBeenLastCalledWith('abc');
      expect(text(fixture)).toContain('Este personagem é seu?');
    });

    it('a failed claim tries again with the same secret', async () => {
      claims.preview.mockResolvedValue(CARD);
      claims.claim.mockRejectedValueOnce(new ConnectError('down', Code.Unavailable));
      claims.claim.mockResolvedValueOnce({
        campaignId: 'camp-1',
        campaignName: 'Mirathel',
        characterId: 'char-1',
        characterName: 'Kai',
      });
      const fixture = await open('#t=abc', SIGNED_IN);

      button(fixture, 'Assumir este personagem').click();
      await nextTurn();
      fixture.detectChanges();
      button(fixture, 'Tentar de novo').click();
      await nextTurn();
      fixture.detectChanges();

      expect(claims.claim).toHaveBeenCalledTimes(2);
      expect(claims.claim).toHaveBeenLastCalledWith('abc');
      expect(text(fixture)).toContain('Pronto: Kai é seu');
    });

    it('"Não é você? Entrar com outra conta" ends this session and asks for the account chooser, carrying the secret', async () => {
      const submit = vi
        .spyOn(HTMLFormElement.prototype, 'submit')
        .mockImplementation(() => undefined);
      claims.preview.mockResolvedValue(CARD);
      const fixture = await open('#t=abc', SIGNED_IN);

      button(fixture, 'Não é você?').click();
      await nextTurn();

      expect(auth.endSessionHere).toHaveBeenCalled();
      const form = document.querySelector('form[action="/auth/login"]') as HTMLFormElement;
      const value = (name: string) =>
        (form.querySelector(`input[name="${name}"]`) as HTMLInputElement).value;
      expect(value('prompt')).toBe('select_account');
      expect(value('intent')).toBe('character_claim');
      expect(value('intent_payload')).toBe('abc');
      expect(submit).toHaveBeenCalled();
      expect(claims.claim).not.toHaveBeenCalled();
    });

    it('an unavailable session says so instead of showing the signed-out page', async () => {
      const fixture = await open('#t=abc', { status: 'unavailable' });

      expect(text(fixture)).toContain('Não foi possível confirmar sua sessão');
      expect(text(fixture)).not.toContain('Entrar com Google');
    });
  });
});
