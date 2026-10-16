import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import type { Observable } from 'rxjs';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  type ContestAttackOption,
  ContestAttackOptionKind,
  ContestKind,
  ContestPurpose,
  ContestSkill,
} from '../../../../../gen/meurpg/play/v1/contest_types_pb';
import { RollAnimator, showOfCheck } from '../../../../shared/roll-overlay/roll-animator';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { type CheckDie, ContestClient } from '../../../../core/combat/contest-client';
import {
  ESCAPE_DC_MAX,
  ESCAPE_DC_MIN,
  byWho,
  escapeDcOf,
} from '../../../../core/combat/contest-master';
import type { ContestState } from '../../../../core/combat/contest-state';
import { isPlayer } from '../../../../core/combat/combat-view';
import { skillLine, whoIs } from '../../../../core/combat/contest-view';
import { ActionKey } from '../../../../core/connect/idempotency';
import { CheckRollForm } from '../check-roll-form/check-roll-form';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../../../shared/sheet/sheet-host';

/** What the page hands the sheet of an NPC's grapple or shove. */
export interface NpcGrappleData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly state: CombatState;
  readonly contests: ContestState;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  /** The NPC whose turn it is, the first to be chosen. */
  readonly initiatorId: string;
  /** The special attacks of that NPC's turn (`GetTurnOptions`), for the modifier of its Athletics when it is known. */
  readonly attacks: readonly ContestAttackOption[];
}

/** The contest the sheet opened. */
export interface NpcGrappleResult {
  readonly contestId: string;
}

export function openNpcGrappleSheet(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: NpcGrappleData,
): Observable<NpcGrappleResult | undefined> {
  return openSheet<NpcGrappleSheet, NpcGrappleData, NpcGrappleResult>(
    dialog,
    bottomSheet,
    NpcGrappleSheet,
    { data, ariaLabel: 'Agarrar ou empurrar por um NPC', labelledBy: 'npc-grapple-t' },
  );
}

/**
 * "Agarrar ou empurrar por um NPC" (W7-X, boards W7-Xa 1 and W7-Xb 4 and 5, the master's side): the master starts the grapple or the
 * shove of a creature (`StartContest`). Two ways for a grapple. "Disputa": the master rolls Força (Atletismo) for the creature
 * (in the app or the typed d20) and the target answers; the shove is always this way. "CD de escape": the creature's attack grapples
 * with a fixed escape DC (the SRD's "escape DC 16"): nobody rolls, the target is Agarrado at once, and the DC is the master's alone
 * (RN-20): the target escapes with Atletismo or Acrobacia against it. The attack (or the action) is spent by the server in the same
 * transaction; the idempotency key is made once for each request.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-npc-grapple-sheet',
  imports: [CheckRollForm, MatButtonModule, MatIconModule, SheetFrame],
  styleUrl: './master-card.scss',
  template: `
    <app-sheet-frame
      title="Agarrar ou empurrar por um NPC"
      subtitle="Disputa"
      titleId="npc-grapple-t"
      [phone]="inSheet"
      (closed)="close()"
    >
      @if (error()) {
        <div class="mr-notice mr-notice--danger" role="alert">
          <mat-icon aria-hidden="true">error</mat-icon>
          <p>{{ error() }}</p>
        </div>
      }
      <div class="field">
        <label for="npc-grapple-who">Quem ataca</label>
        <select id="npc-grapple-who" [value]="initiatorId()" (change)="pickInitiator($event)">
          @for (c of initiators(); track c.id) {
            <option [value]="c.id" [selected]="c.id === initiatorId()">{{ c.label }}</option>
          }
        </select>
      </div>
      <fieldset class="choices">
        <legend class="choices__cap">O que faz</legend>
        <label class="choice" [class.choice--on]="purpose() === Grapple">
          <input
            type="radio"
            class="mr-visually-hidden"
            name="npc-grapple-purpose"
            [checked]="purpose() === Grapple"
            (change)="purpose.set(Grapple)"
          />
          <span class="choice__name">Agarrar</span>
        </label>
        <label class="choice" [class.choice--on]="purpose() === Shove">
          <input
            type="radio"
            class="mr-visually-hidden"
            name="npc-grapple-purpose"
            [checked]="purpose() === Shove"
            (change)="pickShove()"
          />
          <span class="choice__name">Empurrar</span>
        </label>
      </fieldset>
      <div class="field">
        <label for="npc-grapple-target">Alvo</label>
        <select id="npc-grapple-target" [value]="targetId()" (change)="pickTarget($event)">
          @for (c of targets(); track c.id) {
            <option [value]="c.id" [selected]="c.id === targetId()">{{ c.label }}</option>
          }
        </select>
      </div>

      @if (purpose() === Grapple) {
        <fieldset class="choices">
          <legend class="choices__cap">Como o agarrão se decide</legend>
          <label class="choice" [class.choice--on]="!fixedDc()">
            <input
              type="radio"
              class="mr-visually-hidden"
              name="npc-grapple-way"
              [checked]="!fixedDc()"
              (change)="fixedDc.set(false)"
            />
            <span class="choice__text">
              <span class="choice__name">Disputa</span>
              <span class="choice__sub">Você rola Atletismo pela criatura; o alvo escolhe e rola.</span>
            </span>
          </label>
          <label class="choice" [class.choice--on]="fixedDc()">
            <input
              type="radio"
              class="mr-visually-hidden"
              name="npc-grapple-way"
              [checked]="fixedDc()"
              (change)="fixedDc.set(true)"
            />
            <span class="choice__text">
              <span class="choice__name">CD de escape</span>
              <span class="choice__sub">O agarrão vem de um ataque: ninguém rola.</span>
            </span>
          </label>
        </fieldset>
      }

      @if (fixedDc() && purpose() === Grapple) {
        <div class="field">
          <label for="npc-grapple-dc">CD de escape ({{ dcMin }} a {{ dcMax }})</label>
          <input
            id="npc-grapple-dc"
            type="text"
            inputmode="numeric"
            autocomplete="off"
            [value]="dcText()"
            (input)="typeDc($event)"
          />
          <span class="small">
            O alvo fica Agarrado na hora. Para escapar, faz um teste de Atletismo ou Acrobacia contra esta CD. Só você a vê.
          </span>
        </div>
        <button
          mat-flat-button
          type="button"
          class="btn"
          [disabled]="busy() || dc() === null || !targetId()"
          (click)="start()"
        >
          Agarrar
        </button>
      } @else {
        <p class="card__text">Teste de <b>{{ skill }}</b> {{ byInitiator() }}.</p>
        <app-check-roll-form
          [modifier]="modifier()"
          [checkName]="skill"
          [appLabel]="'Rolar ' + byInitiator()"
          [diceMode]="data.diceMode"
          [preference]="data.preference"
          [busy]="busy() || !targetId()"
          (roll)="start($event)"
        />
      }
    </app-sheet-frame>
  `,
})
export class NpcGrappleSheet {
  private readonly api = inject(ContestClient);
  private readonly animator = inject(RollAnimator);
  private readonly sheet = injectSheet<NpcGrappleData, NpcGrappleResult>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  private readonly keys = new ActionKey();
  private readonly frame = viewChild(SheetFrame);

  protected readonly Grapple = ContestPurpose.GRAPPLE;
  protected readonly Shove = ContestPurpose.SHOVE;
  protected readonly dcMin = ESCAPE_DC_MIN;
  protected readonly dcMax = ESCAPE_DC_MAX;
  protected readonly skill = skillLine(ContestSkill.ATHLETICS);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly purpose = signal<ContestPurpose>(ContestPurpose.GRAPPLE);
  protected readonly fixedDc = signal(false);
  protected readonly dcText = signal('');
  private readonly pickedInitiator = signal('');
  private readonly pickedTarget = signal('');

  private readonly combatants = computed(() => this.data.state.encounter()?.combatants ?? []);
  protected readonly initiators = computed(() =>
    this.combatants().filter((c) => !isPlayer(c) && !c.defeated),
  );
  protected readonly initiatorId = computed(() => {
    const picked = this.pickedInitiator() || this.data.initiatorId;
    return this.initiators().some((c) => c.id === picked)
      ? picked
      : (this.initiators()[0]?.id ?? '');
  });
  protected readonly targets = computed(() =>
    this.combatants().filter((c) => c.id !== this.initiatorId() && !c.defeated),
  );
  protected readonly targetId = computed(() => {
    const picked = this.pickedTarget();
    return this.targets().some((c) => c.id === picked) ? picked : (this.targets()[0]?.id ?? '');
  });
  protected readonly dc = computed(() => escapeDcOf(this.dcText()));
  /** The modifier of the creature's Athletics, when the turn options of this very creature say it. */
  protected readonly modifier = computed(() => {
    const kind =
      this.purpose() === ContestPurpose.SHOVE
        ? ContestAttackOptionKind.SHOVE
        : ContestAttackOptionKind.GRAPPLE;
    const option = this.data.attacks.find((o) => o.kind === kind)?.rollOption;
    return this.initiatorId() === this.data.initiatorId && option?.known ? option.modifier : 0;
  });
  protected readonly byInitiator = computed(() => {
    return byWho(whoIs(this.combatants().find((c) => c.id === this.initiatorId())));
  });

  constructor() {
    effect(() => this.sheet.lock(this.busy()));
    effect(() => {
      if (this.error()) {
        this.frame()?.scrollToTop();
      }
    });
  }

  protected pickInitiator(event: Event): void {
    this.pickedInitiator.set((event.target as HTMLSelectElement).value);
  }

  protected pickTarget(event: Event): void {
    this.pickedTarget.set((event.target as HTMLSelectElement).value);
  }

  protected pickShove(): void {
    this.purpose.set(ContestPurpose.SHOVE);
    this.fixedDc.set(false);
  }

  protected typeDc(event: Event): void {
    this.dcText.set((event.target as HTMLInputElement).value);
    this.error.set('');
  }

  /** The grapple with the fixed DC (no `die`), or the contest with the master's roll. */
  protected async start(die?: CheckDie): Promise<void> {
    const dc = this.fixedDc() && this.purpose() === ContestPurpose.GRAPPLE ? this.dc() : null;
    if (this.busy() || !this.targetId() || (die === undefined && dc === null)) {
      return;
    }
    const request = {
      campaignId: this.data.campaignId,
      encounterId: this.data.encounterId,
      initiatorId: this.initiatorId(),
      targetId: this.targetId(),
      purpose: this.purpose(),
      kind: dc === null ? ContestKind.CONTEST : ContestKind.ESCAPE_DC,
      skill: ContestSkill.ATHLETICS,
      ...(dc === null && die ? { die } : {}),
      ...(dc === null ? {} : { escapeDc: dc }),
    };
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.start(request, this.keys.keyFor(request));
      this.data.state.apply(res.encounter);
      this.data.contests.applyContest(res.contest);
      // The master's d20 for the creature, when the app rolled it (a fixed escape DC rolls nothing).
      const show = showOfCheck('Teste de Atletismo', res.contest.initiatorRoll, {
        withTotal: true,
      });
      if (show) {
        this.animator.play(show);
      }
      this.sheet.close({ contestId: res.contest.id });
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'começar a disputa'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    if (!this.busy()) {
      this.sheet.close();
    }
  }
}
