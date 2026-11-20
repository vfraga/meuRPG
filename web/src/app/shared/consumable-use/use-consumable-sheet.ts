import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  signal,
  viewChild,
  viewChildren,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import { parseSum } from '../../core/combat/combat-dice';
import {
  type ConsumableKind,
  type ConsumableVm,
  signedText,
} from '../../core/characters/consumable-vm';
import {
  canGive,
  type ConsumableTargetVm,
  effectIcon,
  effectLine,
  kindWords,
  subtitleOf,
  type TurnBlock,
  useCost,
} from '../../core/characters/consumables';
import { ActionKey } from '../../core/connect/idempotency';
import { focusWithRing } from '../../core/creatures/focus-ring';
import type { ConsumableRoll } from '../../core/play/charges-client';
import {
  type ConsumableRefusal,
  goneSheetRefusal,
  turnBlockParts,
  unreadSheetRefusal,
} from '../../core/play/charges-errors';
import { EconomyMark } from '../economy-shapes/economy-shapes';
import { SheetFrame } from '../sheet/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../sheet/sheet-host';

const NBSP = '\u00a0';

/** The sentence under the effect line of an item with no dice: what a lasting effect is called and how long it lasts, as the server says it. */
function explainOf(effectPt: string, consumable: ConsumableVm): string {
  if (effectPt === '') {
    return 'O app só gasta o item e mostra o texto dele: o mestre e você seguem o texto.';
  }
  const lasting = consumable.lasting;
  if (!lasting) {
    return '';
  }
  const length = lasting.length ? ` ${lasting.length}` : '';
  return `Quem bebe ganha o efeito “${lasting.title}”${length}: aparece em “Seus efeitos” e acaba sozinho.`;
}

/** What the page holds once a refusal made it read the character again: the unit the sheet is open on, and its count. */
export interface Aftermath {
  /** The units of the item the page holds now. */
  readonly left?: number;
  /** The unit the sheet was open on is gone and another of the same item takes its place (the next call has a new key). */
  readonly rebound?: boolean;
  /** No unit is left: the sheet closes, and the page shows the notice. */
  readonly close?: boolean;
  /** The unit is gone and the page could not read the sheet again: the sheet offers nothing until a read lands. */
  readonly unread?: boolean;
}

/** Who may receive the consumable, or why the list could not be read. */
export interface TargetsOutcome extends Aftermath {
  readonly targets: readonly ConsumableTargetVm[];
  /** The character is in a running combat (the server's own test); `null` when the list could not be read. */
  readonly inCombat: boolean | null;
  /** A refusal in words; `null` when the list was read. */
  readonly error: ConsumableRefusal | null;
}

/** What a use answered: `refusal` is `null` when it was used; the rest is what the page now holds. */
export interface UseAnswer extends Aftermath {
  readonly refusal: ConsumableRefusal | null;
}

/** What a tap on the item's own button or "Dar a …" asks: the receiver (`null` for the user) and how the healing dice come. */
export interface UseConsumableRequest {
  readonly target: ConsumableTargetVm | null;
  readonly roll: ConsumableRoll;
}

export interface UseConsumableData {
  readonly itemName: string;
  readonly kind: ConsumableKind;
  /** What the server says the use is and costs (`SheetMagicItem.consumable`). */
  readonly consumable: ConsumableVm;
  /** What "Usar" does, as the server words it ("Cura 2d4 + 2"); empty for an item with no automatic effect. */
  readonly effectPt: string;
  /** The units of the item the sheet holds: "Restam 2". */
  readonly left: number;
  /** The master acts for this character: the title reads "Usar por Tor". `null` for the player's own use. */
  readonly actingFor: string | null;
  /** Whether a combat is running, when the page knows (the combat's "Itens" does); `null` on the sheet, which learns it from the targets read. */
  readonly inCombat: boolean | null;
  /** What the page knows keeps the use from the Action (it is not the turn, or the Action is spent): both buttons are dashed and the sheet says why, with no call to the server. Only for an item that costs the action. */
  readonly blockedBy: TurnBlock | null;
  /** The app may roll the dice (the table did not force real dice). */
  readonly canApp: boolean;
  /** Typing the faces of real dice is allowed (the table did not force the app). */
  readonly canType: boolean;
  /** Where the player rolls once the table's mode and their choice are put together (RN-18). */
  readonly preferApp: boolean;
  /** Reads who the item may be given to; never rejects. */
  readonly loadTargets: () => Promise<TargetsOutcome>;
  /** Reads the sheet again after an `unread` answer; never rejects. */
  readonly reread: () => Promise<Aftermath>;
  /** Uses the item under `key`; answers the refusal in words, or none when it was used. */
  readonly use: (request: UseConsumableRequest, key: string) => Promise<UseAnswer>;
}

/** Opens "Usar · <item>": a bottom sheet on a phone, a dialog from a tablet up. Answers `true` when the unit was used. */
export function openUseConsumable(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: UseConsumableData,
) {
  return openSheet<UseConsumableSheet, UseConsumableData, boolean>(
    dialog,
    bottomSheet,
    UseConsumableSheet,
    {
      data,
      ariaLabel: data.actingFor ? `Usar por ${data.actingFor}` : `Usar ${data.itemName}`,
      labelledBy: 'use-consumable-t',
      width: '500px',
      // The page puts the focus back on the item's row (with the focus ring) once it has drawn it again.
      restoreFocus: false,
    },
  );
}

/**
 * "Usar · Poção de cura": drink it ("Beber"; an oil or any other item is "Usar") or give it ("Dar a…", the receivers the server lists as radio rows). The dice of
 * a healing potion follow the table's rule (RN-18): the app rolls them, or the player types one face for each die ("Digitar
 * os dados"). The call has one idempotency key for the request it carries: the same use again after a failure is a retry and
 * never spends twice. Works on a locked sheet: the sheet only reads and sends.
 */
@Component({
  selector: 'app-use-consumable-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [EconomyMark, MatButtonModule, MatIconModule, SheetFrame],
  templateUrl: './use-consumable-sheet.html',
  styleUrl: './use-consumable-sheet.scss',
})
export class UseConsumableSheet {
  private readonly sheet = injectSheet<UseConsumableData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  private readonly injector = inject(Injector);
  private readonly radios = viewChildren<ElementRef<HTMLInputElement>>('radio');
  private readonly giveButton = viewChild('give', { read: ElementRef<HTMLButtonElement> });
  private readonly faceFields = viewChildren<ElementRef<HTMLInputElement>>('face');
  private readonly typeSwitch = viewChild('typeSwitch', { read: ElementRef<HTMLButtonElement> });

  protected readonly title = this.data.actingFor
    ? `Usar por ${this.data.actingFor}`
    : `Usar · ${this.data.itemName}`;
  private readonly words = kindWords(this.data.kind);
  /** The server says whether the use costs the action in a running combat: every potion but an oil does, and nothing else. */
  private readonly costs = this.data.consumable.costsAction;
  /** The units of the item the page holds: a refusal that made the page read the character again changes it. */
  protected readonly left = signal(this.data.left);
  /** Whether a combat is running: what the page knew, then what the targets read said (the server's own test). */
  protected readonly inCombat = signal<boolean | null>(this.data.inCombat);
  protected readonly subtitle = computed(() =>
    subtitleOf({
      itemName: this.data.itemName,
      left: this.left(),
      actingFor: this.data.actingFor,
      inCombat: this.inCombat(),
      costsAction: this.costs,
    }),
  );
  protected readonly effectText = effectLine(this.data.effectPt);
  protected readonly effectIconName = effectIcon(this.data.effectPt, this.data.consumable);
  /** The dice of a healing potion, as the server sent them; `null` for every other consumable, which rolls nothing. */
  protected readonly dice = this.data.consumable.heal;
  protected readonly numbers = Array.from({ length: this.dice?.count ?? 0 }, (_, i) => i);
  /** "+ 2" with a no-break space: the sign never stays alone at the end of a line. */
  protected readonly signedModifier = signedText(this.dice?.modifier ?? 0).replace(' ', NBSP);
  protected readonly drinkWord = this.words.use;
  protected readonly itemIcon = this.words.icon;
  /** What the item does in words, for the items whose effect line says it only in short; the lasting effect's title and length are the server's. */
  protected readonly explain = this.dice ? '' : explainOf(this.data.effectPt, this.data.consumable);

  protected readonly phase = signal<'choose' | 'give'>('choose');
  /** `null` while the list is being read. */
  protected readonly targets = signal<readonly ConsumableTargetVm[] | null>(null);
  protected readonly targetsError = signal<ConsumableRefusal | null>(null);
  protected readonly chosen = signal('');
  protected readonly receiver = computed(
    () => this.targets()?.find((t) => t.id === this.chosen()) ?? null,
  );
  protected readonly receiverName = computed(() =>
    this.phase() === 'give' ? (this.receiver()?.name ?? '') : '',
  );
  /** What keeps the use from the Action: the page knew it, or the server just refused for it. Nothing is offered under it. */
  protected readonly blocked = signal<TurnBlock | null>(this.data.blockedBy);
  protected readonly blockedLine = computed(() => {
    const block = this.blocked();
    return block ? turnBlockParts(block, this.data.actingFor) : null;
  });
  /** The server lists only who is within reach for what costs the action in a running combat; any other use lists the party. */
  private readonly byReach = computed(() => this.inCombat() === true && this.costs);
  protected readonly whyId = 'use-consumable-why';
  /** The line of the refusal under the effect: what "Dar a…" points to while the sheet is not read again. */
  protected readonly refusalId = 'use-consumable-refusal';
  protected readonly targetsErrorId = 'use-consumable-targets-error';
  /** A potion and an oil are given: a scroll and the rest have no "Dar a…", so nobody is listed for them. */
  protected readonly gives = canGive(this.data.kind);
  protected readonly canGive = computed(
    () => this.gives && !this.blocked() && (this.targets()?.length ?? 0) > 0,
  );
  /** "Dar a…" is dashed: the turn does not allow it, or the list is read and has nobody (or could not be read). */
  protected readonly giveOff = computed(
    () => !!this.blocked() || this.unread() || (this.targets() !== null && !this.canGive()),
  );
  /** The block icon says there is nobody to give to; a use the turn does not allow, or a sheet not read again, keeps the open hand, like the item's own button. */
  protected readonly giveIcon = computed(() =>
    this.giveOff() && !this.blocked() && !this.unread() ? 'block' : 'volunteer_activism',
  );
  /** What says why "Dar a…" is dashed: the line of the turn or of the empty list, or the error of the read. */
  protected readonly giveWhyId = computed(() => {
    if (!this.giveOff()) {
      return null;
    }
    if (this.unread()) {
      return this.refusalId;
    }
    if (this.reasonText() || this.blockedLine()) {
      return this.whyId;
    }
    return this.targetsError() ? this.targetsErrorId : null;
  });
  protected readonly legend = computed(() =>
    this.byReach() ? 'Dar a quem está ao seu lado' : 'Dar a alguém do grupo',
  );
  /** The verb of the button that applies the item to its user: "Beber" for a potion, "Usar" for the rest. */
  protected readonly selfWord = computed(() => `${this.drinkWord} eu mesmo`);

  /** The player rolls their own dice: the fields open first (the table's mode and the choice of the player, RN-18). */
  protected readonly typing = signal(
    this.data.canType && (!this.data.canApp || !this.data.preferApp),
  );
  protected readonly faces = signal<readonly string[]>(this.numbers.map(() => ''));
  private readonly parsed = computed(() =>
    this.faces().map((text) => parseSum(text, 1, this.dice?.sides ?? 1)),
  );
  protected readonly invalid = computed(() =>
    this.faces().map((text, i) => text.trim() !== '' && this.parsed()[i] === null),
  );
  /** Some die holds a face that is no number of its range: the one message under the row says it. */
  protected readonly anyInvalid = computed(() => this.invalid().some(Boolean));
  protected readonly faceErrorId = 'use-consumable-face-err';
  /** Every die has its face (or the app rolls them). */
  private readonly rolled = computed(
    () => !this.dice || !this.typing() || this.parsed().every((v) => v !== null),
  );

  protected readonly busy = signal(false);
  protected readonly error = signal<ConsumableRefusal | null>(null);
  /** The unit is gone and the page could not read the sheet again: nothing is offered until a read lands. */
  protected readonly unread = signal(false);
  protected readonly canSend = computed(
    () => !this.blocked() && !this.unread() && !this.busy() && this.rolled(),
  );
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  private key = new ActionKey();
  /** The receiver of the last call, for "Tentar de novo". */
  private lastTarget: ConsumableTargetVm | null = null;
  /** What the gone unit's refusal said, for when a later read lands: the unit was used, and the sheet is up to date now. */
  private goneRefusal: ConsumableRefusal | null = null;

  /** Why the first state offers less than two buttons: the turn does not allow it, or the list is read and has nobody. */
  protected readonly reasonText = computed(() => {
    if (this.blocked() || this.unread() || !this.gives || this.targets() === null) {
      return '';
    }
    if (this.canGive() || this.targetsError()) {
      return '';
    }
    return this.byReach()
      ? `Ninguém a 1,5 m de ${this.data.actingFor ?? 'você'} para receber ${this.words.the}.`
      : `Ninguém mais no grupo para receber ${this.words.the}.`;
  });

  /** What the use costs: the mark of the Ação and a bold lead where it spends it, an info icon where it says what it does not. */
  protected readonly cost = computed(() =>
    useCost({
      kind: this.data.kind,
      costsAction: this.costs,
      inCombat: this.inCombat(),
      blocked: !!this.blocked(),
      phase: this.phase(),
      receiverName: this.receiverName(),
      canGive: this.canGive(),
    }),
  );

  constructor() {
    if (this.blocked() || !this.gives) {
      // The turn does not allow the use, or the item is not given: nobody is listed and the server is not asked.
      this.targets.set([]);
      return;
    }
    void this.refreshTargets();
  }

  /** Reads who may receive the item and what the targets say of the combat; `leaving` is a receiver the list must not offer again. */
  protected async refreshTargets(leaving = ''): Promise<void> {
    const out = await this.data.loadTargets();
    this.targets.set(out.targets.filter((t) => t.id !== leaving));
    this.targetsError.set(out.error);
    if (out.inCombat !== null) {
      this.inCombat.set(out.inCombat);
    }
    this.follow(out);
    if (out.unread) {
      // The unit is gone and the sheet was not read: one message, with the way to read it again.
      this.targetsError.set(null);
      this.goneRefusal = goneSheetRefusal(this.data.kind);
      this.error.set(unreadSheetRefusal(this.data.kind));
    }
    const still = this.targets()?.find((t) => t.id === this.chosen());
    this.chosen.set(still?.id ?? this.targets()?.[0]?.id ?? '');
    if (this.phase() === 'give' && (this.targets()?.length ?? 0) === 0) {
      this.phase.set('choose');
    }
  }

  /** What the page holds now of the unit: its count, the unit that takes the place of a gone one, or the sheet closing. */
  private follow(after: Aftermath): void {
    if (after.unread) {
      this.unread.set(true);
    } else if (after.left !== undefined || after.rebound || after.close) {
      this.unread.set(false);
    }
    if (after.left !== undefined) {
      this.left.set(Math.max(1, after.left));
    }
    if (after.rebound) {
      this.key = new ActionKey();
    }
    if (after.close) {
      this.sheet.close(true);
    }
  }

  /** "Voltar": the first choice again, with the focus on "Dar a…". */
  protected back(): void {
    this.phase.set('choose');
    afterNextRender(() => this.giveButton()?.nativeElement.focus(), { injector: this.injector });
  }

  protected openGive(): void {
    this.phase.set('give');
    afterNextRender(
      () =>
        this.radios()
          .find((r) => r.nativeElement.checked)
          ?.nativeElement.focus(),
      {
        injector: this.injector,
      },
    );
  }

  /** "Digitar os dados": the fields open and the focus goes to the first die; the toggle is one control that changes its word and keeps its place. */
  protected startTyping(): void {
    this.typing.set(true);
    afterNextRender(() => this.faceFields()[0]?.nativeElement.focus(), { injector: this.injector });
  }

  /** "Rolar no app": the fields go, and the focus goes back to the toggle, which is still there. */
  protected stopTyping(): void {
    this.typing.set(false);
    afterNextRender(() => focusWithRing(this.typeSwitch()?.nativeElement), {
      injector: this.injector,
    });
  }

  protected onFace(index: number, event: Event): void {
    const text = (event.target as HTMLInputElement).value;
    this.faces.update((all) => all.map((old, i) => (i === index ? text : old)));
  }

  /** "Dar a X": the receiver is chosen; with nobody chosen nothing is sent (a call with no receiver would be the user's own). */
  protected give(): void {
    const receiver = this.receiver();
    if (receiver) {
      void this.send(receiver);
    }
  }

  protected async send(target: ConsumableTargetVm | null): Promise<void> {
    if (!this.canSend()) {
      return;
    }
    this.lastTarget = target;
    const faces = this.parsed().filter((v): v is number => v !== null);
    const request: UseConsumableRequest = {
      target,
      roll: this.dice && this.typing() ? { faces } : { inApp: true },
    };
    this.busy.set(true);
    this.error.set(null);
    try {
      const answer = await this.data.use(
        request,
        this.key.keyFor({ target: target?.id ?? '', roll: request.roll }),
      );
      this.follow(answer);
      const why = answer.refusal;
      if (!why) {
        this.sheet.close(true);
      } else if (answer.unread) {
        this.goneRefusal = why;
        this.error.set(unreadSheetRefusal(this.data.kind));
      } else if (why.block) {
        // The turn or the action: the sheet is blocked and its ban line says why, so the refusal is not said twice.
        this.blocked.set(why.block);
      } else {
        this.error.set(why);
        if (target && (why.cause === 'target-dead' || why.cause === 'target-away')) {
          // The receiver is out of reach or dead: the list is read again and the choice dropped.
          await this.refreshTargets(why.cause === 'target-dead' ? target.id : '');
        }
      }
    } finally {
      this.busy.set(false);
    }
  }

  /** "Tentar de novo" under a call that never reached the server: the same call again, with the same key; under a sheet that was not read, the read. */
  protected retry(): void {
    if (this.unread()) {
      void this.reread();
      return;
    }
    void this.send(this.lastTarget);
  }

  /** Reads the sheet again after a gone unit: once it lands the sheet says what the refusal said, and now it is true. */
  private async reread(): Promise<void> {
    this.busy.set(true);
    try {
      const after = await this.data.reread();
      this.follow(after);
      this.error.set(after.unread ? unreadSheetRefusal(this.data.kind) : this.goneRefusal);
      if (!after.unread && !after.close) {
        await this.refreshTargets();
      }
    } finally {
      this.busy.set(false);
    }
  }

  /** "Tentar de novo" under a list that could not be read. */
  protected retryTargets(): void {
    this.targetsError.set(null);
    void this.refreshTargets();
  }

  protected close(): void {
    if (!this.busy()) {
      this.sheet.close();
    }
  }
}
