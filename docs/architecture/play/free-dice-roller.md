# The free dice roller

The free dice roller ("Rolar dados", MR-052) rolls any dice the person asks for with the server's dice, outside every game rule, and shows the result to the table or only to the master.
`RollFreeDice` and `ListFreeRolls` are `PlayService` calls (`play.proto`); the messages are in `free_dice.proto`.
The code is `free_roll.go` (the roll, with no database) and `free_roll_rpc.go` (the service).

## What a roll is

| Part | Rule |
| --- | --- |
| The die | One of d4, d6, d8, d10, d12, d20 or d100 |
| How many | 1 to 20 dice of that one die |
| The modifier | -20 to +20, added once to the sum |
| Advantage or disadvantage | Only for a single d20: two d20 are rolled, the higher (or lower) counts, and the other is marked `discarded`. On a tie the first counts |
| The total | The sum of the counted dice plus the modifier; it can be zero or negative (`1d4 - 20`) |

The server validates all of it (`parseFreeRoll`): anything else is `invalid_argument`, and nothing is rolled or written.
The dice come from the service's `Roller` (the operating system's random source in production), the same source the other roll calls use, so a test passes faces in order.
**The table's dice mode (RN-18) does not apply.**
The mode is about how a player rolls the game's rolls (attacks, checks, saves, initiative).
This roll has no game consequence, so the app always rolls it, and the log line says "Rolagem livre" to tell it from a game roll.
A table that uses physical dice still gets the app's roll here.

## Who may roll, and who sees the roll

Any active member of the campaign may roll while a session is open: the master or a player, with or without a character (the roll is tied to the person, not to a character).
No open session is `failed_precondition` `NO_OPEN_SESSION`; a stranger and a pending member get `not_found`.

| Roll | Stored | The stream | `ListFreeRolls` |
| --- | --- | --- | --- |
| A player's | Yes | `free_roll_rolled` to everyone, the roller too, with the whole roll | Everyone's `table`; the roller's `mine` |
| The master's, "Para a mesa" | Yes | The same | The same |
| The master's, "Só eu" (`secret`) | Yes, `secret: true` | **Nothing**, not even to the master's other screens | The master's `table` and `mine`; no player's answer holds it or counts it (RN-10) |

Only the master may ask for `secret`; a player who does gets `permission_denied` and nothing is rolled.
The audience is decided by one function (`freeRollEvent.audience`): a secret roll has the zero audience, which reaches nobody, so a forgotten field cannot leak it.
The same rule is applied twice on the read path: the SQL (`ListFreeRollEvents`) keeps a player to rows whose payload says `"secret": false`, and the shaping (`visibleTo`) drops a secret roll again.
Free rolls are not part of the session summary or the highlights, which read only the kinds they name, and the event has no `encounter_id`, so no combat log or undo reads it.
The leak tests (`internal/leaktest`) roll a secret roll and a table roll before the reads and again while the streams are open, with the id of the secret roll as the needle and the table roll as the positive control.

## The event and the log line

A roll is one `free_roll_rolled` event (a row of `session_event_kinds`, the `eventFreeRoll` constant) written in the transaction that locks the open session.
The payload has numbers only: sides, count, modifier, mode, each face with `discarded`, the total, `by_master` and `secret` (see [Data](../../data/play.md#session_events-and-session_event_kinds)).
The log line is built from the payload and the roller's display name when it is read: "Rolagem livre · Brisa: 2d6 + 3 = 11 (4, 4)", and under advantage "Rolagem livre · Brisa: 1d20 + 5 com vantagem = 19 (14, 9 descartado)".
`FreeRoll` carries it ready (`log_text`), with the `formula` ("2d6 + 3", "1d20 - 2", "1d20 + 5 com vantagem") and the dice with `discarded`, so the app draws the dice, the result and the log from one message.
An account without a name reads as "Mestre" (the master) or "Jogador".

## Reading the rolls

`ListFreeRolls` answers for the open session only:

- `table` is the newest 50 rolls the caller may read, newest first: the log of the session's free rolls.
- `mine` is the caller's own newest 10, secret ones included for the master who made them: "Suas últimas rolagens", where "Repetir" asks for the same dice again.

A new session starts both lists empty.

## The key, the limit and the retry

- **The idempotency key is required**, 1 to 64 characters (`idem.Clean`).
  `session_events.idempotency_key` is a UUID, so the server stores a UUID derived from the member and the key (`freeRollKey`): two members may send the same text, and one member's key is unique in the session.
  The same key with the same request returns the first roll (same id, same dice, same time) and rolls and writes nothing (the roll is told again, see below); the same key with another request is `invalid_argument` (the request hash, `idem.Hash`).
  The app makes a key per tap and renews it after a roll succeeded, so a double tap rolls once.
- **The rate limit** is per user: a burst of 20 and one more roll every 3 seconds (`ratelimit.Policy.FreeRoll`, wired as `play.Config.FreeRolls`), checked right after the membership and before anything else, as the lines of the conversation are.
  A refusal is `resource_exhausted` with `Retry-After`, the answer the app already turns into "Tente de novo em N segundos" ([API](../api.md#abuse-limits)).
- **After the commit** a table roll is published to the hub with the session's number, which never waits for a slow stream ([The hub](live-session.md#the-hub)); a secret roll is published to nobody, and moves the number all the same. A repeat of the key tells the table roll again, the same roll to the same streams, and moves nothing.
