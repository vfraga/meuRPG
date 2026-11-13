# Transactions and the connection pool

Inside a `db.InTx` or `db.ReadTx` closure, nothing reads through the pool: every statement goes through the transaction.
The closure already holds a pool connection, and a read through the pool takes a second one while the first is held.
With a few simultaneous requests every one holds a connection and waits for another, and all hang until their context ends.

## The helpers

| Helper | Use |
| --- | --- |
| `db.InTx(ctx, pool, fn)` | Every write. It runs `fn` in a transaction and, on SQLSTATE `40001` (a serialization conflict, on both databases), rolls back and runs `fn` again. SQLSTATE `40P01` (a deadlock) is retried too. |
| `db.ReadTx(ctx, pool, fn)` | A read that joins several statements into one response. `READ ONLY` on every attempt, with the same retry. |

Both run one loop of our own (`backend/internal/platform/db/tx.go`) on CockroachDB and on PostgreSQL.
The loop backs off with the cockroach-go retry policy.
It opens each attempt with `BEGIN` at `SERIALIZABLE` and the transaction's access mode.
It reads the error of `COMMIT`.
A lost serialization race can surface there, so a lost write is never reported as success.
A `COMMIT` that PostgreSQL answers with `ROLLBACK` returns that error as a plain failure.
A `COMMIT` whose outcome is unknown (`40003`, or a connection that broke) returns `*db.AmbiguousCommitError`, which `rpcerr` answers as an unknown outcome and never retries.

`fn` may run more than once, so it follows the rules below.
The pool opens at most 10 connections unless `pool_max_conns` is in `DATABASE_URL` (`db.defaultMaxConns`; why 10 is in [Operations](../operations/reference.md#database-connections)), and every statement has a 30 s `statement_timeout` (`db.defaultStatementTimeout`).
The pool size is a safety net, not the fix: no path may hold a connection and ask for another, whatever the size.

A call whose context ends is cancelled by a pgwire cancel request, not by expiring the connection's read deadline at once (`db.NewPoolWith`).
The deadline is set 5 s ahead (`db.cancelDeadlineFallback`) as the fallback if the server does not answer the request, and cleared when the call ends.
A cancelled call returns its connection to the pool after about 100 ms, the time pgx waits so the cancel cannot reach the next statement.

## Rules for a closure

| Rule | How it is met |
| --- | --- |
| Everything the transaction reads or writes goes through it | `queries.WithTx(tx)`, or a function that takes the `pgx.Tx`. Never the service's `s.queries` or `s.pool`, and never a function of another module that reads through the pool. |
| A cross-module call that reads receives the transaction | Its first argument after `ctx` is `tx pgx.Tx`; `nil` means the pool, for callers outside a transaction. Each module has a `queriesIn(tx)` that picks the transaction or the pool. Examples: `campaigns.XPAwards.AwardedXP`, `maps.MapDefaults.FogOnNewMaps`, `characters.ContentSource.For`, `progression.Campaigns.CampaignXPMode`. |
| A read error inside the transaction propagates | In CockroachDB a failed statement aborts the transaction and the next one fails with `25P02`, which nobody retries. Swallowing a `40001` (log and go on) turns a retryable conflict into a failure. With a `tx`, the error comes back wrapped with `%w` so `InTx` can see the code; only the pool path may log and go on. |
| The closure starts from the same state on every run | A variable shared with the code around the closure is not overwritten with something that exists only inside one attempt; it is assigned to a local, and flags are reset at the top. |
| Inside the transaction nobody waits for another request | A cache filled by single-flight (the compiled fog scene in `maps`) is not filled from inside a transaction: whoever holds a connection must not wait for someone who may be waiting for one. |
| What must not change during the transaction, or does its own write, is prepared before | The fog view is read first. Files an NPC portrait needs are copied beforehand and the gallery row is inserted inside the transaction; if it fails the files are removed. |
| No effect outside the database | Publishing to the stream, touching files, logging a game event and calling another service wait until after the commit, because the closure may run again. The `play` module collects its hints in the closure and `Service.tx` sends them ([Hints sent after the commit](#hints-sent-after-the-commit)). |

## Hints sent after the commit

The combat write path of the `play` module (`write`, `changeInTx`, `StartEncounter`) runs in `Service.tx(ctx, sessionID, fn)`, which wraps `db.InTx` and sends after the commit.
Other writers of the module call `db.InTx` and then `send`; they do not bump the hint number.
`txOn` is the same on any transaction starter; a test passes one whose `COMMIT` fails.

| Rule | How it is met |
| --- | --- |
| Lock first | The first statement of every attempt takes the session's row (`FOR UPDATE`), before the closure reads. A closure that locks it again finds it held |
| Collect, do not send | The closure calls `h.tell(campaignID, event)` for each hint, and `h.touch()` for a write that sends none but must move the screens. Nothing goes out before the commit |
| Bump once | If the closure told or touched, `Service.tx` adds 1 to `game_sessions.hint_seq` in the same transaction, once however many events it told. A closure that told nothing writes nothing to the session |
| A fresh collector | `h` is empty at the start of every attempt, so a retry keeps nothing of a rolled-back one |
| Send after the commit | Each told event is stamped with the new number and sent through `send`, in the order told; then the steps of `h.after` run, in order, with the context of `afterCommit` (the request's values, without its cancellation, bounded) |
| An error sends nothing | The number stays as it was |
| An ambiguous commit sends nothing | `*db.AmbiguousCommitError` is not retried and `Service.tx` returns it before it sends; if the write did commit, the next heartbeat shows the number ahead and the page reads again |
| A repeat of a stored key replays | The closure calls `h.replay()` and tells what the first call would have; the events go out with the number as it stands, and it is not bumped |

A hint that goes with no write (typing, presence, the table screen's "changed") does not go through `Service.tx`: it calls `send`.
The effect on a session's live streams is in [Live session](play/live-session.md#the-hint-number).

### What the archtest sees

`TestHintsAreNotSentInsideATransaction` (`backend/internal/archtest/txhints_test.go`) fails for:

- a call to `send` or `tellMembers` inside a function literal passed to `Service.tx` or `db.InTx` in a non-test file of `internal/play`, because it would tell before the commit and again on a retry;
- a call to `BumpHintSeq` outside `Service.txOn`, and a string literal that names `hint_seq` in a non-test file, because a raw `UPDATE` would move the number outside the funnel.

Its exceptions (`txhints_exceptions.txt`) can only shrink.
Its limits:

- It reads a closure given as a literal, or as an identifier assigned a literal in the same function. A method value, a parameter or a package-level function is not read.
- A helper that the closure calls and that itself calls `send` is not seen; only the closure's own calls are.
- It reads the files directly in `internal/play` only.

The behaviour is proved by `backend/internal/play/hints_test.go`: the collector reset on a retry, an error and an ambiguous commit that send nothing, the number saved with the write, one bump per transaction, the lock taken first, and the replay that does not bump.
`TestHintSeqContentionOnCockroachDB` runs concurrent writers of one session on CockroachDB.

## Reads that join several statements

A loose statement outside a transaction picks its own instant, so a write that commits between two statements of one response gives a view that mixes two states (the new combatant list next to the old revision).
`db.ReadTx` opens a `READ ONLY` transaction, and all its statements see one instant.
It begins every attempt as `READ ONLY`.
It does not use `AS OF SYSTEM TIME follower_read_timestamp()`: that reads data a few seconds old, and in live play a player must see what the table just did.
An ordinary CockroachDB read transaction reads what was committed at its start and never blocks writers.
The single-connection rule applies inside it too.

Every read RPC that puts several rows together uses it (the combat views in `play`, the map reads in `maps`, the table rules, the XP lists, the gallery list, the level-up previews, the campaign export).
Each has a test with a hook between the reads, on a second pool, that commits a write from the hook.
What a view adds from another module (names, HP, fog) is read afterwards, outside the transaction: one transaction across modules would be a change of its own.

## What proves it

Every test pool (`platform/dbtest`) has one connection (`MaxConns = 1`) and an `Acquire` tracker.

- An acquisition made from inside a `db.InTx` closure fails the test at once, with the stack, and the `Acquire` is canceled, so nothing waits.
- A wait of more than 90 s for a connection (the case the stack cannot see, such as a goroutine the closure started) brings down the whole package's test binary with the stack of whoever waits.
- So every write in every package runs on one connection, and a new call that reads through the pool inside a transaction breaks the test that exercises it.
- A test that races transactions asks for a bigger pool with `dbtest.PoolSize(t, n)` (n at least the number of racers) and starts them together with `dbtest.NewBarrier`.
  A test that must hold a transaction open while the service works uses `dbtest.SideConnection`.
- `MEURPG_TEST_POOL_MAX_CONNS=8` applies to the whole process and lists all violations at once.

How to run these tests is in [CONTRIBUTING](../development.md#test-databases).
