import {
  ChangeDetectionStrategy,
  Component,
  computed,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { Encounter } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { HideAttemptView } from '../../../../../gen/meurpg/play/v1/contest_types_pb';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { ContestClient } from '../../../../core/combat/contest-client';
import {
  REFUSAL_MAX,
  applyLabel,
  notNoticing,
  observerRows,
} from '../../../../core/combat/contest-master';
import type { ContestState } from '../../../../core/combat/contest-state';
import { hiddenWord } from '../../../../core/combat/contest-view';
import { ActionKey } from '../../../../core/connect/idempotency';

let nextId = 0;

/**
 * "Brisa tenta se esconder: Furtividade 19" (W7-X, board W7-Xc 8, the master's side): the Hide that waits for him. The total
 * against the passive Perception of each creature that could see the hider (his only: the hider reads "Você está escondido" and
 * nothing more, RN-10 and RN-20), a "Vê claramente" key on each row for the cover the app does not see, and the decision:
 * "Aplicar: escondido (3 não notam)" (one state per creature) or "Recusar: não há onde se esconder" with a one-sentence reason
 * the hider reads. A tie keeps the creature noticed (the app's reading). Answers with `ResolveHide`; the key is made once.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-hide-master-card',
  imports: [MatButtonModule, MatIconModule],
  styleUrl: './master-card.scss',
  template: `
    <section class="card" [attr.aria-labelledby]="uid + 't'" data-testid="hide-card">
      <h2 class="card__title" [id]="uid + 't'">{{ title() }}</h2>
      <p class="card__sub">{{ sub() }}</p>
      <ul class="rows">
        @for (r of rows(); track r.id) {
          <li class="row">
            <span class="row__main">
              <span class="row__name">{{ r.label }}</span>
              <span class="row__sub">{{ r.passive }}</span>
            </span>
            <span class="tag" [class.tag--ok]="!noticed(r)">{{ noticed(r) ? 'Nota' : 'Não nota' }}</span>
            <label class="choice" [class.choice--on]="clearly().has(r.id)">
              <input
                type="checkbox"
                class="mr-visually-hidden"
                [checked]="clearly().has(r.id)"
                [disabled]="busy()"
                (change)="toggle(r.id)"
              />
              <span class="choice__name">Vê claramente</span>
            </label>
          </li>
        }
      </ul>

      @if (refusing()) {
        <div class="field">
          <label [for]="uid + 'r'">Motivo, em uma frase (a jogadora lê)</label>
          <input
            [id]="uid + 'r'"
            type="text"
            [attr.maxlength]="max"
            autocomplete="off"
            [value]="reason()"
            (input)="typeReason($event)"
          />
          <span class="small">Vazio: “Alguém vê você claramente: não dá para se esconder agora.”</span>
        </div>
      }

      @if (error()) {
        <div class="mr-notice mr-notice--danger" role="alert">
          <mat-icon aria-hidden="true">error</mat-icon>
          <p>{{ error() }}</p>
        </div>
      }

      <div class="btns">
        @if (refusing()) {
          <button matButton="filled" type="button" class="btn" [disabled]="busy()" (click)="decide(true)">
            Recusar o esconderijo
          </button>
          <button matButton="outlined" type="button" class="btn" [disabled]="busy()" (click)="refusing.set(false)">
            Voltar
          </button>
        } @else {
          <button matButton="filled" type="button" class="btn" data-initial [disabled]="busy()" (click)="decide(false)">
            {{ applyText() }}
          </button>
          <button matButton="outlined" type="button" class="btn" [disabled]="busy()" (click)="refusing.set(true)">
            Recusar: não há onde se esconder
          </button>
        }
      </div>
      <p class="small">Empate: continua notado (leitura do app: o total precisa superar a Percepção passiva).</p>
    </section>
  `,
})
export class HideMasterCard {
  private readonly api = inject(ContestClient);
  private readonly keys = new ActionKey();

  readonly attempt = input.required<HideAttemptView>();
  readonly encounter = input.required<Encounter | null>();
  readonly campaignId = input.required<string>();
  readonly state = input.required<CombatState>();
  readonly contests = input.required<ContestState>();

  readonly said = output<string>();

  protected readonly uid = `hm-${nextId++}-`;
  protected readonly max = REFUSAL_MAX;
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly refusing = signal(false);
  protected readonly reason = signal('');
  /** The creatures the master marked "Vê claramente". */
  protected readonly clearly = signal<ReadonlySet<string>>(new Set());

  private readonly hiderLabel = computed(
    () => this.encounter()?.combatants.find((c) => c.id === this.attempt().hiderId)?.label ?? '',
  );
  protected readonly title = computed(
    () => `${this.hiderLabel()} tenta se esconder: Furtividade ${this.attempt().roll?.total ?? 0}`,
  );
  protected readonly sub = computed(() => {
    return 'Compare com a Percepção passiva de quem poderia ver quem se esconde. Quem vê claramente quem se esconde não é enganado.';
  });
  protected readonly rows = computed(() => observerRows(this.encounter(), this.attempt()));
  protected readonly applyText = computed(() =>
    applyLabel(hiddenWord(), notNoticing(this.rows(), this.clearly())),
  );

  protected noticed(r: { readonly id: string; readonly noticed: boolean }): boolean {
    return r.noticed || this.clearly().has(r.id);
  }

  protected typeReason(event: Event): void {
    this.reason.set((event.target as HTMLInputElement).value);
  }

  protected toggle(id: string): void {
    this.clearly.update((set) => {
      const next = new Set(set);
      if (!next.delete(id)) {
        next.add(id);
      }
      return next;
    });
  }

  protected async decide(refuse: boolean): Promise<void> {
    if (this.busy()) {
      return;
    }
    const decision = {
      attemptId: this.attempt().id,
      refuse,
      refusal: refuse ? this.reason().trim() : '',
      seesClearlyIds: refuse ? [] : [...this.clearly()].sort(),
    };
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.resolveHide(
        this.campaignId(),
        this.encounter()?.id ?? '',
        decision,
        this.keys.keyFor(decision),
      );
      this.state().apply(res.encounter);
      this.contests().applyAttempt(res.attempt);
      this.said.emit(
        refuse
          ? `Esconderijo de ${this.hiderLabel()} recusado.`
          : `${this.hiderLabel()} está ${hiddenWord()}.`,
      );
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'decidir o esconderijo'));
    } finally {
      this.busy.set(false);
    }
  }
}
