# The table screen

The table screen is the TV at the table: a browser tab the master opens on a screen the whole table looks at.
It shows what the master chose, and only what every player may read (RN-10).
The code is `backend/internal/play/tablescreen*.go` and `backend/internal/maps/tablescreen.go`; the contract is `proto/meurpg/play/v1/table_screen.proto`.

## Three calls, all the master's

`TableScreenService` has three calls.

| Call | What it does | Own errors |
| --- | --- | --- |
| `GetTableScreen` | Answers the choice, what the TV shows now, the names for the header, the panel's options and whether a TV tab is open | `failed_precondition` `NO_OPEN_SESSION` |
| `SetTableScreen` | Writes the master's choice | `NO_OPEN_SESSION`; `invalid_argument` for a missing choice or a pin that is not an item; `aborted` when `choice_revision` is no longer the stored one |
| `WatchTableScreen` | The stream of the TV tab, see [The stream](#the-stream) | `NO_OPEN_SESSION` |

A pending member and a non-member get `not_found` on all three.
A player gets `permission_denied`, with a message that names nothing.
The TV tab is signed in as the master: it has no account of its own.
The app renders what `GetTableScreen` answers and orders nothing.

## Where the choice lives

The choice is three columns of the open session (`game_sessions`): `tv_mirror`, `tv_pin` and `tv_choice_revision` ([Play data](../../data/play.md)).
It is a stored row, not hub memory.
So a deploy, a second instance or a reloaded tab keeps it.
`SetTableScreen` locks the session row, checks the revision, writes the choice, bumps the revision and writes nothing else.
After the commit the hub tells the master's streams (`table_screen_changed`) and the TV streams (a `changed`), and no player's stream.
A pin stays stored when its item ends; the stage then falls to the idle screen with the notice `PIN_ENDED`, and a new item of that kind brings the pin back.

## The three decision points

What the TV shows is decided in three places, each one function in `tablescreen_resolve.go`.

| Decision | Function | Rule |
| --- | --- | --- |
| Which screen | `resolveStage` | Following the table, the first that exists: the image shown to the whole table, the combat, the scene open to the whole table, the map the TV may show, the idle screen. A pin shows its item when it can be shown, and otherwise the idle screen with the notice that says why. The master's own tabs do not count |
| Whether a map goes to the TV | `mapOnTV` | Not when the fog is on and "Visão do grupo" is off: the TV would show what no player sees together |
| Whether the conversation goes to the TV | `tvShowsConversation` | Never. It is a constant, `false`, not a function. The Conversa (RN-36), narration to the whole table included, is not on the TV |

`tvOptions` uses the same facts to say which items the panel can pick and why the others cannot.

## The TV's viewer and the projector

The TV reads as one synthetic viewer, a player with no account and no character (`tvViewer`, `tablescreen_viewer.go`).
Its user id is empty: it owns nothing, has nothing of its own, and reads a scene or an image only when it is open to the whole table.
The TV has no filter of its own.
The reads of a player run as that viewer (`viewFor`, `shownImageFor`, `stageOf`, the combat log, the free rolls).
The projector copies a whitelist of fields from what they answer into the narrow `Tv*` messages (`tablescreen_project.go`).
A field added to `Encounter` or `OpenSceneInfo` never reaches the TV; a new TV field is a new line in the projector.
`TvMap.map` and `TvMap.vision` embed the player form of the `mapsv1` messages, so they are the exception.
A new field there reaches the TV under the maps module's `masterOnly` rules, not under a `Tv*` row.
`TestTvProjectorFields` lists every field of the projected messages and fails for one that has no reason.

| Stage | What the TV receives |
| --- | --- |
| Image | The name, the route and the size of the image shown to the whole table |
| Scene | The name and description of a scene open to the whole table, and the name and "speaking" mark of the NPCs the table sees on the stage; no hooks, clues, actions or DCs |
| Combat | The round, the combatants the table sees with name, initial, one word of state (RN-20) and whose turn it is, "Vez do mestre" when the turn is one the table does not see, up to 6 log lines every player reads, and the combat's map |
| Map | The map as the maps module answers the TV's viewer, or `hidden_by_fog` and nothing else |
| Rolls | With a scene, a combat or a map: at most 3 free rolls made for the whole table, newest first |

The header carries the campaign name, the session number and the names of the living player characters.
The server sends up to 6 log lines in every combat, newest first.
The app chooses how many to draw.
A log line is a plain sentence made from the entry's kind and the labels a player reads.
Only a few kinds have a sentence: the combat begun and ended, a door opened, a move, an attack, a spell and an action.
Any other kind of entry is not on the TV.
A name of table content (RN-23) is not told, because the master may have hidden the entry.
A spell or an action of the table reads "conjurou uma magia" or "agiu"; the name of a creature's own action is told as a player reads it.
A table creature in the combat is named in the order, as every player reads it.
The server sends the word FALLEN for a character that is down or stable, and DEFEATED for a dead one, as the player reads it.
An up player character has no word.

The TV never receives:

- hit points, armor class, initiative totals, conditions or effects, and an NPC's dice;
- a hidden NPC, or one the table does not see on a fog map;
- hidden entries of the table's content;
- the master's notes, hooks, clues and DCs;
- a roll "Só eu", a log line only some players read, and the conversation;
- the fog's hidden squares, the light the master painted, and the map's image when the fog is on;
- a scene or an image meant for chosen players.

## The map

`maps.TableScreenMap` reads a map for the TV's viewer: the map, its points and tokens from the code of the player's `GetMap`, and the squares from the code of `GetMapVision`.
The viewer is the maps the players see now, with no trap known and nothing owned.
With the fog on, "Visão do grupo" on gives the union of the characters' sight and "Visão do grupo" off gives nothing, so `mapOnTV` has already stopped the map.
The map's own revision is left out: the TV carries a hash of what it reads, which does not move for a change only the master sees.
The map's image does not go to the TV when the fog is on.

### Tiles

A fog map is drawn from tiles, as for a player ([Fog of war](../maps/fog.md#the-route-and-the-index)).
The route takes `?as=tv` from the master and builds the tiles of the group's vision, black where the group does not see.
A player, a pending member and a stranger asking `?as=tv` get the `404` of a tile that does not exist, byte for byte; someone with no session gets `401`.
With "Visão do grupo" off, the master gets that `404` too.

## The stream

`WatchTableScreen` is a hub subscription of the master that forwards no event.
It hears what the master hears.
Each event, coalesced to at most one per 250 ms, becomes a `changed` that carries nothing: not a revision, not a part, not a count.
The tab then calls `GetTableScreen`, and what it shows is only that read.
This is why the TV shows nothing when a hidden NPC moves, even though the master's own stream knows it.
The stream sends `ready` first, `heartbeat` every 25 s and `session_ended` when the session ends.
It lives at most 10 minutes, like `WatchGameSession`.
Its hub user id is `tv:` and the master's id.
So the TV's streams do not count in the master's cap of four streams.
A newer one replaces the oldest of its own.

### Presence

The master's panel says "Aberta", "Fechada" or "Sem conexão desde 21:14".
The play service counts the TV streams of each session in memory (`Service.screens`), like the live streams: a connection is not data.
With more than one server instance, a session's count is the share the instance holds.
A change of the count reaches the master's streams as `table_screen_changed`, with `presence_known` set; the TV streams do not hear it, so their own coming and going starts no loop.
`GetTableScreen` carries the count it holds now.
When the session ends the count is dropped, because nothing was lost.

## Tests

The leak tests inspect the master's `GetTableScreen` as a player's answer, with the master's own reads as the positive controls ([Leak tests](../../testing.md#leak-tests-rn-10)).
The projector, the decision points, the stream and the presence have tests in `backend/internal/play`, and the TV's map and tiles in `backend/internal/maps`.
