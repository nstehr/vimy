-- Does the stream agree with the counters?
--
--   make verify GAME=148
--
-- The stream and SQLite's rule_firings measure the same thing by different
-- routes: one counts rows, the other increments an integer in the rule loop.
-- Keeping both is deliberate. A disagreement is a bug in one of them, and
-- finding it beats trusting either alone -- this whole line of work started
-- with a confident count off a sampled table that was simply wrong.
--
-- EXPECT A KNOWN, POSITIVE DRIFT. rule_firings cannot hold anything that fired
-- before the first rule-set swap: strategist.go flushes the window counters
-- unconditionally and then attaches them only `if n := len(s.history); n > 0`,
-- so the seed window is read out and dropped for want of a prior record. In
-- game 149 that is ticks 10-1100 -- the MCV deploy, the first power plant, the
-- first refinery. The stream has them; the counter never did.
--
-- So the comparison below is scoped to ticks at or after the first swap, which
-- is the only like-for-like window. Within it, expect EXACT agreement: the
-- stream is unsampled and has no excuse for a shortfall. Drift there means
-- either the log dropped rows -- check rows_dropped above -- or the two
-- definitions have come apart.
--
-- The second query reports the pre-swap firings the counter cannot see, which
-- is not drift but the opening, and is worth reading on its own.

-- First: was this game streamed at all? A game played before -stream, or with
-- it off, has no session here -- and then every rule below shows a negative
-- drift that looks like total disagreement when the answer is "no data".
SELECT
    {game:UInt32}                        AS game,
    count()                              AS sessions,
    if(count() = 0,
       'NOT STREAMED -- the drift below is the whole counter, not a disagreement',
       'streamed')                       AS status,
    sum(rows_dropped)                    AS dropped,
    if(sum(rows_dropped) > 0,
       'counts below are a FLOOR',
       'complete')                       AS completeness
FROM currie.stream_sessions FINAL
WHERE game_id = {game:UInt32};

SELECT
    rule,
    stream_fired,
    counter_fired,
    stream_fired - counter_fired AS drift
FROM
(
    SELECT
        e.rule                AS rule,
        countIf(e.fired)      AS stream_fired
    FROM currie.stream_rule_evals AS e
    INNER JOIN currie.stream_sessions AS s FINAL USING (session_id)
    WHERE s.game_id = {game:UInt32}
      AND e.tick >= (
          -- the first swap: the earliest tick running a rule set other than
          -- the one the game opened with
          SELECT min(tick) FROM currie.stream_rule_evals AS x
          INNER JOIN currie.stream_sessions AS y FINAL USING (session_id)
          WHERE y.game_id = {game:UInt32}
            AND x.rule_set != (
                SELECT argMin(rule_set, tick) FROM currie.stream_rule_evals AS z
                INNER JOIN currie.stream_sessions AS w FINAL USING (session_id)
                WHERE w.game_id = {game:UInt32}))
    GROUP BY rule
) AS st
FULL OUTER JOIN
(
    SELECT
        f.rule_name         AS rule,
        sum(f.fire_count)   AS counter_fired
    FROM sqlite('vimy.db', 'rule_firings') AS f
    INNER JOIN sqlite('vimy.db', 'archived_doctrines') AS d ON d.id = f.doctrine_id
    WHERE d.game_id = {game:UInt32}
    GROUP BY rule
) AS ct
USING (rule)
ORDER BY abs(drift) DESC, rule
LIMIT 40;

SELECT '--- the opening, which rule_firings structurally cannot see ---';

SELECT
    e.rule    AS rule,
    count()   AS fired_before_first_swap,
    min(e.tick) AS first,
    max(e.tick) AS last
FROM currie.stream_rule_evals AS e
INNER JOIN currie.stream_sessions AS s FINAL USING (session_id)
WHERE s.game_id = {game:UInt32} AND e.fired
  AND e.tick < (
      SELECT min(tick) FROM currie.stream_rule_evals AS x
      INNER JOIN currie.stream_sessions AS y FINAL USING (session_id)
      WHERE y.game_id = {game:UInt32}
        AND x.rule_set != (
            SELECT argMin(rule_set, tick) FROM currie.stream_rule_evals AS z
            INNER JOIN currie.stream_sessions AS w FINAL USING (session_id)
            WHERE w.game_id = {game:UInt32}))
GROUP BY rule
ORDER BY fired_before_first_swap DESC;
