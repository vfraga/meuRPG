import {
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
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import {
  type CharacterCreature,
  CreatureSource,
  type SummonSpellOptions,
} from '../../../../gen/meurpg/characters/v1/characters_pb';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { focusWithRing } from '../../../core/creatures/focus-ring';
import { creaturesHidden } from '../../../core/creatures/creature-errors';
import { castNotice, creaturesText } from '../../../core/creatures/summon-labels';
import { tight } from '../../../core/format/text';
import { OpenSessions } from '../../../shell/live-notice/open-sessions';
import { openSheet } from '../../live-session/combat/sheet-host';
import { openWildShape } from '../../../shared/wild-shape/wild-shape-sheet';
import { combatErrorMessage } from '../../../core/combat/combat-errors';
import { ActionKey } from '../../../core/connect/idempotency';
import { CreatureCard } from './creature-card';
import type { EditMode } from './creature-edit';
import { SummonSheet, type SummonSheetData, type SummonSheetResult } from './summon-sheet';

type ListState = 'loading' | 'hidden' | 'ready' | 'failed';

/**
 * The sheet's "Criaturas" panel (E9-10, MR-037): the creatures that belong to the character, one card
 * each (see `CreatureCard`), and what the character can conjure. It sits right after "Combate" and
 * "Magias", only for who can have a creature: a character that can cast Convocar Familiar, Animar os
 * Mortos or Conjurar Animais (the server says, `GetSummonOptions`), a druid, or any character that
 * already has one (no empty panel for the rest). RN-20: the list is the owner's player's and the
 * master's; for anyone else the server answers `not_found` and the panel does not exist.
 *
 * Empty: an invitation ("Nenhuma criatura ainda. Use Convocar Familiar ou peça ao mestre para dar
 * uma.") and one outlined button per spell the character can cast, with what it costs under it. A
 * cast needs a session: outside one the button is dashed and says why. The master does not cast from
 * here (the master gives, in the campaign's character list).
 *
 * A live region confirms what arrived ("Nanquim chegou. Convocar Familiar, ritual de 1 hora. Nenhum
 * espaço de magia foi gasto.", "O mestre deu uma criatura a você: Mastim."); it goes away when the next
 * action starts. The list is read again on the stream's `creatures_changed` (`reload`, a counter the
 * page bumps); an answer that arrives after a newer read started is thrown away. When the read fails,
 * the panel says so and offers to try again: it never shows "Nenhuma criatura" for a list it could not read.
 */
@Component({
  selector: 'app-creatures-panel',
  imports: [CreatureCard, MatButtonModule, MatIconModule],
  templateUrl: './creatures-panel.html',
  styleUrl: './creatures-panel.scss',
})
export class CreaturesPanel {
  private readonly client = inject(CreaturesClient);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly openSessions = inject(OpenSessions);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  readonly characterId = input.required<string>();
  readonly characterName = input.required<string>();
  readonly isMaster = input(false);
  /** A druid with Wild Shape: the panel exists before a first creature (display only). */
  readonly wildShape = input(false);
  /** Bumped by the page when the stream says the creatures changed. */
  readonly reload = input(0);
  /** Bumped by the page when the character's vitals or the combat changed: the Wild Shape form is read again (it ends
   * by damage, by sleep or by the master's hand, none of which is a creature change). */
  readonly formReload = input(0);

  protected readonly state = signal<ListState>('loading');
  protected readonly creatures = signal<readonly CharacterCreature[]>([]);
  protected readonly spells = signal<readonly SummonSpellOptions[]>([]);
  /** What the live region says now. */
  protected readonly notice = signal('');
  private known = new Set<string>();
  private first = true;
  private castInFlight = false;
  /** What a gift from the master said while a cast sheet was open: it is told once the sheet is closed. */
  private heldGift = '';
  /** A read of a list that was already shown failed: the list stays, and the panel says it may be out of date. */
  protected readonly refreshFailed = signal(false);
  private seq = 0;
  private wildSeq = 0;

  protected readonly live = computed(() =>
    this.openSessions.sessions().some((s) => s.campaignId === this.campaignId()),
  );
  protected readonly casts = computed(() => (this.isMaster() ? [] : this.spells()));
  protected readonly visible = computed(
    () =>
      this.state() === 'failed' ||
      (this.state() === 'ready' &&
        (this.creatures().length > 0 ||
          (!this.isMaster() && (this.casts().length > 0 || this.wildShape())))),
  );
  protected readonly countText = computed(() =>
    this.creatures().length === 0 ? '' : creaturesText(this.creatures().length),
  );
  protected readonly emptyText = computed(() => {
    const names = this.casts().map((c) => c.namePt);
    if (names.length === 0) {
      return 'Nenhuma criatura ainda. Peça ao mestre para dar uma.';
    }
    return `Nenhuma criatura ainda. Use ${names.join(' ou ')} ou peça ao mestre para dar uma.`;
  });

  /** Wild Shape on the sheet (E9-11): the beast the druid is now (`''` in its own shape), the uses and a refusal in words. */
  protected readonly form = signal('');
  protected readonly uses = signal<{
    readonly left: number;
    readonly total: number;
    readonly recharge: string;
  } | null>(null);
  protected readonly wildError = signal('');
  /** "Voltar à forma normal" is on its way. */
  protected readonly leaving = signal(false);
  private readonly leaveKey = new ActionKey();
  /** "restam 2 de 2 usos · volta no descanso curto ou longo", or what stops it outside a session. */
  protected readonly wildLine = computed(() => {
    const u = this.uses();
    if (!this.live()) {
      return 'Só durante um combate ou uma sessão.';
    }
    if (!u) {
      return '';
    }
    const back =
      u.recharge === 'long_rest' ? 'volta no descanso longo' : 'volta no descanso curto ou longo';
    return tight(
      u.left === 0
        ? `Sem usos · ${back}`
        : `Restam ${u.left} de ${u.total} ${u.total === 1 ? 'uso' : 'usos'} · ${back}`,
    );
  });

  private async loadWild(): Promise<void> {
    if (!this.wildShape() || this.isMaster() || !this.live()) {
      return;
    }
    const seq = ++this.wildSeq;
    try {
      const v = await this.client.vitalsOf(this.campaignId(), this.characterId());
      if (seq !== this.wildSeq) {
        return;
      }
      const r = v?.resources.find((x) => x.key === 'wild_shape');
      this.uses.set(
        r
          ? {
              left: r.total - r.used,
              total: r.total,
              recharge: r.recharge === 2 ? 'long_rest' : 'short_rest',
            }
          : null,
      );
      this.form.set(v?.wildShape?.beastNamePt ?? '');
    } catch {
      if (seq === this.wildSeq) {
        this.uses.set(null);
      }
    }
  }

  protected transform(): void {
    if (!this.live()) {
      return;
    }
    this.wildError.set('');
    openWildShape(this.dialog, this.bottomSheet, {
      campaignId: this.campaignId(),
      characterId: this.characterId(),
      inCombat: false,
      uses: this.uses() ? { left: this.uses()!.left, total: this.uses()!.total } : null,
    }).subscribe((r) => {
      if (r) {
        void this.loadWild();
        this.notice.set(`Você virou ${r.beastNamePt}. O mestre encerra a forma.`);
      }
    });
  }

  protected async leave(): Promise<void> {
    // One request at a time: the server refuses a second one (the form has already ended) as a new action.
    if (this.leaving()) {
      return;
    }
    this.leaving.set(true);
    this.wildError.set('');
    try {
      await this.client.leaveWildShape(
        this.campaignId(),
        this.characterId(),
        this.leaveKey.keyFor([this.campaignId(), this.characterId()]),
      );
      this.leaveKey.renew();
      this.notice.set('Você voltou à forma normal.');
      await this.loadWild();
    } catch (err) {
      this.wildError.set(combatErrorMessage(err, 'voltar à forma normal'));
    } finally {
      this.leaving.set(false);
    }
  }

  constructor() {
    effect(() => {
      const ticks = this.formReload();
      untracked(() => {
        if (ticks > 0) {
          void this.loadWild();
        }
      });
    });
    effect(() => {
      this.reload();
      this.live();
      untracked(() => void this.loadWild());
      const campaignId = this.campaignId();
      const characterId = this.characterId();
      untracked(() => void this.load(campaignId, characterId));
    });
  }

  private async load(campaignId: string, characterId: string): Promise<CharacterCreature[] | null> {
    const seq = ++this.seq;
    // The options are the player's (the master gives, in the campaign's list); a failed read of them only
    // takes the buttons away.
    const [list, options] = await Promise.allSettled([
      this.client.list(campaignId, characterId),
      this.isMaster() ? Promise.resolve(null) : this.client.summonOptions(campaignId, characterId),
    ]);
    if (seq !== this.seq) {
      return null; // a newer read started: its answer is the one that counts
    }
    if (list.status === 'rejected') {
      // RN-20: a creature list nobody may read is no panel at all; any other failure says so.
      if (creaturesHidden(list.reason)) {
        this.state.set('hidden');
      } else if (this.state() !== 'ready') {
        this.state.set('failed');
      } else {
        this.refreshFailed.set(true);
      }
      return null;
    }
    this.refreshFailed.set(false);
    this.spells.set(
      options.status === 'fulfilled' && options.value ? [...options.value.spells] : [],
    );
    this.announceArrivals(list.value);
    this.creatures.set(list.value);
    this.state.set('ready');
    return [...list.value];
  }

  /** A creature that was not on the list before and did not come from a cast made here: the master gave it. */
  private announceArrivals(list: readonly CharacterCreature[]): void {
    const fresh = list.filter((c) => !this.known.has(c.id));
    this.known = new Set(list.map((c) => c.id));
    if (this.first) {
      this.first = false;
      return;
    }
    if (fresh.length === 0) {
      return;
    }
    const gift = fresh.find((c) => c.source === CreatureSource.MASTER);
    if (this.castInFlight) {
      // What arrives from the cast is told by the cast; a gift made meanwhile is told after it.
      if (gift) {
        this.heldGift = `O mestre deu uma criatura a você: ${gift.name}.`;
      }
      return;
    }
    this.notice.set(
      gift
        ? `O mestre deu uma criatura a você: ${gift.name}.`
        : `${fresh.map((c) => c.name).join(', ')} chegou.`,
    );
  }

  protected costText(spell: SummonSpellOptions): string {
    const where = 'Só durante uma sessão, fora de combate.';
    if (spell.canRitual) {
      return tight(`Ritual de ${spell.castingTimePt}: não gasta espaço de magia. ${where}`);
    }
    return tight(`Gasta um espaço de ${spell.level}º nível ou maior. ${where}`);
  }

  protected cast(spell: SummonSpellOptions): void {
    if (!this.live()) {
      return;
    }
    this.notice.set('');
    this.heldGift = '';
    this.castInFlight = true;
    openSheet<SummonSheet, SummonSheetData, SummonSheetResult>(
      this.dialog,
      this.bottomSheet,
      SummonSheet,
      {
        data: {
          campaignId: this.campaignId(),
          characterId: this.characterId(),
          spellKey: spell.spellKey,
        },
        ariaLabel: spell.namePt,
        labelledBy: 'summon-t',
        width: '520px',
      },
    ).subscribe((result) => {
      if (!result) {
        this.castInFlight = false;
        this.notice.set(this.heldGift);
        this.heldGift = '';
        return;
      }
      void this.load(this.campaignId(), this.characterId()).then(() => {
        this.castInFlight = false;
        this.notice.set([this.confirmation(result), this.heldGift].filter((t) => t).join(' '));
        this.heldGift = '';
        this.focusAfterRender('.js-cast');
      });
    });
  }

  private confirmation(r: SummonSheetResult): string {
    const arrived =
      r.names.length === 1
        ? `${r.names[0]} chegou.`
        : r.count === 1
          ? 'A criatura chegou.'
          : `${creaturesText(r.count)} chegaram.`;
    return castNotice(arrived, r.spellName, r.ritual, r.castingTime, r.dismissed);
  }

  /** A card's rename, dismissal or correction went through: read the list again and put the focus somewhere that exists. */
  protected async changed(id: string, mode: EditMode): Promise<void> {
    const index = this.creatures().findIndex((c) => c.id === id);
    const list = await this.load(this.campaignId(), this.characterId());
    if (!list) {
      return;
    }
    if (mode === 'dismiss') {
      // The card is gone: the next one (the same place in the list), or the panel's title.
      this.focusAfterRender(
        this.creatures().length > 0 ? 'app-creature-card .name' : '.js-title',
        index,
      );
    } else {
      // The card stays: its name is where the person was.
      this.focusAfterRender(`#creature-${id}`);
    }
  }

  private focusAfterRender(selector: string, nth = 0): void {
    afterNextRender(
      () => {
        const all = this.host.nativeElement.querySelectorAll<HTMLElement>(selector);
        // A button shows its ring; a title (the last place left) takes the focus without one.
        const target = all[Math.min(nth, all.length - 1)];
        if (target?.tagName === 'BUTTON') {
          focusWithRing(target);
        } else {
          target?.focus();
        }
      },
      { injector: this.injector },
    );
  }

  protected retry(): void {
    if (this.state() === 'failed') {
      this.state.set('loading');
    }
    void this.load(this.campaignId(), this.characterId());
  }
}
