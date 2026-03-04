# Strategy Improvements

Identified gaps and improvements for the grid trading bot strategy. Each item is a standalone work item to evaluate and potentially integrate into the strategy doc.

Status key: `open` = not yet addressed, `accepted` = will implement, `rejected` = decided against, `done` = merged into strategy doc.

---

## Critical Gaps

### 1. Hedge lock must always use taker order

Status: `accepted`

The strategy doc says hedge lock opens "an equal short (maker order)." During a crash, a maker order sits on the book while price keeps falling — by the time it fills (if it fills — post_only rejects it if price has already moved past), the loss has grown beyond the intended lock threshold.

The context doc already has the correct answer: "The hedge lock uses a TAKER order (market) in emergency." The strategy doc needs to match this.

#### Decision: always taker, no "try maker first" path

A maker-first-then-fallback approach saves 0.045% per hedge lock but introduces a timing decision (how long to wait?) that can't be answered correctly. During a crash, any delay costs more than the fee savings.

Taker fee at each account scale (one hedge lock covering all positions on one side):

```
\$100 account  (0.003 BTC hedge):  \$0.11 taker fee
\$1,000 account (0.025 BTC hedge): \$0.94 taker fee
\$10,000 account (0.14 BTC hedge): \$5.24 taker fee
```

Even at \$10,000, the fee is 0.05% of account — negligible vs the -3% loss being locked. Meanwhile, a 2-second delay during a crash costs more than the fee savings:

```
\$10,000 account, 0.14 BTC position:
  BTC drops \$50 in 2 seconds (conservative for a crash)
  Extra loss from delay: 0.14 x \$50 = \$7.00
  Fee saved by maker:   0.045% x 0.14 x \$68,000 = \$4.28
  Net: -\$2.72 worse off trying maker first
```

Safety mechanisms should be simple and fast. One code path, guaranteed fill.

Action: update `docs/trading-bot-strategy.md` to state hedge lock uses taker/market order as the sole exception to maker-only policy. Update `docs/trading-bot-strategy-context.md` to remove ambiguity — the context doc already says taker but should reference the scaling analysis above.

---

### 2. Correlated losses across grid levels — per-trade risk understates real exposure

Status: `accepted`

Position sizing says "max 1-2% loss per trade." But during a trend, ALL positions on the wrong side lose simultaneously. With 3 shorts open during a pump, the aggregate unrealized loss is 3x a single level.

At \$100 account with \$200 spacing this is manageable (~\$0.60 aggregate). But at the \$500 scaling tier (\$100 spacing, 5 levels), aggregate exposure on one side is:

```
5 levels x 0.002 BTC x \$500 move = \$5.00 = 1% of account
```

The scaling guide should include a correlated worst-case column showing aggregate exposure, not just per-trade risk. This informs circuit breaker thresholds too — Level 1 (-3% unrealized) needs to account for multi-level correlation.

Action: add aggregate exposure calculation to the scaling guide.

---

### 3. Restart reconciliation algorithm is unspecified

Status: `open`

"Must survive restarts" and "state persistence" are mentioned but the actual reconciliation logic is missing. On restart the bot must answer:

- Which orders are still live on the exchange? (query open orders)
- Which orders filled while the bot was down? (query trade history since last known state)
- Is a hedge lock active? What's its expiry timer?
- Is a circuit breaker pause active?
- Has the grid drifted while the bot was offline?
- Were any orders cancelled by the exchange during maintenance?

Incorrect reconciliation means duplicate orders, missed take-profits, or orphaned positions. This is a critical path — the bot WILL crash.

Action: design and document the restart reconciliation algorithm as part of the strategy before implementation.

---

### 4. Hedge lock exit trigger is undefined

Status: `open`

The strategy says "bounce happens -> close short, ride long back up." But at what price? Options:

- **Breakeven exit:** close hedge when price returns to original entry. Maximum recovery but may never trigger.
- **Partial recovery exit:** close hedge when 50-70% of locked loss is recovered. More likely to trigger, still positive outcome.
- **EMA cross exit:** close hedge when price crosses back above EMA. Aligns with trend filter logic.
- **Trailing exit:** once price starts recovering, set a trailing stop on the hedge that locks in partial recovery.

Partial recovery (50-70%) is probably the sweet spot — better than the 0% recovery of a stop-loss, and much more likely to trigger than waiting for full breakeven.

Action: define the explicit exit rule with a concrete threshold. This is also a parameter the simulation system should optimize.

---

### 5. Post-only rejection retry logic is missing

Status: `accepted`

When a post_only order gets rejected (price moved past the order level, would fill as taker), the grid has a gap. The strategy doesn't specify what happens next.

Handling differs by order type because the risk profile is opposite:

#### Entry orders (grid buy/sell to open a position)

Options considered:

**A. Taker market order (fill immediately)**
Place a market order at current price to guarantee fill. Ensures no grid gap.
Downside: taker fee (0.055%) on every rejection, contradicts maker-only strategy. Chasing a dropping price is the opposite of grid logic — the grid wants to buy low, not chase.

**B. Maker retry at original grid level price**
Re-place the same order at the same price with `post_only: true`.
Downside: if price has moved significantly (e.g. \$200 drop), the original price is now far from market and won't fill. The order sits as a stale entry and the grid gap persists until the next rebalance.

**C. Maker retry at current market price (chosen)**
Place a new `post_only: true` order at the current best price (`current_ask - min_price_increment` for buys, `current_bid + min_price_increment` for sells). One retry only.

```
on_entry_post_only_rejected(order):
  if order.side == BUY:
    new_price = current_ask - min_price_increment
  if order.side == SELL:
    new_price = current_bid + min_price_increment

  place_order(new_price, post_only=true, grid_level=order.original_grid_level)
  // If rejected again, log and skip. Grid self-heals via rebalancing.
```

Why C is the best option: the grid logic is built on patience — buy low, wait for bounce. If the ask dropped to \$59,749 (below the original \$59,800 grid level), that's a better entry. Placing a maker order there is exactly what the grid wants. Using taker (option A) to chase a dropping price contradicts the strategy. Retrying at the original price (option B) is stale and won't fill.

The TP target stays at the original grid level (not adjusted entry + spacing), preserving grid structure and capturing slightly more profit from the better entry.

If the maker retry also fails, price is moving fast. Skip it — the rebalancing logic handles it on the next 15-min candle or emergency rebalance.

#### Take-profit orders (close a position at profit target)

Options considered:

**A. Maker retry at original TP price**
Re-place the same TP order at the same price with `post_only: true`.
Downside: if price has moved past the TP level, the original price is now below market. A maker sell at \$60,000 when price is at \$60,020 would fill as taker and get rejected again. Even if placed at \$60,021, price could reverse below \$60,000 before fill — the profit disappears and the position could turn into a loss.

**B. Maker retry at current market price**
Place a new `post_only: true` sell at `current_bid + min_price_increment`.
Downside: same reversal risk as option A. The profit already exists right now — any delay risks losing it. A maker order sits on the book waiting to fill while the market can move against us. Saving 0.045% in fees is not worth risking the entire \$0.19 profit from the round trip.

**C. Taker market order (chosen)**
Fill immediately as taker to lock in the existing profit.

```
on_tp_post_only_rejected(order):
  place_market_order(order.side, order.amount)
  // Taker fill. Profit is already there — lock it in.
```

Why C is the best option: the profit already exists and the risk is reversal. If the long TP sell at \$60,000 was rejected because price is at \$60,020, placing a maker order (options A or B) risks price reversing below \$60,000 before fill — the profit disappears and the position could turn into a loss. A taker order guarantees the profit is captured immediately.

Fee cost: \$0.027 extra per event (taker \$0.033 vs maker \$0.006 on 0.001 BTC at \$60,000). This scenario is extremely rare — price must move a full \$200 grid spacing in the milliseconds between entry fill detection and TP placement. Expected frequency: 0-1 times per week. Total impact: under \$0.50/month.

#### Summary

| Order type | On rejection | Options rejected | Why |
|---|---|---|---|
| Entry (open position) | C: Maker retry at `current_price ± 1` | A: taker (chases price), B: maker at original (stale) | Better entry, patience = grid logic |
| Take-profit (close position) | C: Taker market order | A: maker at original (reversal risk), B: maker at current (reversal risk) | Profit exists now, risk of reversal |

Action: update `docs/trading-bot-strategy.md` to add post-only rejection handling (with option analysis) to the core grid algorithm section. Update `docs/trading-bot-strategy-context.md` to add the reasoning behind the chosen options (why maker for entries, why taker for TPs) and the fee/frequency analysis.

---

## Strategy Improvements

### 6. Volatility-adaptive grid spacing (ATR-based)

Status: `not reviewed`

Fixed \$200 spacing works in average conditions. But BTC daily range varies from 1% (\$680) to 10% (\$6,800). During low-volatility periods, \$200 spacing means zero fills for days. During high-volatility periods, \$200 means fills every few minutes with high exposure.

Improvement: use ATR (Average True Range) on the same 15-min candles to dynamically adjust spacing:

```
base_spacing = \$200
atr_14 = 14-period ATR on 15-min candles
volatility_ratio = atr_14 / historical_avg_atr
adjusted_spacing = base_spacing x volatility_ratio

Clamp to [min_profitable_spacing, max_spacing]
where min_profitable_spacing = \$120 (0.02% round-trip fee threshold)
```

This makes the grid breathe with the market — tighter during quiet periods (more fills), wider during volatile periods (less exposure).

Tradeoff: adds complexity, requires ATR calculation, and grid adjustment during live trading means cancelling/replacing orders. Worth evaluating via simulation first.

Action: model as a simulation parameter sweep. Compare fixed spacing vs ATR-adjusted spacing across volatility regimes.

---

### 7. Maximum net exposure limit

Status: `not reviewed`

The grid can accumulate positions on one side between rebalance checks. The gradual shift at "1 level beyond" shifts the grid but doesn't close positions. Between 0 and 2 levels beyond, positions accumulate unchecked.

Example: price slowly drifts up through all 3 short levels over 2 hours (but never more than 1 level beyond per 15-min candle). Result: 3 short positions, 0 long positions — a fully directional bet, not a grid.

Add a rule: if net exposure (longs minus shorts) exceeds 2 levels worth, immediately reduce the overweight side regardless of candle timing. This is different from the flash crash detector (3+ level gap) — this triggers on position imbalance, not price gap.

Action: define the net exposure limit and add it to the fast-loop checks.

---

### 8. Circuit breaker Level 2 taker cost optimization

Status: `not reviewed`

Level 2 says "close all positions -> bot pauses 24h." Closing all positions during high volatility means market/taker orders. With 6 positions:

```
6 x 0.001 BTC x \$68,000 x 0.055% = ~\$2.24 in taker fees
```

Not huge at \$100 account, but it adds to the loss that already triggered the circuit breaker.

Alternative: hedge-lock everything (one taker order for the net exposure) and then unwind via maker orders over the next 15-60 minutes. This preserves the pause behavior while minimizing taker cost.

Tradeoff: more complex, takes longer to fully close. But saves \$1-2 in fees during an already-losing moment. Evaluate whether the added complexity is worth it at \$100 account size.

Action: decide after initial implementation. May be premature optimization at \$100.

---

### 9. Profit-taking / compounding trigger automation

Status: `not reviewed`

The scaling guide says to tighten grid as account grows (\$200 -> \$150 -> \$100 spacing) but doesn't define when or how. Is it manual? Automatic?

The bot should have a periodic check (daily or weekly) that compares current account balance against scaling thresholds and notifies or auto-adjusts grid parameters. Without this, the bot runs at \$200 spacing long after the account has grown past the \$200 threshold.

Options:
- **Manual:** bot sends Telegram notification when account crosses a threshold, human adjusts config
- **Semi-auto:** bot proposes new parameters, human confirms
- **Auto:** bot adjusts parameters automatically within pre-defined bounds

Manual is safest for v1. Auto is the goal for v2.

Action: implement manual notification in v1. Flag auto-adjustment as a v2 feature.

---

## Parameters for Simulation Validation

### 10. EMA period is unvalidated

Status: `not reviewed`

EMA(50) on 15-min candles = 12.5 hours of lookback. This was chosen qualitatively ("SMA too slow, DEMA too fast, EMA sweet spot"). The Monte Carlo simulation should sweep EMA periods (20, 30, 50, 75, 100) and compare grid profitability across regimes.

It's possible that a different period works better, or that the optimal period varies by volatility regime (shorter during trending, longer during choppy).

Action: flag as simulation parameter. Do not change the default until simulation data supports it.

---

### 11. Hedge lock window (48h) is arbitrary

Status: `not reviewed`

The 48-hour hedge lock expiry was set without empirical backing. The context doc acknowledges this as an open question.

The simulation should sweep 12h, 24h, 48h, and 72h windows and measure:
- Recovery probability at each window length
- Funding cost at each window length (very small per the data below, but still worth measuring)
- Optimal window = maximizes (recovery_probability x recovery_amount - funding_cost)

Action: flag as simulation parameter.

---

## Corrected Assumptions

### 12. WhiteBit BTC-PERP funding rates are negligible

Status: `accepted` (factual correction)

Original assumption in discussion: ~0.01% per 8h period. This was wrong by 10-50x.

Actual WhiteBit BTC-PERP funding rate data (Feb 26 - Mar 4, 2026):

```
Typical range:    +/-0.0001% to +/-0.008% per 8h period
Highest observed: 0.008203% (1 Mar 2026 16:00 UTC)
Most common:      0.0003% to 0.003%
Negative rates:   frequent (shorts pay longs, partially cancel over time)
```

Recalculated impact for hedge lock (48h = 6 funding periods):
```
Worst case: 6 x 0.008% x 0.001 BTC x \$68,000 = \$0.033 per position
With 6 positions: \$0.20 total over 48 hours
```

This is negligible — roughly equal to the profit from a single grid fill (\$0.19). Funding rates are NOT a meaningful cost factor for this strategy on WhiteBit.

Key implication: the hedge lock tradeoff is not about funding cost. The real cost of hedge lock is opportunity cost — margin is tied up in locked positions and can't be used for grid trading. The 48h window decision should be driven by recovery probability, not funding fees.

---

## Minor Items

### 13. Exchange maintenance window handling

Status: `not reviewed`

WhiteBit has scheduled and unscheduled maintenance. During maintenance:
- WebSocket connections drop (handled by reconnection logic)
- REST API may return errors (handled by rate limiting / retry)
- Open orders may be cancelled by the exchange

The last point is dangerous — if the exchange cancels all orders during maintenance, the bot restarts with no grid and potentially open positions with no take-profits. This overlaps with the restart reconciliation problem (item 3).

Action: address as part of restart reconciliation design.

---

### 14. Orderbook depth awareness at scale

Status: `not reviewed`

At 0.001 BTC per level (\$68 per position), the bot is invisible in the orderbook. As the account scales to \$500+ with larger positions, orders become visible to other bots that may front-run or adversarially interact.

Not relevant at \$100 but should be noted in the scaling guide as a consideration at \$2,000+.

Action: add a note to the scaling guide. No implementation needed for v1.