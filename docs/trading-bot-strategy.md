# BTC/USDT Futures Grid Trading Bot — Strategy Document

## Overview

A hedged grid trading bot for BTC/USDT perpetual futures on WhiteBit exchange, written in Go. The bot places maker-only limit orders above and below the grid anchor, profiting from BTC's natural price oscillations. It uses hedge mode to hold long and short positions simultaneously, an EMA trend filter to bias grid direction, and a multi-layer risk management system to survive black swan events.

**One-sentence summary:** Place buy orders below the grid anchor and sell orders above it, profit from BTC bouncing through the grid levels, and protect yourself if price moves outside the grid range.

## Exchange & Fees

- **Exchange:** WhiteBit
- **Pair:** BTC/USDT perpetual futures
- **Mode:** Hedge mode (long and short positions simultaneously)
- **Maker fee:** 0.01% (our edge — most exchanges charge 0.02-0.025%)
- **Taker fee:** 0.055% (avoid at all costs)
- **All orders must be `post_only: true`** to guarantee maker fee

## Grid Terminology

| Term | Meaning |
|---|---|
| **Grid anchor** | The center price around which the grid is built. Set on startup to current market price. Updated by the rebalancing algorithm when price drifts outside the grid range. No order is placed at the anchor itself — it's a reference point, not a level. |
| **Grid step** | The fixed price distance between adjacent grid levels (e.g. \$200). |
| **Grid level** | A specific price where an order sits. Each level is `anchor ± (N × step)` where N is the level number (1, 2, 3...). |
| **Long levels** | Grid levels below the anchor where buy (open long) orders are placed. Labeled L1 (closest to anchor), L2, L3, L4, L5. |
| **Short levels** | Grid levels above the anchor where sell (open short) orders are placed. Labeled S1 (closest to anchor), S2, S3, S4, S5. |
| **Grid range** | The total price span from the lowest long level to the highest short level. With 5 levels per side and \$200 step: range = 10 × \$200 = \$2,000. |
| **Round trip** | A complete entry fill + take-profit fill on the same grid level. One round trip = one profit capture. |

## Account Parameters

| Parameter | Starting Value | Notes |
|---|---|---|
| Account size | \$500 | Minimum practical amount for trailing grid |
| Leverage | 5x | Buying power: \$2,500 |
| Grid step | \$200 | Tighten as account grows |
| Grid levels | 5 per side (10 total) | Increase as account grows |
| Position size | 0.002 BTC per level | Adjust to stay within margin |

Margin budget at \$500:
- Initial grid: 10 positions × 0.002 BTC × \$68,000 / 5x = \$272 (54% of account)
- Trailing buffer: remaining \$228 supports ~8 additional trailing positions
- Unrealized loss buffer: absorbed by the remaining margin

### Scaling Guide

```
$500 account   → $200 grid step, 5 levels per side, 0.002 BTC
$1,000 account → $150 grid step, 7 levels per side, 0.002 BTC
$2,000 account → $100 grid step, 10 levels per side, 0.003 BTC
$5,000+ account → consider adding ETH/USDT as second grid
$10,000+ account → diversify across exchanges and strategies
```

## Core Strategy: Hedged Grid

### Grid Setup

On startup, the bot sets the grid anchor to the current market price and places maker limit orders at each grid level:

```
Example: grid anchor = $68,000, grid step = $200, 5 levels per side

Short levels (limit sells above anchor):
  S5  $69,000 — open short (take profit at $68,800)
  S4  $68,800 — open short (take profit at $68,600)
  S3  $68,600 — open short (take profit at $68,400)
  S2  $68,400 — open short (take profit at $68,200)
  S1  $68,200 — open short (take profit at $68,000)

          --- $68,000 (grid anchor, no order here) ---

Long levels (limit buys below anchor):
  L1  $67,800 — open long (take profit at $68,000)
  L2  $67,600 — open long (take profit at $67,800)
  L3  $67,400 — open long (take profit at $67,200)
  L4  $67,200 — open long (take profit at $67,000)
  L5  $67,000 — open long (take profit at $66,800)
```

### How It Profits

BTC oscillates through the grid levels — entry orders fill on both sides, take-profits close them at the next level. Each completed round trip captures one grid step as gross profit.

### Profit Per Round Trip

```
Grid step: $200
Position size: 0.002 BTC
Gross profit per round trip: 0.002 × $200 = $0.40
Maker fees (0.01% × 2 fills): 2 × 0.01% × 0.002 × $68,000 = ~$0.027
Net profit per round trip: ~$0.37
```

### Core Grid Algorithm

```
on_startup:
  anchor = get_current_price()
  for i = 1 to NUM_LEVELS:
    place_buy(anchor - step * i)     // long levels
    place_sell(anchor + step * i)    // short levels

on_order_filled(order):
  if order.side == BUY:
    next_side = SELL
    next_price = order.price + step
  if order.side == SELL:
    next_side = BUY
    next_price = order.price - step

  if order.is_grid:
    // Entry filled → place take profit at the next level
    place_order(next_side, next_price, type=TAKE_PROFIT)
  if order.is_take_profit:
    // Take profit filled → restore the consumed grid level
    place_order(next_side, next_price, type=GRID)
```

The fill/take-profit cycle automatically restores consumed grid levels as long as price stays within the grid range. Each completed round trip returns the level to its original state with profit captured. The bot does not predict price direction — it profits from price oscillating through the grid levels.

When price drifts outside the grid range, the core algorithm can no longer generate fills. The rebalancing algorithm (see Grid Rebalancing section) handles this by trailing the grid to follow the price.

## Trend Filter: EMA(50) on 15-Minute Candles

The grid runs with a directional bias based on a 50-period EMA calculated on 15-minute candlestick closes.

### Logic

```
Price > EMA → Bullish bias
  Long levels:  5 (full allocation)
  Short levels: 3 (reduced)

Price < EMA → Bearish bias
  Short levels: 5 (full allocation)
  Long levels:  3 (reduced)

Price ≈ EMA → Neutral
  Both sides: 5 levels each
```

### Why EMA

- SMA is too slow — confirms trends too late
- DEMA/HMA are too fast — flip bias too often, causing unnecessary order churn
- EMA is the sweet spot for a trend filter — reacts fast enough to catch real trends, smooth enough to ignore noise

### EMA Calculation

```
multiplier = 2 / (period + 1)
EMA = (price - previous_EMA) × multiplier + previous_EMA
```

On startup: fetch 50 candles via REST API, seed EMA by iterating through them. Then update with each new 15-minute candle close from WebSocket.

### Data Source

WhiteBit API provides raw kline data:
```
GET /api/v4/public/kline?market=BTC_USDT&interval=15m&limit=50
Response: [timestamp, open, close, high, low, volume]
```

EMA is calculated locally — no exchange provides pre-calculated indicators.

## Risk Management

### Three-Layer Stop-Loss System

```
Layer 1 — Bot logic (smart):
  At circuit breaker threshold: hedge lock with opposite position
  Tries limit orders first, intelligent exit

Layer 2 — Exchange stop-loss (dumb but reliable):
  Wider threshold than Layer 1: market order sitting in exchange matching engine
  Fires only if bot fails to act (API down, bot crash)

Layer 3 — Low leverage (airbag):
  5x leverage, liquidation price far below current price
  Survives even if both Layer 1 and Layer 2 fail temporarily
```

### Hedge Lock Mechanism (Black Swan Protection)

Instead of a hard stop-loss that realizes a loss permanently, the bot opens an equal opposite position to freeze the loss.

```
Normal stop-loss:
  Price hits SL → market sell → loss is REALIZED and FINAL
  If price bounces back, you already sold. Missed recovery.

Hedge lock:
  Price hits threshold → open equal short (taker market order)
  Loss is FROZEN but NOT realized
  Wait up to 48 hours for bounce:
    - Bounce happens → close short, ride long back up (potential full recovery)
    - No bounce in 48h → close both, accept loss (same outcome as stop-loss)
```

**Hedge lock wins when:** price bounces (most common for BTC), avoids selling at worst moment.

**Stop-loss wins when:** price never recovers, extended sideways (funding fees accumulate).

**For BTC specifically, hedge lock has an edge** because BTC almost always bounces to some degree after crashes.

### Circuit Breakers

```
Level 1: Unrealized PnL < -3% of account
  → Stop opening new positions
  → Hedge lock any open positions

Level 2: Realized + Unrealized PnL < -5% daily
  → Close all positions
  → Bot pauses 24 hours

Level 3: Weekly drawdown > 12%
  → Bot pauses 7 days
  → Send alert (Telegram notification)
```

### Position Sizing Rules

- Max 1-2% loss per trade
- Max 5% daily drawdown
- Max 12% weekly drawdown
- Never risk more than you can lose without it affecting your life

## Architecture

```
┌──────────────────────────────────────────────────┐
│                WebSocket Feed                     │
│               BTC/USDT price                      │
└──────────┬───────────────────────┬───────────────┘
           │                       │
    FAST LOOP (every tick)   SLOW LOOP (15-min candle close)
           │                       │
    ┌──────▼──────┐         ┌──────▼──────┐
    │ Order Fill  │         │ Update EMA  │
    │ Monitor     │         │ Trend Filter│
    │ → place TPs │         └──────┬──────┘
    └──────┬──────┘                │
           │                ┌──────▼──────┐
    ┌──────▼──────┐         │ Rebalance?  │
    │ Circuit     │         │ Adjust bias │
    │ Breaker     │         │ Move anchor │
    │ Monitor     │         └─────────────┘
    └──────┬──────┘
           │
    ┌──────▼──────┐
    │ Flash Crash │
    │ Detector    │
    │ (5+ levels) │
    │ → emergency │
    │   rebalance │
    └──────┬──────┘
           │
    ┌──────▼──────┐
    │ Hedge Lock  │
    │ (if needed) │
    └──────┬──────┘
           │
    ┌──────▼──────┐
    │ PnL Tracker │
    └─────────────┘
```

## Technical Implementation Notes

### WhiteBit API

- REST API for order management, balance queries, kline data
- WebSocket API for real-time price feed and kline stream
- Auth: API key + secret with HMAC-SHA512 signing, nonce-based
- No official Go SDK — wrap REST/WS endpoints manually
- Docs: https://docs.whitebit.com/

### Key Implementation Details

- **Maker-only enforcement:** Set `post_only: true` on every order. Order gets rejected rather than filled as taker.
- **Rate limiting:** Use token bucket or `time.Ticker` to respect API limits.
- **WebSocket reconnection:** Auto-reconnect with exponential backoff. Connections will drop.
- **State persistence:** Track open positions and grid state in SQLite or Postgres. Must survive restarts.
- **Paper trading mode:** Run against real market data with simulated orders before going live.

### Grid Rebalancing (Trailing Grid)

When price drifts outside the grid range, orders on the far side become stale and the grid stops generating round trips. The bot trails the grid by shifting one level at a time in the direction of price movement, keeping existing positions and their take-profits alive.

#### Why Rebalancing Is Needed

```
Grid anchor: $68,000, grid step: $200. BTC trends to $70,000.

  S5  $69,000 — filled, TP waiting at $68,800 (unrealized loss)
  S4  $68,800 — filled, TP waiting at $68,600 (unrealized loss)
  S3  $68,600 — filled, TP waiting at $68,400 (unrealized loss)
  S2  $68,400 — filled, TP waiting at $68,200 (unrealized loss)
  S1  $68,200 — filled, TP waiting at $68,000 (unrealized loss)
      ------- price is $70,000 (outside grid range) -------
  L1  $67,800 — will never fill at this price
  L2  $67,600 — will never fill
  L3  $67,400 — will never fill
  L4  $67,200 — will never fill
  L5  $67,000 — will never fill

Grid is dead. All short positions are losing. Long levels are unreachable.
No round trips can complete.
```

#### Two Processing Speeds

The bot operates at two different speeds:

**Fast loop (every WebSocket tick):**
- Monitor order fills → place take-profits instantly
- Monitor circuit breaker thresholds → react instantly
- Detect flash crash / price gaps 5+ grid steps beyond the grid range → emergency rebalance instantly

**Slow loop (every 15-minute candle close):**
- Update EMA
- Check if price is outside the grid range
- Adjust long/short level allocation based on trend filter
- Trail grid if needed

This separation keeps the bot responsive to fills and emergencies while avoiding unnecessary grid restructuring on every price tick.

#### Trailing Grid Algorithm

When price moves beyond the grid range, the bot shifts the grid one level at a time instead of closing positions and rebuilding from scratch:

```
on_candle_close_15min(price):
  update_ema(candle.close)

  if price_inside_grid_range(price):
    // Price inside grid range. Only adjust trend bias.
    adjust_grid_bias(ema)
    return

  // Price outside grid range. Trail the grid.
  // Cancel the farthest order from current price (stale, won't fill).
  // Place a new order 1 step beyond the current grid edge near price.
  // Keep all existing positions and their TPs alive.
  trail_grid_one_step(direction)
```

**Emergency rebalance (on any WebSocket tick):**
```
on_price_update(price):
  levels_beyond = calculate_levels_beyond_grid_range(price)

  if levels_beyond >= 5:
    // Flash crash or pump. Don't wait for candle.
    // Trail multiple steps immediately to catch up.
    emergency_trail(price)
    return
```

#### Trailing Example — Price Drops \$1,200

```
Grid anchor: $68,000, grid step: $200, 5 levels per side

Starting grid:
  S5  $69,000    S4  $68,800    S3  $68,600    S2  $68,400    S1  $68,200
          --- $68,000 anchor ---
  L1  $67,800    L2  $67,600    L3  $67,400    L4  $67,200    L5  $67,000


Step 1: price drops to $67,800 → L1 fills. TP placed at $68,000.
Step 2: price drops to $67,600 → L2 fills. TP placed at $67,800.
Step 3: price drops to $67,400 → L3 fills. TP placed at $67,600.
Step 4: price drops to $67,200 → L4 fills. TP placed at $67,400.
Step 5: price drops to $67,000 → L5 fills. TP placed at $67,200.

All long levels filled. 5 open long positions. Price still dropping.

Step 6: 15-min candle closes at $66,900 (below grid range).
  → Cancel S5 at $69,000 (farthest from price, frees margin)
  → Place new buy at $66,800
  → Positions at $67,800-$67,000 stay open with TPs waiting

Step 7: price drops to $66,800 → new level fills. TP at $67,000.
  → Cancel S4 at $68,800
  → Place new buy at $66,600

Price stabilizes at $66,800.
Bot holds 6 longs with TPs stacked above.
If price bounces to $67,000 → closest TP fills, round trip profit $0.37.
If price bounces to $68,000 → all TPs fill, full recovery + profit.
```

#### Why Trailing Instead of Full Rebalance

Full rebalance (the old approach) closes all losing positions and rebuilds the grid. This realizes losses that might have recovered if the bot had waited. BTC frequently bounces after directional moves — trailing keeps positions alive to capture those bounces.

The tradeoff: if price never bounces, trailing positions consume margin and sit as unrealized losses. Circuit breakers (Level 1 at -3%, Level 2 at -5%) are the safety net — they force-close positions if losses grow too large.

#### Why Not React Instantly to Every Tick

```
BTC at $68,000:
  12:00:00 — price spikes to $66,750 (beyond L5)
  12:00:03 — price recovers to $67,100 (inside grid range)

  Instant reaction: trailed grid for no reason, wasted API calls
  Wait for 15-min candle: did nothing. Correct decision.
```

Tying trailing to 15-minute candle closes naturally filters out wicks and noise. The only exception is extreme gaps (5+ grid steps beyond the grid range) which require immediate trailing.

## Expected Performance

### Per-Trade Metrics

```
Win rate per round trip: ~85%
Profit per winning round trip: ~$0.37 (at $500 account, 0.002 BTC)
Loss per losing trade: capped by circuit breakers
```

### Monthly Estimates (not compounding)

```
Conservative (low volatility):  $44-$67/month   (MRR 9-13%)
Average month:                  $89-$133/month  (MRR 18-27%)
Great month (high volatility):  $167-$222/month (MRR 33-44%)
Bad month (black swan):         -$25 to +$10    (MRR -5% to +2%)
```

### Growth Projection (with compounding)

```
Month 1:  $500  → $590
Month 2:  $590  → $690
Month 3:  $690  → $630  (bad month)
Month 4:  $630  → $740
Month 5:  $740  → $870
Month 6:  $870  → $800  (pullback)
Month 7:  $800  → $950
Month 8:  $950  → $1,120
Month 12: ~$1,500-$2,500

Target: $500 → $2,000 in 12 months
Realistic ARR: 200-400% (accounting for bad months)
```

### Important Caveats

- These estimates assume average BTC volatility. Low-volatility periods will underperform.
- You will have losing months. The strategy wins in aggregate, not on every trade.
- Percentage returns decrease as account size grows (fewer fills relative to capital, orderbook depth limits).
- Past BTC volatility patterns may not continue.
- All estimates must be validated by backtesting before live trading.

## Three Outcomes

```
1. BTC oscillates within grid range (70% of days) → round trips complete on both sides → profit
2. BTC trends slowly (20% of days) → trailing grid follows price, TPs fill on bounces, reduced profit
3. BTC crashes/pumps hard (10%) → hedge lock + circuit breaker → small locked loss
```

## Pre-Launch Checklist

- [ ] Implement paper trading mode and run for 2+ weeks
- [ ] Verify all orders are maker-only (check fill reports)
- [ ] Test circuit breakers with simulated crashes
- [ ] Test hedge lock mechanism manually
- [ ] Verify WebSocket reconnection works reliably
- [ ] Set up Telegram alerts for circuit breaker triggers
- [ ] Confirm WhiteBit minimum order sizes for BTC/USDT futures
- [ ] Start with minimum position sizes, scale up gradually
- [ ] Keep only trading capital on exchange, rest in cold wallet
- [ ] Backtest against 1-second WhiteBit kline data with pessimistic fill assumptions