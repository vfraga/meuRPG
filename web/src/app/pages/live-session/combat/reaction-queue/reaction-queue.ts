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
import { CombatClient } from '../../../../core/combat/combat-client';
import {
  type ConcentrationAnswer,
  type ReactionAnswer,
} from '../../../../core/combat/combat-client-types';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { ofThe } from '../../../../core/combat/move-plan';
import {
  type QueueRow,
  type ReactionCard,
  type SlotOption,
  reactionCards,
} from '../../../../core/combat/reaction-master';
import { ActionKey } from '../../../../core/connect/idempotency';
import { autoPassText, rollingText } from '../../../../core/combat/reaction-autopass';
import {
  RollAnimator,
  type RollShow,
  showOfConcentration,
  showOfEffectSave,
  showOfReaction,
} from '../../../../shared/roll-overlay/roll-animator';
import { type EffectSaveAnswer, EffectsClient } from '../../../../core/effects/effects-client';
import { zoneCasterLabel } from '../../../../core/effects/zone-save';
import { ExtraDiceState } from '../../../../core/effects/extra-dice-state';
import { ExtraDice } from '../../effects/extra-dice/extra-dice';
import { MultiRoll, type RollField } from '../../../../shared/multi-roll/multi-roll';
import {
  bonusWord,
  concentrationBonus,
  concentrationDc,
  concentrationDice,
  concentrationLead,
  concentrationMode,
  concentrationModeLine,
  concentrationRule,
  concentrationSaid,
  concentrationTitle,
  effectSaveAbility,
  effectSaveBonus,
  effectSaveMode,
  effectSavePairHint,
  effectSaveRule,
  effectSaveSaid,
  effectSaveSkippable,
  effectSaveText,
  effectSaveTitle,
  effectSaveWho,
  zoneIdOf,
  rebukeAggressor,
  rebukeBonus,
  rebukeModifier,
  rebukeText,
  rebukeTitle,
  sayAnswer,
} from './reaction-queue-text';
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
  templateUrl: './reaction-queue.html',
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

  /** `late`: only the questions that came to the master after 10 s (they lead the "Turno"); `rest`: all the others. */
  readonly part = input<'all' | 'late' | 'rest'>('all');

  protected readonly cards = computed<readonly ReactionCard[]>(() => {
    const part = this.part();
    return reactionCards(this.encounter()).filter(
      (c) => part === 'all' || (part === 'late') === (c.type === 'unanswered'),
    );
  });

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
      .flatMap((c) =>
        c.type === 'queue' ? c.rows : c.type === 'single' || c.type === 'unanswered' ? [c.row] : [],
      )
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
      const res = await this.api.attacks.answerReaction(
        this.campaignId(),
        this.encounter().id,
        w.id,
        answer,
        this.keys.keyFor({ window: w.id, answer }),
      );
      this.state().apply(res.encounter);
      this.said.set(sayAnswer(w, answer, res.result?.result));
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
      const res = await this.api.attacks.resolveConcentrationSave(
        this.campaignId(),
        this.encounter().id,
        w.id,
        sent,
        this.keys.keyFor({ window: w.id, how: sent }),
      );
      this.state().apply(res.encounter);
      const r = res.result;
      this.play(() => showOfConcentration(r));
      this.said.set(concentrationSaid(w, how, r));
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
        showOfEffectSave(`Teste de resistência de ${effectSaveAbility(w)}`, res.result),
      );
      this.skipAsked.set('');
      this.said.set(effectSaveSaid(w, how));
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

  /** "Confirmar" of the two typed d20 (advantage or disadvantage): the server takes both faces. */
  protected effectSaveTyped(w: ReactionWindow, faces: readonly number[]): Promise<void> {
    return this.effectSave(w, {
      kind: 'typed',
      face: faces[0],
      ...(faces.length > 1 ? { second: faces[1] } : {}),
      extra: [],
    });
  }

  // What the cards say (`reaction-queue-text.ts`), under the names the template reads.
  protected readonly effectSaveMode = effectSaveMode;
  protected readonly effectSavePairHint = effectSavePairHint;
  protected readonly effectSaveAbility = effectSaveAbility;
  protected effectSaveTitle(w: ReactionWindow): string {
    return effectSaveTitle(w, zoneCasterLabel(this.encounter(), zoneIdOf(w)));
  }
  protected readonly effectSaveText = effectSaveText;
  protected readonly effectSaveRule = effectSaveRule;
  protected readonly effectSaveWho = effectSaveWho;
  protected readonly effectSaveBonus = effectSaveBonus;
  protected readonly effectSaveSkippable = effectSaveSkippable;
  protected readonly rebukeTitle = rebukeTitle;
  protected readonly rebukeAggressor = rebukeAggressor;
  protected readonly rebukeText = rebukeText;
  protected readonly rebukeBonus = rebukeBonus;
  protected readonly rebukeModifier = rebukeModifier;
  protected readonly concentrationTitle = concentrationTitle;
  protected readonly concentrationDc = concentrationDc;
  protected readonly concentrationLead = concentrationLead;
  protected readonly concentrationRule = concentrationRule;
  protected readonly concentrationMode = concentrationMode;
  protected readonly concentrationDice = concentrationDice;
  protected readonly concentrationModeLine = concentrationModeLine;
  protected readonly concentrationBonus = concentrationBonus;
  protected readonly bonusWord = bonusWord;
}
