import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  Injector,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { MountSavePrompt, MountSaveResult } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { AnswerHeldRollResponse } from '../../../../../gen/meurpg/play/v1/resources_pb';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import { CombatClient } from '../../../../core/combat/combat-client';
import type { MountSaveRoll } from '../../../../core/combat/combat-client-types';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { mountSaveResultView, mountSaveView } from '../../../../core/combat/mount-save';
import { ActionKey } from '../../../../core/connect/idempotency';
import { genericDieFields, missingDice } from '../../../../core/effects/effects';
import { HeldFollow } from '../../../../core/held-rolls/held-follow';
import { HeldRollController } from '../../../../core/held-rolls/held-roll-controller';
import { type HeldAnswer, heldOfferOf } from '../../../../core/held-rolls/held-roll-offer';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { HeldRollPrompt } from '../../../../shared/held-roll-prompt/held-roll-prompt';
import { rerollShow } from '../../../../shared/held-roll-prompt/held-roll-shows';
import { MultiRoll, type RollField } from '../../../../shared/multi-roll/multi-roll';
import { RollAnimator } from '../../../../shared/roll-overlay/roll-animator';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../../shared/sheet/sheet-host';
import { ExtraDice } from '../../effects/extra-dice/extra-dice';

/** What the page hands the sheet of a rider's save to stay mounted. */
export interface MountSaveSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  /** `Encounter.mount_save`, as the server sent it to this reader. */
  readonly prompt: MountSavePrompt;
  /** The master rolls for the rider ("Rolar por Tor"): the rider is read by name and the sheet may be closed. */
  readonly byMaster: boolean;
  readonly state: CombatState;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
}

type Stage = 'ask' | 'result' | 'gone';

/** The d4 of Bênção and Perdição. */
const EFFECT_DIE_FACES = 4;

/**
 * "Você cai da montaria?" (#588, boards 588-06 and 588-10 card 6): the Dexterity saving throw of a rider whose mount was moved
 * against its will, or who was knocked prone while mounted, answered with `RollMountSave`. It opens by itself for the rider's
 * player, who has to answer it (the turn waits), and from "Rolar por Tor" for the master, who may close it. It says the cause,
 * the roll and the bonus; the DC only where the server sent it (the master, or a table that shows save DCs). "Rolar no app", or
 * "Digitar o resultado" (the d20, the second one with advantage or disadvantage, and the d4 an effect adds once the server asks
 * for it). A rider that fails by itself has no d20: one button, "Continuar". A d20 kept for a question (Lucky, a Bardic
 * Inspiration die) asks it here before the result. The roll has its own key, so a repeated tap never rolls twice.
 */
@Component({
  selector: 'app-mount-save-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    ExtraDice,
    HeldRollPrompt,
    MatButtonModule,
    MatIconModule,
    MultiRoll,
    RollPicker,
    SheetFrame,
  ],
  host: { '(keydown.escape)': 'close()' },
  templateUrl: './mount-save-sheet.html',
  styleUrls: [
    '../resource-pick/resource-sheet.scss',
    '../../effects/effect-save-sheet/effect-save-sheet.scss',
    './mount-save-sheet.scss',
  ],
})
export class MountSaveSheet {
  private readonly api = inject(CombatClient);
  /** Read when the player answers about the die: a sheet that never asks does not build the client. */
  private readonly lazyInjector = inject(Injector);
  private readonly animator = inject(RollAnimator);
  private readonly sheet = injectSheet<MountSaveSheetData, void>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly typing = signal(false);
  protected readonly result = signal<MountSaveResult | null>(null);
  protected readonly extraFaces = signal<readonly (number | null)[]>([]);
  /** The d4 asked when the server refused a typed roll for lack of them. */
  private readonly asked = signal(0);
  protected readonly fields = computed(() => genericDieFields(this.asked(), EFFECT_DIE_FACES));

  protected readonly held = new HeldRollController({
    send: (holdId, answer, key) =>
      this.lazyInjector
        .get(ResourceClient)
        .answerHeldRoll(this.data.campaignId, holdId, answer, key),
    settle: (res, done) => this.answeredHeld(res, done.answer),
    busy: this.busy,
    error: this.error,
  });
  /** Tells the roller that the master answered the question for them. */
  protected readonly follow = new HeldFollow(this.held);
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));

  protected readonly canApp = this.data.diceMode !== DiceMode.PHYSICAL;
  protected readonly canType = this.data.diceMode !== DiceMode.APP;
  protected readonly preferApp =
    effectivePreference(this.data.diceMode, this.data.preference) === DicePreference.APP;
  protected readonly typingNow = computed(() => this.typing() || !this.canApp);
  protected readonly pair: readonly RollField[] = [
    { key: 'd20-1', label: 'Primeiro d20', min: 1, max: 20 },
    { key: 'd20-2', label: 'Segundo d20', min: 1, max: 20 },
  ];

  /** The words as the combat was when the sheet opened: the roll moves the turn and may end the ride. */
  private readonly opened = this.data.state.encounter();
  protected readonly view = this.opened
    ? mountSaveView(this.opened, this.data.prompt, !this.data.byMaster)
    : null;
  private readonly names = {
    rider: this.label(this.data.prompt.riderId),
    mount: this.label(this.data.prompt.mountId),
  };
  protected readonly resultView = computed(() => {
    const r = this.result();
    return r ? mountSaveResultView(r, !this.data.byMaster, this.names) : null;
  });

  private readonly present = computed(
    () => this.data.state.encounter()?.mountSave?.riderId === this.data.prompt.riderId,
  );
  protected readonly stage = computed<Stage>(() => {
    if (this.result()) {
      return 'result';
    }
    return this.present() || this.busy() || this.held.offer() ? 'ask' : 'gone';
  });
  /** The rider's player owes the answer: no "Fechar" and no Esc while the save is asked. */
  protected readonly canClose = computed(
    () => !this.busy() && (this.data.byMaster || this.stage() !== 'ask'),
  );
  protected readonly goneText = this.data.byMaster
    ? 'Este teste já foi rolado, ou deixou de ser preciso.'
    : 'Este teste não espera mais a sua rolagem: o mestre rolou, ou ele deixou de ser preciso.';

  /** One key per roll (how it is rolled): the same tap again is a retry, another way is a new request. */
  private readonly keys = new ActionKey();

  constructor() {
    // The d20 was rolled and kept before this sheet opened (a reload): its question is what is owed, not another roll.
    const kept = this.opened?.combatants.find((c) => c.id === this.data.prompt.riderId);
    const offer = kept ? heldOfferOf(kept) : undefined;
    if (offer) {
      this.held.ask(offer);
    }
    // The master answers the question for the roller: the sheet drops it, so it never offers an answer that is no longer open.
    effect(() => {
      const rider = this.data.state
        .encounter()
        ?.combatants.find((c) => c.id === this.data.prompt.riderId);
      this.held.offer();
      this.held.refusal();
      if (rider) {
        untracked(() => this.follow.see(heldOfferOf(rider)));
      }
    });
  }

  private label(id: string): string {
    return (id && this.opened?.combatants.find((c) => c.id === id)?.label) || '';
  }

  /** "Confirmar" of the typed d20 (and the second one, with advantage or disadvantage). */
  protected answerTyped(faces: readonly number[]): Promise<void> {
    const need = this.fields();
    const typed = this.extraFaces();
    if (need.length > 0 && (typed.length !== need.length || typed.some((f) => f === null))) {
      this.error.set('Digite o resultado de cada d4 antes de confirmar.');
      return Promise.resolve();
    }
    return this.roll({
      kind: 'typed',
      face: faces[0],
      ...(faces.length > 1 ? { second: faces[1] } : {}),
      extra: need.length > 0 ? (typed as readonly number[]) : [],
    });
  }

  protected async roll(how: MountSaveRoll): Promise<void> {
    if (this.busy()) {
      return;
    }
    const riderId = this.data.prompt.riderId;
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.moves.rollMountSave(
        this.data.campaignId,
        this.data.encounterId,
        riderId,
        how,
        this.keys.keyFor({ rider: riderId, how }),
      );
      this.data.state.apply(res.encounter);
      this.typing.set(false);
      if (res.held) {
        // Nothing was written: the d20 alone, and the result after the answer.
        this.held.ask(res.held);
        return;
      }
      this.keys.renew();
      if (!res.result) {
        throw new Error('RollMountSave answered without its result');
      }
      this.settle(res.result);
    } catch (err) {
      this.failed(err);
    } finally {
      this.busy.set(false);
    }
  }

  private answeredHeld(res: AnswerHeldRollResponse, answer: HeldAnswer): void {
    const settled = res.result.case === 'mountSave' ? res.result.value : null;
    if (!settled?.encounter || (!settled.result && !res.next)) {
      throw new Error('AnswerHeldRoll answered without its saving throw');
    }
    this.data.state.apply(settled.encounter);
    if (res.next) {
      const show = answer.kind === 'lucky' ? rerollShow(res.next.d20, answer.roll) : null;
      if (show) {
        this.animator.play(show);
      }
      return;
    }
    this.keys.renew();
    this.settle(settled.result!);
  }

  /** The result takes the sheet, and the d20 the app rolled tumbles with the same words. */
  private settle(r: MountSaveResult): void {
    this.result.set(r);
    const view = this.resultView();
    const roll = r.save;
    if (!view || !roll || r.autoFail) {
      return;
    }
    this.animator.play({
      label: 'Teste de resistência de Destreza',
      dice: roll.faces.map((face, i) => ({
        sides: 20,
        face,
        counts: roll.faces.length === 1 || i === roll.countedIndex,
      })),
      line: `Total ${roll.total}`,
      outcome: { word: view.word, good: view.good },
    });
  }

  /** A refusal: a d4 the sheet did not ask for is asked; any other says what it is, in the server's terms. */
  private failed(err: unknown): void {
    const connectErr = ConnectError.from(err, Code.Unavailable);
    const more = connectErr.code === Code.InvalidArgument ? missingDice(connectErr.rawMessage) : 0;
    if (more > 0) {
      this.asked.set(more);
      this.typing.set(true);
      this.error.set(
        more === 1
          ? 'Este teste leva mais um d4: role-o e digite o resultado.'
          : `Este teste leva mais ${more} d4: role-os e digite os resultados.`,
      );
      return;
    }
    this.error.set(combatErrorMessage(err, 'rolar o teste de resistência'));
  }

  protected close(): void {
    if (this.canClose()) {
      this.sheet.close();
    }
  }
}
