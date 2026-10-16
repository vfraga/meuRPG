import { NgTemplateOutlet } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  input,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { Encounter, ReactionWindow } from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  CombatClient,
  type ConcentrationAnswer,
  type ReactionAnswer,
} from '../../../../core/combat/combat-client';
import { EffectPhase } from '../../../../../gen/meurpg/play/v1/lasting_effects_pb';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { ofThe } from '../../../../core/combat/move-plan';
import {
  type ConcentrationMode,
  concentrationMode,
  concentrationModeLine,
} from '../../../../core/combat/reactions';
import {
  type QueueRow,
  type ReactionCard,
  type SlotOption,
  reactionCards,
} from '../../../../core/combat/reaction-master';
import { ActionKey } from '../../../../core/connect/idempotency';
import { autoPassText, rollingText } from '../../../../core/combat/reaction-autopass';
import { rollText } from '../../../../core/combat/combat-dice';
import {
  RollAnimator,
  type RollShow,
  showOfConcentration,
  showOfEffectSave,
  showOfReaction,
} from '../../../../shared/roll-overlay/roll-animator';
import { type EffectSaveAnswer, EffectsClient } from '../../../../core/effects/effects-client';
import { ExtraDiceState } from '../../../../core/effects/extra-dice-state';
import { ExtraDice } from '../../effects/extra-dice/extra-dice';
import { MultiRoll, type RollField } from '../multi-roll/multi-roll';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';

let nextId = 0;

/**
 * The master's side of the reaction windows (PM-04b 8, PM-04c 9 and 12b, PM-04d 2): every open window of the combat,
 * as the server lists them, in the order they are answered. The windows of one action share a card ("Reações a uma
 * magia", "Reações a um ataque"), each row with its reactor, the "Jogador" or "NPC" tag and "Usar ..." with "Deixar
 * passar"; the one to answer now is first, the others wait grey ("Depois do Mago 1"). An NPC alone is a card of its
 * own ("Esperando a sua reação: ..."). A player's window reads "Respondendo no celular", and the master may answer
 * for them ("pelo jogador"). The second steps (the aggressor's saving throw of Repreensão Infernal, a concentration
 * save), and the "Sem reação" check of the table rule "Sempre", are cards too. Every answer carries its own key, made
 * once, so a repeated tap never answers twice. The numbers on these cards are the master's only (RN-20).
 */
@Component({
  selector: 'app-reaction-queue',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [ExtraDice, MatButtonModule, MatIconModule, MultiRoll, NgTemplateOutlet, RollPicker],
  template: `
    @for (card of cards(); track card.id) {
      @switch (card.type) {
        @case ('queue') {
          <section class="card" [attr.aria-labelledby]="uid + card.id">
            <h2 class="card__title" [id]="uid + card.id">{{ card.title }}</h2>
            <p class="card__sub">{{ card.subtitle }}</p>
            <ul class="rows">
              @for (row of card.rows; track row.window.id) {
                <li class="row" [class.row--wait]="!row.answerNow">
                  <ng-container *ngTemplateOutlet="rowTpl; context: { row: row }" />
                </li>
              }
            </ul>
          </section>
        }
        @case ('single') {
          <section class="card card--single" [attr.aria-label]="card.row.window.reactorLabel">
            <div class="warn">
              <mat-icon aria-hidden="true">shield</mat-icon>
              <p>{{ card.text }}</p>
            </div>
            <ng-container *ngTemplateOutlet="slotsTpl; context: { row: card.row }" />
            <div class="pair">
              <button
                mat-flat-button
                type="button"
                class="btn"
                data-initial
                [disabled]="busy() || !card.row.answerNow"
                disabledInteractive
                (click)="use(card.row)"
              >
                {{ singleUseLabel(card.row) }}
              </button>
              <button
                mat-stroked-button
                type="button"
                class="btn"
                [disabled]="busy() || !card.row.answerNow"
                disabledInteractive
                (click)="pass(card.row.window)"
              >
                Deixar passar
              </button>
            </div>
          </section>
        }
        @case ('check') {
          <section class="card card--single" [attr.aria-label]="'Sem reação: ' + card.summary">
            <h2 class="card__title">{{ card.summary }}</h2>
            @if (card.canReact) {
              <p class="card__text">Um inimigo pode usar uma reação para isto. Responda com um toque.</p>
            } @else {
              <p class="card__text">
                Nenhum inimigo tem uma reação para isto. Responda com um toque; os jogadores veem só “Esperando o mestre”.
              </p>
            }
            <div class="pair">
              <button
                mat-flat-button
                type="button"
                class="btn"
                data-initial
                [disabled]="busy()"
                (click)="pass(card.window)"
              >
                Sem reação
              </button>
            </div>
          </section>
        }
        @case ('rebukeSave') {
          <section class="card card--single" [attr.aria-labelledby]="uid + card.id">
            <h2 class="card__title" [id]="uid + card.id">{{ rebukeTitle(card.window) }}</h2>
            <p class="card__text">{{ rebukeText(card.window) }}</p>
            <p class="card__sub">{{ rebukeBonus(card.window) }}</p>
            <div class="warn">
              <mat-icon aria-hidden="true">info</mat-icon>
              <p>
                <strong>O teste do agressor é do mestre.</strong> O jogador lê só “falhou” ou “passou”.
              </p>
            </div>
            <app-roll-picker
              [outlined]="true"
              [min]="1"
              [max]="20"
              [modifier]="rebukeModifier(card.window)"
              [label]="'Role 1d20 para o teste de ' + rebukeAggressor(card.window)"
              hint="Role o seu dado e digite o número que saiu (1 a 20)."
              totalNote="Teste de Destreza"
              [appLabel]="'Rolar o teste ' + ofTheLabel(rebukeAggressor(card.window))"
              [busy]="busy()"
              (app)="answer(card.window, { use: true, die: { inApp: true } })"
              (typed)="answer(card.window, { use: true, die: { typed: $event } })"
            />
          </section>
        }
        @case ('concentration') {
          <section class="card card--single" [attr.aria-labelledby]="uid + card.id">
            <h2 class="card__title" [id]="uid + card.id">{{ concentrationTitle(card.window) }}</h2>
            @if (card.player) {
              <p class="card__text">
                {{ concentrationLead(card.window) }} O ataque que deu o dano espera o teste (CD {{ concentrationDc(card.window) }}).
              </p>
              <p class="card__sub" role="status">
                Esperando o teste de Constituição de {{ card.window.reactorLabel }}. O jogador está respondendo no celular.
              </p>
              <div class="pair">
                <button
                  mat-flat-button
                  type="button"
                  class="btn"
                  data-initial
                  [disabled]="busy()"
                  (click)="concentrate(card.window, { kind: 'app' })"
                >
                  Rolar o teste por {{ card.window.reactorLabel }}
                </button>
                <button
                  mat-stroked-button
                  type="button"
                  class="btn"
                  [disabled]="busy()"
                  (click)="concentrate(card.window, { kind: 'keep' })"
                >
                  Manter a concentração por {{ card.window.reactorLabel }}
                </button>
              </div>
            } @else {
              <p class="card__text">{{ concentrationLead(card.window) }} O ataque que deu o dano espera este teste.</p>
              <p class="card__sub">{{ concentrationRule(card.window) }}</p>
              <div class="warn">
                <mat-icon aria-hidden="true">info</mat-icon>
                <p>
                  <strong>O teste é seu.</strong> Rolar no app usa d20 {{ bonusWord(card.window) }}; você pode decidir o resultado à mão.
                </p>
              </div>
              @if (extraWindow() === card.window.id && extra.fields().length > 0) {
                <app-extra-dice [fields]="extra.fields()" [(faces)]="extra.faces" />
              }
              @if (concentrationModeLine(card.window)) {
                <p class="card__sub" role="status">{{ concentrationModeLine(card.window) }}</p>
              }
              @if (concentrationDice(card.window) === 1) {
                <app-roll-picker
                  [outlined]="true"
                  [min]="1"
                  [max]="20"
                  [modifier]="concentrationBonus(card.window)"
                  [label]="'Role 1d20 para o teste de Constituição ' + ofTheLabel(card.window.reactorLabel)"
                  hint="Role o seu dado e digite o número que saiu (1 a 20)."
                  totalNote="Teste de Constituição"
                  [appLabel]="'Rolar o teste ' + ofTheLabel(card.window.reactorLabel)"
                  [busy]="busy()"
                  (app)="concentrate(card.window, { kind: 'app' })"
                  (typed)="concentrate(card.window, { kind: 'typed', face: $event })"
                />
              } @else {
                <app-multi-roll
                  [fields]="pair"
                  [modifier]="concentrationBonus(card.window)"
                  [combine]="concentrationMode(card.window) === 'advantage' ? 'higher' : 'lower'"
                  totalNote="Teste de Constituição"
                  [appLabel]="'Rolar o teste ' + ofTheLabel(card.window.reactorLabel)"
                  hint="Role os dois d20 e digite os dois números, na ordem em que saíram (1 a 20 cada)."
                  [busy]="busy()"
                  (app)="concentrate(card.window, { kind: 'app' })"
                  (typed)="concentrate(card.window, { kind: 'typed', face: $event[0], second: $event[1] })"
                />
              }
              <button
                mat-stroked-button
                type="button"
                class="btn btn--one"
                [disabled]="busy()"
                (click)="concentrate(card.window, { kind: 'keep' })"
              >
                Manter a concentração
              </button>
            }
          </section>
        }
        @case ('effectSave') {
          <section class="card card--single" [attr.aria-labelledby]="uid + card.id">
            <h2 class="card__title" [id]="uid + card.id">{{ effectSaveTitle(card.window) }}</h2>
            <p class="card__text">{{ effectSaveText(card.window) }}</p>
            <p class="card__sub">{{ effectSaveRule(card.window) }}</p>
            <div class="warn">
              <mat-icon aria-hidden="true">info</mat-icon>
              <p>
                <strong>O teste é seu.</strong>
                {{ effectSaveWho(card.window) }} Rolar no app usa o d20; você pode decidir o resultado à mão.
              </p>
            </div>
            @if (extraWindow() === card.window.id && extra.fields().length > 0) {
              <app-extra-dice [fields]="extra.fields()" [(faces)]="extra.faces" />
            }
            @if (effectSaveMode(card.window) === 'normal') {
              <app-roll-picker
                [outlined]="true"
                [min]="1"
                [max]="20"
                [modifier]="effectSaveBonus(card.window)"
                [label]="'Role 1d20 para o teste de ' + effectSaveAbility(card.window) + ' ' + ofTheLabel(card.window.reactorLabel)"
                hint="Digite o número que saiu no dado (1 a 20), sem somar o bônus: o app soma."
                [totalNote]="'Teste de ' + effectSaveAbility(card.window)"
                [busy]="busy()"
                (app)="effectSave(card.window, { kind: 'app' })"
                (typed)="effectSave(card.window, { kind: 'typed', face: $event, extra: [] })"
              />
            } @else {
              <app-multi-roll
                [fields]="pair"
                [combine]="effectSaveMode(card.window) === 'advantage' ? 'higher' : 'lower'"
                [modifier]="effectSaveBonus(card.window)"
                [totalNote]="'Teste de ' + effectSaveAbility(card.window)"
                [hint]="effectSavePairHint(card.window)"
                [busy]="busy()"
                (app)="effectSave(card.window, { kind: 'app' })"
                (typed)="effectSaveTyped(card.window, $event)"
              />
            }
            @if (effectSaveSkippable(card.window)) {
              @if (skipAsked() === card.window.id) {
                <div class="warn">
                  <mat-icon aria-hidden="true">warning</mat-icon>
                  <p>Pular o teste dá o resultado sem rolar o dado. É uma opção da mesa. Confirmar?</p>
                </div>
                <div class="pair">
                  <button
                    mat-flat-button
                    type="button"
                    class="btn"
                    [disabled]="busy()"
                    (click)="effectSave(card.window, { kind: 'skip' })"
                  >
                    Sim, pular o teste
                  </button>
                  <button
                    mat-stroked-button
                    type="button"
                    class="btn"
                    [disabled]="busy()"
                    (click)="skipAsked.set('')"
                  >
                    Não pular
                  </button>
                </div>
              } @else {
                <button
                  mat-stroked-button
                  type="button"
                  class="btn btn--one"
                  [disabled]="busy()"
                  (click)="skipAsked.set(card.window.id)"
                >
                  Pular o teste
                </button>
              }
            }
          </section>
        }
      }
    }
    <span class="mr-visually-hidden" role="status" aria-live="polite">{{ said() }}</span>
    @if (error()) {
      <div class="mr-notice mr-notice--danger" role="alert">
        <mat-icon aria-hidden="true">error</mat-icon>
        <p>{{ error() }}</p>
      </div>
    }

    <ng-template #rowTpl let-row="row">
      <span class="avatar" aria-hidden="true">{{ row.label.charAt(0) }}</span>
      <div class="row__main">
        <p class="row__name">
          <b>{{ row.label }}</b>
          <span class="tag" [class.tag--player]="row.isPlayer">{{ row.tag }}</span>
        </p>
        <p class="row__desc">{{ row.description }}</p>
        @if (row.status) {
          <p class="row__status" [id]="uid + row.window.id + '-why'">{{ row.status }}</p>
        }
        @if (autoLeft()[row.window.id] !== undefined) {
          <p class="row__status">
            {{ row.window.rolling ? rollingText(autoLeft()[row.window.id]) : autoText(autoLeft()[row.window.id]) }}
          </p>
        }
        <ng-container *ngTemplateOutlet="slotsTpl; context: { row: row }" />
        <div class="btns">
          <button
            mat-stroked-button
            type="button"
            class="btn"
            [class.btn--off]="!row.answerNow"
            [attr.data-initial]="row.answerNow ? '' : null"
            [disabled]="busy() || !row.answerNow"
            disabledInteractive
            [attr.aria-describedby]="row.status ? uid + row.window.id + '-why' : null"
            (click)="use(row)"
          >
            {{ row.useLabel }}
          </button>
          <button
            mat-stroked-button
            type="button"
            class="btn"
            [class.btn--off]="!row.answerNow"
            [disabled]="busy() || !row.answerNow"
            disabledInteractive
            [attr.aria-describedby]="row.status ? uid + row.window.id + '-why' : null"
            (click)="pass(row.window)"
          >
            {{ row.passLabel }}
          </button>
        </div>
      </div>
    </ng-template>

    <ng-template #slotsTpl let-row="row">
      @if (row.slots.length > 0) {
        <fieldset class="slots" [disabled]="!row.answerNow">
          <legend class="slots__cap">Espaço de magia</legend>
          @for (o of row.slots; track o.title) {
            <label class="slots__row">
              <input
                type="radio"
                class="mr-visually-hidden"
                [name]="uid + row.window.id"
                [checked]="isSlot(row.window.id, o)"
                (change)="pickSlot(row.window.id, o)"
              />
              <span class="slots__dot" aria-hidden="true"></span>
              <span><b>{{ o.title }}</b> · {{ o.effect }}</span>
            </label>
          }
        </fieldset>
      }
    </ng-template>
  `,
  styleUrl: './reaction-queue.scss',
})
export class ReactionQueue {
  private readonly api = inject(CombatClient);
  private readonly effects = inject(EffectsClient);
  private readonly animator = inject(RollAnimator);
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  readonly encounter = input.required<Encounter>();
  readonly campaignId = input.required<string>();
  readonly state = input.required<CombatState>();
  /** The seconds left before a player's optional reaction passes by itself, by window id (the master's screen sends the pass). */
  readonly autoLeft = input<Readonly<Record<string, number>>>({});

  protected readonly uid = `rq-${nextId++}-`;
  protected readonly busy = signal(false);
  /** The d4 an effect adds to a typed concentration save, asked for the window whose roll the server refused. */
  protected readonly extra = new ExtraDiceState();
  protected readonly extraWindow = signal('');
  protected readonly error = signal('');
  /** The window whose "Pular o teste" waits for the master's confirmation. */
  protected readonly skipAsked = signal('');
  /** What the last answer did, for the live region. */
  protected readonly said = signal('');
  protected readonly ofTheLabel = (label: string): string => ofThe([label]);
  /** The slot the master picked for an NPC's Counterspell, by window. */
  private readonly picks = signal<Readonly<Record<string, SlotOption>>>({});
  private readonly keys = new ActionKey();

  protected readonly cards = computed<readonly ReactionCard[]>(() =>
    reactionCards(this.encounter()),
  );

  constructor() {
    // The focus opens on the first button of the first window to answer, when it is a new one.
    let first = '';
    effect(() => {
      const id = this.cards()[0]?.id ?? '';
      if (id !== first) {
        first = id;
        if (id) {
          untracked(() =>
            afterNextRender(
              () =>
                this.host.nativeElement
                  .querySelector<HTMLElement>('[data-initial]:not([disabled])')
                  ?.focus(),
              { injector: this.injector },
            ),
          );
        }
      }
    });
  }

  protected readonly autoText = autoPassText;
  protected readonly rollingText = rollingText;

  protected singleUseLabel(row: QueueRow): string {
    const slot = this.slotOf(row);
    return slot ? `${row.useLabel} (${slot.title})` : row.useLabel;
  }

  private slotOf(row: QueueRow): SlotOption | null {
    return row.slots.length === 0 ? null : (this.picks()[row.window.id] ?? row.slots[0]);
  }

  protected isSlot(windowId: string, option: SlotOption): boolean {
    const row = this.cards()
      .flatMap((c) => (c.type === 'queue' ? c.rows : c.type === 'single' ? [c.row] : []))
      .find((r) => r.window.id === windowId);
    const chosen = row ? this.slotOf(row) : null;
    return (
      !!chosen && chosen.slot.level === option.slot.level && chosen.slot.pact === option.slot.pact
    );
  }

  protected pickSlot(windowId: string, option: SlotOption): void {
    this.picks.update((p) => ({ ...p, [windowId]: option }));
  }

  /** "Usar ... pelo ...": the slot an NPC spends is the chosen one (Counterspell) or the lowest free (Escudo). */
  protected use(row: QueueRow): Promise<void> {
    const w = row.window;
    const chosen = this.slotOf(row);
    if (chosen) {
      return this.answer(w, {
        use: true,
        slot: { level: chosen.slot.level, pact: chosen.slot.pact },
      });
    }
    const shield = w.prompt.case === 'shield' ? w.prompt.value.slots : [];
    const slot = [...shield].filter((s) => s.free > 0).sort((a, b) => a.level - b.level)[0];
    return this.answer(w, {
      use: true,
      ...(slot ? { slot: { level: slot.level, pact: slot.pact } } : {}),
    });
  }

  protected pass(w: ReactionWindow): Promise<void> {
    return this.answer(w, { use: false });
  }

  protected async answer(w: ReactionWindow, answer: ReactionAnswer): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.answerReaction(
        this.campaignId(),
        this.encounter().id,
        w.id,
        answer,
        this.keys.keyFor({ window: w.id, answer }),
      );
      this.state().apply(res.encounter);
      this.said.set(this.sayAnswer(w, answer, res.result?.result));
      this.play(() => showOfReaction(res.result));
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'responder a reação'));
    } finally {
      this.busy.set(false);
    }
  }

  protected async concentrate(w: ReactionWindow, how: ConcentrationAnswer): Promise<void> {
    if (this.busy()) {
      return;
    }
    const extra = this.extraWindow() === w.id ? this.extra.take(how.kind === 'typed') : [];
    if (extra === null) {
      this.error.set(this.extra.missingText());
      return;
    }
    const sent: ConcentrationAnswer =
      how.kind === 'typed' && extra.length > 0 ? { ...how, extra } : how;
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.resolveConcentrationSave(
        this.campaignId(),
        this.encounter().id,
        w.id,
        sent,
        this.keys.keyFor({ window: w.id, how: sent }),
      );
      this.state().apply(res.encounter);
      const r = res.result;
      this.play(() => showOfConcentration(r));
      this.said.set(
        r
          ? `${r.save ? `${rollText(r.save)} ` : ''}contra CD ${r.dc}: ${w.reactorLabel} ${r.kept ? 'manteve' : 'perdeu'} a concentração${r.spellNamePt ? ` em ${r.spellNamePt}` : ''}.`
          : how.kind === 'keep'
            ? `Concentração mantida ${ofThe([w.reactorLabel])}.`
            : 'Respondido.',
      );
    } catch (err) {
      const more = this.extra.fromRefusal(err);
      if (more) {
        this.extraWindow.set(w.id);
      }
      this.error.set(more || combatErrorMessage(err, 'resolver o teste de concentração'));
    } finally {
      this.busy.set(false);
    }
  }

  /** The master's answer to an effect's saving throw (RN-22): the d20 in the app, a typed one, or skipping the save. */
  protected async effectSave(w: ReactionWindow, how: EffectSaveAnswer): Promise<void> {
    if (this.busy()) {
      return;
    }
    const extra = this.extraWindow() === w.id ? this.extra.take(how.kind === 'typed') : [];
    if (extra === null) {
      this.error.set(this.extra.missingText());
      return;
    }
    const sent: EffectSaveAnswer = how.kind === 'typed' ? { ...how, extra } : how;
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.effects.rollEffectSave(
        this.campaignId(),
        this.encounter().id,
        w.id,
        sent,
        this.keys.keyFor({ window: w.id, how: sent }),
      );
      this.state().apply(res.encounter);
      this.play(() =>
        showOfEffectSave(`Teste de resistência de ${this.effectSaveAbility(w)}`, res.result),
      );
      this.skipAsked.set('');
      this.said.set(
        how.kind === 'skip'
          ? `Teste pulado ${ofThe([w.reactorLabel])}.`
          : `Teste de resistência ${ofThe([w.reactorLabel])} resolvido.`,
      );
    } catch (err) {
      const more = this.extra.fromRefusal(err);
      if (more) {
        this.extraWindow.set(w.id);
      }
      this.error.set(more || combatErrorMessage(err, 'resolver o teste de resistência'));
    } finally {
      this.busy.set(false);
    }
  }

  /** The dice tumble; whatever happens here never turns an answered window into an error. */
  private play(make: () => RollShow | null): void {
    try {
      const show = make();
      if (show) {
        this.animator.play(show);
      }
    } catch {
      // The animation is a nicety: the result is already on the screen.
    }
  }

  /** The two d20 of an effect save with advantage or disadvantage, in the order they are rolled. */
  protected readonly pair: readonly RollField[] = [
    { key: 'd20-1', label: 'Primeiro d20', min: 1, max: 20 },
    { key: 'd20-2', label: 'Segundo d20', min: 1, max: 20 },
  ];

  protected effectSaveMode(w: ReactionWindow): 'normal' | 'advantage' | 'disadvantage' {
    const mode = this.effectSavePrompt(w)?.mode;
    return mode === 'advantage' || mode === 'disadvantage' ? mode : 'normal';
  }

  protected effectSavePairHint(w: ReactionWindow): string {
    const counts =
      this.effectSaveMode(w) === 'advantage'
        ? 'Com vantagem conta o maior'
        : 'Com desvantagem conta o menor';
    return `${counts}. Role os dois d20 e digite os números que saíram nos dados (1 a 20 cada), sem somar o bônus.`;
  }

  /** "Confirmar" of the two typed d20 (advantage or disadvantage): the server takes both faces. */
  protected effectSaveTyped(w: ReactionWindow, faces: readonly number[]): Promise<void> {
    return this.effectSave(w, {
      kind: 'typed',
      face: faces[0],
      ...(faces.length > 1 ? { second: faces[1] } : {}),
      extra: [],
    });
  }

  private effectSavePrompt(w: ReactionWindow) {
    return w.prompt.case === 'effectSave' ? w.prompt.value : null;
  }

  protected effectSaveAbility(w: ReactionWindow): string {
    return this.effectSavePrompt(w)?.abilityNamePt ?? '';
  }

  /** "Teste de resistência de Sabedoria do Goblin 1 · Riso Histérico · CD 13". */
  protected effectSaveTitle(w: ReactionWindow): string {
    const p = this.effectSavePrompt(w);
    const whose = w.reactorIsPlayer ? `de ${w.reactorLabel}` : ofThe([w.reactorLabel]);
    const parts = [
      `Teste de resistência de ${p?.abilityNamePt ?? ''} ${whose}`,
      p?.sourceNamePt ?? '',
      p?.dc !== undefined ? `CD ${p.dc}` : '',
    ];
    return parts.filter((x) => x !== '').join(' · ');
  }

  protected effectSaveText(w: ReactionWindow): string {
    const p = this.effectSavePrompt(w);
    const when = p?.phase === EffectPhase.START ? 'no começo' : 'no fim';
    return `${p?.sourceNamePt ?? 'Um efeito'} pede este teste ${when} da vez ${ofThe([w.reactorLabel])}. ${p?.textPt ?? ''}`.trim();
  }

  protected effectSaveRule(w: ReactionWindow): string {
    const p = this.effectSavePrompt(w);
    if (!p) {
      return '';
    }
    const bonus = p.bonusKnown ? `${p.abilityNamePt}: ${signed(p.modifier)}` : 'Bônus desconhecido';
    return [bonus, p.mode].filter((x) => x !== '').join(' · ');
  }

  protected effectSaveWho(w: ReactionWindow): string {
    return w.reactorIsPlayer ? `O jogador de ${w.reactorLabel} deixou a rolagem com você.` : '';
  }

  protected effectSaveBonus(w: ReactionWindow): number {
    const p = this.effectSavePrompt(w);
    return p?.bonusKnown ? p.modifier : 0;
  }

  protected effectSaveSkippable(w: ReactionWindow): boolean {
    return this.effectSavePrompt(w)?.skippable ?? false;
  }

  private sayAnswer(
    w: ReactionWindow,
    answer: ReactionAnswer,
    result: { readonly case?: string; readonly value?: unknown } | undefined,
  ): string {
    if (!answer.use) {
      return `Deixou passar ${ofThe([w.reactorLabel])}.`;
    }
    if (result?.case === 'shield') {
      const stopped = (result.value as { stopped: boolean }).stopped;
      return stopped
        ? 'Escudo Arcano usado: o ataque errou.'
        : 'Escudo Arcano usado: o ataque ainda acerta.';
    }
    return 'Reação usada.';
  }

  // ---- the second steps ----

  protected rebukeTitle(w: ReactionWindow): string {
    return `Repreensão Infernal ${w.reactorIsPlayer ? `de ${w.reactorLabel}` : ofThe([w.reactorLabel])}`;
  }

  protected rebukeAggressor(w: ReactionWindow): string {
    return w.prompt.case === 'hellishRebukeSave' ? w.prompt.value.aggressorLabel : '';
  }

  protected rebukeText(w: ReactionWindow): string {
    const p = w.prompt.case === 'hellishRebukeSave' ? w.prompt.value : null;
    return p
      ? `${p.aggressorLabel} sofre ${p.diceCount}d10 de fogo, ou metade se passar no teste de Destreza contra CD ${p.saveDc}.`
      : '';
  }

  protected rebukeBonus(w: ReactionWindow): string {
    const p = w.prompt.case === 'hellishRebukeSave' ? w.prompt.value : null;
    return p
      ? `Destreza ${ofThe([p.aggressorLabel])}: ${p.bonusKnown ? signed(p.saveBonus) : 'desconhecida'} · o teste é seu`
      : '';
  }

  protected rebukeModifier(w: ReactionWindow): number {
    const p = w.prompt.case === 'hellishRebukeSave' ? w.prompt.value : null;
    return p?.bonusKnown ? p.saveBonus : 0;
  }

  protected concentrationTitle(w: ReactionWindow): string {
    const damage = w.trigger?.damageTaken ?? 0;
    return `${w.reactorLabel} sofreu ${damage} de dano`;
  }

  private concentrationSpell(w: ReactionWindow): string {
    return w.prompt.case === 'concentrationSave'
      ? w.prompt.value.spellNamePt
      : (w.trigger?.concentrationSpellKey ?? '');
  }

  protected concentrationDc(w: ReactionWindow): number {
    return w.prompt.case === 'concentrationSave' ? w.prompt.value.dc : 10;
  }

  protected concentrationLead(w: ReactionWindow): string {
    return `${w.reactorLabel} concentra em ${this.concentrationSpell(w)}.`;
  }

  protected concentrationRule(w: ReactionWindow): string {
    const damage = w.trigger?.damageTaken ?? 0;
    const p = w.prompt.case === 'concentrationSave' ? w.prompt.value : null;
    const bonus = p?.bonusKnown
      ? ` · Constituição ${ofThe([w.reactorLabel])}: ${signed(p.saveBonus)}`
      : '';
    return `Teste de resistência de Constituição contra CD ${this.concentrationDc(w)} (metade de ${damage} seria ${Math.floor(damage / 2)}; o mínimo é 10)${bonus}`;
  }

  protected concentrationMode(w: ReactionWindow): ConcentrationMode {
    return w.prompt.case === 'concentrationSave' ? concentrationMode(w.prompt.value) : 'normal';
  }

  /** How many d20 a typed save takes: two with advantage or disadvantage. */
  protected concentrationDice(w: ReactionWindow): number {
    return this.concentrationMode(w) === 'normal' ? 1 : 2;
  }

  protected concentrationModeLine(w: ReactionWindow): string {
    return w.prompt.case === 'concentrationSave'
      ? concentrationModeLine(w.prompt.value, w.reactorLabel)
      : '';
  }

  protected concentrationBonus(w: ReactionWindow): number {
    return w.prompt.case === 'concentrationSave' && w.prompt.value.bonusKnown
      ? w.prompt.value.saveBonus
      : 0;
  }

  protected bonusWord(w: ReactionWindow): string {
    const bonus = this.concentrationBonus(w);
    return `${bonus < 0 ? '−' : '+'} ${Math.abs(bonus)}`;
  }
}

function signed(n: number): string {
  return n < 0 ? `−${Math.abs(n)}` : `+${n}`;
}
