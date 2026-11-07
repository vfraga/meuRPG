import { computed, signal } from '@angular/core';
import type {
  Encounter,
  GetTurnOptionsResponse,
  OpportunityOffer,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import type { CombatCalls } from '../../../core/combat/combat-calls';
import type { CombatClient } from '../../../core/combat/combat-client';
import { newKey } from '../../../core/connect/idempotency';
import { currentCombatant } from '../../../core/combat/combat-view';
import { type ReactorAttack, reactorAttacks } from '../../../core/combat/opportunity';
import { reactorRows } from '../../../core/combat/theatre';
import { metersText } from '../../../core/units';
import type { PickMark } from '../../../shared/combat-map/combat-map';
import type { CombatantInfo } from './combat-info';
import { type MasterAnswer } from './opportunity/opportunity-card';

/** What `CombatOffers` reads from the screen it serves. */
export interface CombatOffersContext {
  readonly api: Pick<CombatClient, 'moves'>;
  readonly calls: Pick<CombatCalls, 'run' | 'error'>;
  readonly campaignId: () => string;
  readonly encounter: () => Encounter | null;
  /** The combat is played without a map: no distance, no rings. */
  readonly theatre: () => boolean;
  /** The mover's own turn options (the distances of its attacks' targets). */
  readonly options: () => GetTurnOptionsResponse | null;
  /** The offers the caller answers. */
  readonly toAnswer: () => readonly OpportunityOffer[];
  readonly info: () => ReadonlyMap<string, CombatantInfo>;
  /** The master chose to attack for an NPC reactor: opens the attack sheet (`options` are the reactor's own, if read). */
  readonly attack: (
    offer: OpportunityOffer,
    key: string,
    options: GetTurnOptionsResponse | undefined,
  ) => void;
}

/**
 * The master's side of the opportunity attacks (E9-13): the form that offers one, on a map or off it, his answers for
 * an NPC reactor, the reactor's own attacks with their numbers, and the offers that were already opened for the player.
 */
export class CombatOffers {
  /** The master's "Oferecer ataque de oportunidade" form is open. */
  readonly offering = signal(false);
  readonly error = signal('');
  /** Who the mover can have left the reach of (the other side, standing): the form's rows. */
  readonly rows = computed(() => {
    const e = this.ctx.encounter();
    return e
      ? reactorRows(
          e,
          (c) =>
            this.ctx.info().get(c.characterId)?.classSummary ??
            this.ctx.info().get(c.characterId)?.kindLabel ??
            '',
        )
      : [];
  });
  /** The candidate the master picked (the panel's choice, or a tap on its ring on the map); `''` until the panel preselects one. */
  readonly picked = signal('');
  /** Counts the times the master chose the offer: the panel takes the focus at each one. */
  readonly focusAsk = signal(0);
  /** On a map: how far each candidate is now, as the mover's own attacks measure it ("agora a 3 m"). Absent for a theatre or an unknown distance. */
  readonly hints = computed<ReadonlyMap<string, string>>(() => {
    const near = new Map<string, number>();
    if (this.ctx.theatre()) {
      return new Map();
    }
    for (const a of this.ctx.options()?.attackTargets ?? []) {
      for (const t of a.targets) {
        if (t.distanceFt !== undefined) {
          near.set(t.combatantId, Math.min(near.get(t.combatantId) ?? t.distanceFt, t.distanceFt));
        }
      }
    }
    return new Map([...near].map(([id, ft]) => [id, `agora a ${metersText(ft)}`]));
  });
  /** The candidates ringed on the map while the panel is open: whoever can still receive the offer. Null in a theatre or with the panel closed. */
  readonly marks = computed<readonly PickMark[] | null>(() => {
    if (!this.offering() || this.ctx.theatre()) {
      return null;
    }
    const open = this.rows().filter((r) => !r.spent && !r.offered);
    if (!open.length) {
      return null;
    }
    return open.map((r) => ({
      id: r.id,
      label: r.label,
      reason: '',
      note: '',
      count: this.picked() === r.id ? 1 : 0,
      mode: 'offer' as const,
    }));
  });
  /** Why the offer cannot be made now (nobody on the other side can react), or `''`. */
  readonly why = computed(() => {
    const rows = this.rows();
    return rows.every((r) => r.spent || r.offered) ? 'Ninguém pode reagir agora.' : '';
  });
  /** The NPC reactors' attacks with their numbers, by offer (read from the reactor's own options). */
  readonly reactorOptions = signal<ReadonlyMap<string, GetTurnOptionsResponse>>(new Map());
  readonly attacks = computed<ReadonlyMap<string, readonly ReactorAttack[]>>(
    () =>
      new Map(
        this.ctx.toAnswer().map((o) => [
          o.id,
          reactorAttacks(
            o,
            (this.reactorOptions().get(o.id)?.options?.attacks ?? []).flatMap((a) =>
              a.attack ? [a.attack] : [],
            ),
          ),
        ]),
      ),
  );
  /** The offers whose prompt the player's screen has opened already. */
  readonly handled = new Set<string>();
  readonly reading = new Set<string>();
  /** The offers whose reactor's attacks could not be read: the master's card offers "Tentar de novo". */
  readonly failed = signal<ReadonlySet<string>>(new Set());
  /** One idempotency key per form and reactor: a repeated tap never offers twice, a new form is a new offer. */
  private keys = new Map<string, string>();

  constructor(private readonly ctx: CombatOffersContext) {}

  /** Reads an NPC reactor's own options (its attacks with their numbers) for the master's prompt. */
  async loadReactor(offer: OpportunityOffer): Promise<void> {
    const e = this.ctx.encounter();
    if (!e) {
      return;
    }
    this.reading.add(offer.id);
    this.failed.update((s) => {
      const next = new Set(s);
      next.delete(offer.id);
      return next;
    });
    try {
      const res = await this.ctx.api.moves.turnOptions(
        this.ctx.campaignId(),
        e.id,
        offer.reactorId,
      );
      this.reactorOptions.update((m) => new Map(m).set(offer.id, res));
    } catch {
      this.failed.update((s) => new Set(s).add(offer.id));
    } finally {
      this.reading.delete(offer.id);
    }
  }

  /** The master answers for an NPC: "Não atacar", or "Atacar com <arma>". */
  answer(a: MasterAnswer): Promise<boolean> | void {
    if (!a.attack) {
      return this.ctx.calls.run((e) =>
        this.ctx.api.moves.declineOpportunity(this.ctx.campaignId(), e.id, a.offer.id),
      );
    }
    this.ctx.attack(a.offer, a.attack.key, this.reactorOptions().get(a.offer.id));
  }

  /** "Seguir sem esperar": the master passes over an offer the player does not answer. */
  skip(offer: OpportunityOffer): Promise<boolean> {
    return this.ctx.calls.run((e) =>
      this.ctx.api.moves.skipOpportunity(this.ctx.campaignId(), e.id, offer.id),
    );
  }

  /** "Retirar a oferta": the master takes back an offer nobody answered; the reactor keeps its reaction. */
  withdraw(offer: OpportunityOffer): Promise<boolean> {
    return this.ctx.calls.run((e) =>
      this.ctx.api.moves.withdrawOpportunity(this.ctx.campaignId(), e.id, offer.id),
    );
  }

  /** "Oferecer a Toren": the master says the one on turn left that combatant's reach (on a map or off it). */
  async make(reactorId: string): Promise<void> {
    const e = this.ctx.encounter();
    const mover = e ? currentCombatant(e) : null;
    if (!e || !mover) {
      return;
    }
    this.error.set('');
    let key = this.keys.get(reactorId);
    if (!key) {
      key = newKey();
      this.keys.set(reactorId, key);
    }
    const ok = await this.ctx.calls.run((current) =>
      this.ctx.api.moves.offerOpportunity(
        this.ctx.campaignId(),
        current.id,
        mover.id,
        reactorId,
        key,
      ),
    );
    if (ok) {
      this.offering.set(false);
    } else if (this.ctx.calls.error()) {
      // The refusal stays in the form, where the master is looking (the page's notice has it too).
      this.error.set(this.ctx.calls.error());
      this.ctx.calls.error.set('');
      // A refused offer is not a retry: the next try is a new one.
      this.keys.delete(reactorId);
    }
  }

  /** "Oferecer ataque de oportunidade" chosen in the card's menu: opens the form, or, when it is open already, keeps it and asks it to take the focus. */
  open(): void {
    if (!this.offering()) {
      this.toggle();
    }
    this.focusAsk.update((n) => n + 1);
  }

  /** A tap on a ringed candidate of the map picks it in the panel. */
  tap(id: string): void {
    if (this.marks()?.some((m) => m.id === id)) {
      this.picked.set(id);
    }
  }

  /** "Oferecer ataque de oportunidade": opens the form under its button (a new form, new keys), or closes it. */
  toggle(): void {
    if (!this.offering()) {
      this.keys = new Map();
      this.error.set('');
      this.picked.set('');
    }
    this.offering.set(!this.offering());
  }
}
