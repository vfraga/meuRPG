import { Component, computed, inject, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { RouterLink } from '@angular/router';
import { Code } from '@connectrpc/connect';

import { AuthService } from '../../core/auth/auth.service';
import { describeConnectError } from '../../core/connect/connect-errors';
import { ServerInfoService } from '../../core/system/server-info.service';

/** State of the one call this page makes, kept as a single signal so the
 * template only ever renders one of the three cases (no impossible states). */
type ServerInfoState =
  | { status: 'loading' }
  | { status: 'ready'; version: string; commit: string }
  | { status: 'error'; message: string };

/**
 * "/": for a visitor, what MeuRPG is for this table and "Entrar"; for
 * someone signed in, a greeting and the way to "Minhas campanhas" (plus a
 * nudge to "Meu perfil" while they have no display name, which is how the
 * rest of the table sees them).
 *
 * At the bottom, quietly, the server's version (`GetServerInfo`): the
 * e2e smoke test (`e2e/tests/app.spec.ts`) reads "Servidor conectado" and
 * the `<dl>`'s "Versão".
 */
@Component({
  selector: 'app-home',
  imports: [MatButtonModule, MatIconModule, RouterLink],
  templateUrl: './home.html',
  styleUrl: './home.scss',
})
export class Home {
  private readonly serverInfo = inject(ServerInfoService);
  private readonly auth = inject(AuthService);

  protected readonly state = signal<ServerInfoState>({ status: 'loading' });
  protected readonly authState = this.auth.state;

  /** Where "Entrar" goes: the server's sign-in, back to "Minhas campanhas"
   * afterwards (the same URL `AuthService.signIn('/campaigns')` opens). */
  protected readonly signInHref = `/auth/login?return_to=${encodeURIComponent('/campaigns')}`;

  private readonly displayName = computed(() => {
    const auth = this.auth.state();
    return auth.status === 'signed-in' ? auth.user.displayName : null;
  });

  protected readonly hasDisplayName = computed(() => this.displayName() !== null);

  protected readonly greeting = computed(() => {
    const name = this.displayName();
    return name ? `Olá, ${name}` : 'Olá';
  });

  constructor() {
    this.load();
  }

  protected retry(): void {
    this.state.set({ status: 'loading' });
    this.load();
  }

  private load(): void {
    // A single two-argument `.then` (rather than `.then().catch()`) settles
    // in one microtask either way, which keeps success and failure equally
    // fast and easy to await in tests.
    this.serverInfo.getServerInfo().then(
      (res) => this.state.set({ status: 'ready', version: res.version, commit: res.commit }),
      (err: unknown) => {
        // ConnectError.from() also handles plain network failures (the
        // fetch failing before it ever reaches the server), not just RPCs
        // that came back with an error.
        this.state.set({
          status: 'error',
          message: describeConnectError(err, { [Code.Unavailable]: 'Tente de novo em instantes.' }),
        });
      },
    );
  }
}
