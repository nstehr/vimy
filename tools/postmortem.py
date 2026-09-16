#!/usr/bin/env python3
"""What a game cost and what stopped it.

Three analyses that were hand-rolled in throwaway scripts across a long
session, one of them four separate times:

  the ledger   what the engine recorded, including the value of the trade
  the spend    what the money bought, priced from the engine's own rules
  the blame    which clause of which rule blocked it, and how often alone

The blame half is not new work — `vimyc --blame` has always reported it.
It was written again by hand four times because nobody ran the tool.

  tools/postmortem.py            the most recent game
  tools/postmortem.py 122        a particular one
"""

import gzip
import json
import os
import re
import sqlite3
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DB = Path(os.path.expanduser("~/.vimy/vimy.db"))
EXPORTS = Path(os.path.expanduser("~/.vimy/exports"))
RULES = ROOT / "vimy-core" / "rules" / "vy"
VIMYC = ROOT.parent / "vimyc" / "target" / "release" / "vimyc"
ENGINE_RULES = ROOT / "engine" / "mods" / "ra" / "rules"


def engine_costs():
    """Unit and building prices, read from the engine rather than copied.

    A table transcribed by hand drifts the moment the mod changes, and the
    whole point of pricing the spend is that the numbers are real.
    """
    costs = {}
    for path in sorted(ENGINE_RULES.glob("*.yaml")):
        current = None
        for line in path.read_text(errors="ignore").splitlines():
            header = re.match(r"^([A-Za-z0-9._]+):\s*$", line)
            if header:
                current = header.group(1).split(".")[0].lower()
                continue
            cost = re.match(r"^\s+Cost:\s*(\d+)", line)
            if cost and current and current not in costs:
                costs[current] = int(cost.group(1))
    return costs


# What each producing rule actually buys. Rules that build several things name
# the representative one; the totals are an estimate and say so.
RULE_ITEM = {
    "produce-infantry": ("e1", "infantry"),
    "produce-infantry-rush": ("e1", "infantry"),
    "produce-bridge-infantry": ("e1", "infantry"),
    "produce-capture-defense-infantry": ("e1", "infantry"),
    "produce-rocket-soldier": ("e3", "infantry"),
    "produce-grenadier": ("e2", "infantry"),
    "produce-attack-dog": ("dog", "infantry"),
    "produce-specialist-infantry": ("e4", "infantry"),
    "produce-spy": ("spy", "infantry"),
    "produce-engineer": ("e6", "infantry"),
    "produce-vehicle": ("2tnk", "armour"),
    "produce-heavy-vehicle": ("3tnk", "armour"),
    "produce-siege-vehicle": ("arty", "armour"),
    "produce-scout-vehicle": ("1tnk", "armour"),
    "produce-apc": ("apc", "armour"),
    "produce-assault-apc": ("apc", "armour"),
    "produce-flak-truck": ("ftrk", "armour"),
    "produce-extra-harvester": ("harv", "harvester"),
    "rebuild-harvester": ("harv", "harvester"),
    "produce-aircraft": ("heli", "air"),
    "produce-basic-aircraft": ("heli", "air"),
    "produce-attack-aircraft": ("mig", "air"),
    "build-base-defense": ("pbox", "defence"),
    "build-base-defense-rush": ("pbox", "defence"),
    "build-aa-defense": ("agun", "defence"),
    "build-power": ("powr", "economy"),
    "build-advanced-power": ("apwr", "economy"),
    "build-refinery": ("proc", "economy"),
    "build-second-refinery": ("proc", "economy"),
    "build-extra-refinery": ("proc", "economy"),
    "build-war-factory": ("weap", "economy"),
    "build-barracks": ("tent", "economy"),
    "build-radar": ("dome", "economy"),
    "build-service-depot": ("fix", "economy"),
    "build-tech-center": ("atek", "economy"),
    "build-airfield": ("hpad", "economy"),
    "build-extra-airfield": ("hpad", "economy"),
}


def latest_game(db):
    row = db.execute("SELECT MAX(id) FROM games").fetchone()
    return row[0] if row else None


def ledger(db, game):
    cols = ("duration_ticks won our_faction opponent_faction engine_kills_cost "
            "engine_deaths_cost engine_buildings_killed engine_earned "
            "our_army_peak enemy_army_seen_peak infantry_lost vehicles_lost "
            "vehicles_lost_forward").split()
    row = db.execute(f"SELECT {','.join(cols)} FROM games WHERE id=?", (game,)).fetchone()
    if not row:
        sys.exit(f"no game {game}")
    v = dict(zip(cols, row))
    print(f"GAME {game}: {v['duration_ticks']} ticks, "
          f"{'WON' if v['won'] else 'lost'}, "
          f"{v['our_faction']} vs {v['opponent_faction']}")
    kills, deaths = v["engine_kills_cost"], v["engine_deaths_cost"]
    if kills and deaths:
        print(f"  trade      destroyed {kills} credits, lost {deaths}  "
              f"= 1:{deaths / max(kills, 1):.2f}")
    if v["engine_buildings_killed"] is not None:
        print(f"  buildings  destroyed {v['engine_buildings_killed']}")
    if v["our_army_peak"] is not None and v["enemy_army_seen_peak"]:
        ours, theirs = v["our_army_peak"], v["enemy_army_seen_peak"]
        print(f"  army peak  ours {ours}, theirs {theirs} (seen — a floor)"
              f"  = {theirs / max(ours, 1):.1f}x")
    if v["vehicles_lost"] is not None and v["vehicles_lost_forward"] is not None:
        home = v["vehicles_lost"] - v["vehicles_lost_forward"]
        print(f"  losses     {v['infantry_lost']} infantry, {v['vehicles_lost']} vehicles "
              f"({v['vehicles_lost_forward']} forward, {home} at home)")
    return v


def spend(db, game, earned):
    costs = engine_costs()
    rows = db.execute(
        "SELECT f.rule_name, SUM(f.act_count) FROM rule_firings f "
        "JOIN archived_doctrines d ON d.id = f.doctrine_id "
        "WHERE d.game_id = ? AND f.act_count > 0 GROUP BY f.rule_name", (game,)).fetchall()

    by_kind, detail, unpriced = {}, [], []
    for name, acted in rows:
        entry = RULE_ITEM.get(name)
        if not entry:
            continue
        item, kind = entry
        price = costs.get(item)
        if price is None:
            unpriced.append(item)
            continue
        total = price * acted
        by_kind[kind] = by_kind.get(kind, 0) + total
        detail.append((name, acted, price, total))

    grand = sum(by_kind.values())
    if not grand:
        print("\n  no priced production recorded")
        return
    print(f"\n  SPEND (estimated: rule acts x engine price)"
          f"{f', against {earned} earned' if earned else ''}")
    for kind, total in sorted(by_kind.items(), key=lambda kv: -kv[1]):
        print(f"    {kind:<10} {total:>7}  {100 * total // grand:>3}%")
    print(f"    {'TOTAL':<10} {grand:>7}")
    print("    largest lines:")
    for name, acted, price, total in sorted(detail, key=lambda d: -d[3])[:6]:
        print(f"      {name:<28} {acted:>4} x {price:<5} = {total:>7}")
    if unpriced:
        print(f"    (no engine price for: {', '.join(sorted(set(unpriced)))})")


def blame(game, top):
    """Which clause stopped which rule — from vimyc, not rewritten here."""
    exports = sorted(EXPORTS.glob("*.json.gz"), key=lambda p: p.stat().st_mtime)
    if not exports or not VIMYC.exists():
        print("\n  no export or vimyc binary; skipping blame")
        return
    with gzip.open(exports[-1]) as fh:
        states = json.load(fh).get("states", [])
    if not states:
        print("\n  export has no states")
        return

    params = {}
    for (js,) in sqlite3.connect(DB).execute(
            "SELECT doctrine_json FROM archived_doctrines WHERE game_id=? LIMIT 1", (game,)):
        try:
            doc = json.loads(js)
        except json.JSONDecodeError:
            continue
        params = {k.replace("_", "-"): v for k, v in doc.items()
                  if isinstance(v, (int, float)) and not isinstance(v, bool)}
    # Params the doctrine did not carry still have to bind.
    for decl in RULES.glob("*.vy"):
        for name, kind in re.findall(r"^param ([\w-]+):\s*(\w+)", decl.read_text(), re.M):
            params.setdefault(name, 0 if kind == "int" else 0.0)

    with tempfile.TemporaryDirectory() as tmp:
        sp, pp = Path(tmp) / "states.json", Path(tmp) / "params.json"
        sp.write_text(json.dumps(states))
        pp.write_text(json.dumps(params))
        run = subprocess.run([str(VIMYC), str(RULES), "--blame", str(sp), "--params", str(pp)],
                             capture_output=True, text=True)
    if run.returncode != 0:
        print(f"\n  blame failed: {run.stderr.strip().splitlines()[-1:] or run.returncode}")
        return
    report = json.loads(run.stdout)
    rules = report["rules"] if isinstance(report, dict) else report

    print(f"\n  BLAME (from vimyc, {len(states)} sampled states) — "
          f"rules that never held, worst first")
    ranked = [r for r in rules if r.get("held", 0) == 0 and r.get("seen", 0) > 0]
    ranked.sort(key=lambda r: -max((c.get("sole", 0) for c in r.get("clauses", [])), default=0))
    for r in ranked[:top]:
        worst = max(r.get("clauses", []), key=lambda c: c.get("sole", 0), default=None)
        if not worst or not worst.get("sole"):
            continue
        print(f"    {r['rule']:<30} never held; sole blocker {worst['sole']:>4}x: "
              f"{(worst.get('source') or '').strip()[:52]}")


def main():
    if not DB.exists():
        sys.exit(f"no database at {DB}")
    db = sqlite3.connect(DB)
    game = int(sys.argv[1]) if len(sys.argv) > 1 else latest_game(db)
    v = ledger(db, game)
    spend(db, game, v.get("engine_earned"))
    blame(game, top=8)


if __name__ == "__main__":
    main()
