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
| No effect outside the database | Publishing to the stream, touching files, logging a game event and calling another service wait until after the commit, because the closure may run again. |

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
