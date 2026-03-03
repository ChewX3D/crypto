# BTC/USDT Futures Grid Trading Bot — Strategy Document

## Overview

A hedged grid trading bot for BTC/USDT perpetual futures on WhiteBit exchange, written in Go. The bot places maker-only limit orders above and below current price, profiting from BTC's natural price oscillations. It uses hedge mode to hold long and short positions simultaneously, an EMA trend filter to bias grid direction, and a multi-layer risk management system to survive black swan events.

**One-sentence summary:** Place buy orders below current price and sell orders above it, profit from BTC bouncing around, and protect yourself if it moves too far in one direction.

## Exchange & Fees

- **Exchange:** WhiteBit
- **Pair:** BTC/USDT perpetual futures
- **Mode:** Hedge mode (long and short positions simultaneously)
- **Maker fee:** 0.01% (our edge — most exchanges charge 0.02-0.025%)
- **Taker fee:** 0.055% (avoid at all costs)
- **All orders must be `post_only: true`** to guarantee maker fee

## Account Parameters

| Parameter | Starting Value | Notes |
|---|---|---|
| Account size | $100 | Only keep trading capital on exchange |
| Leverage | 5x | Buying power: $500 |
| Grid spacing | $200 | Tighten as account grows |
| Grid levels | 3 per side (6 total) | Increase as account grows |
| Position size | ~0.001 BTC per level | Adjust to stay within margin |

### Scaling Guide

```
$100 account  → $200 grid spacing, 3 levels per side
$200 account  → $150 grid spacing, 4 levels per side
$500 account  → $100 grid spacing, 5 levels per side
$2,000+ account → consider adding ETH/USDT as second grid
$10,000+ account → diversify across exchanges and strategies
```

## Core Strategy: Hedged Grid

### Grid Setup

Place maker limit orders at fixed intervals above and below current price:

```
Example: BTC at $60,000, grid spacing $200, 3 levels each side

SHORT side (limit sells above price):
  $60,200 — open short (take profit at $60,000)
  $60,400 — open short (take profit at $60,200)
  $60,600 — open short (take profit at $60,400)

LONG side (limit buys below price):
  $59,800 — open long (take profit at $60,000)
  $59,600 — open long (take profit at $59,800)
  $59,400 — open long (take profit at $59,600)
```

### How It Profits

BTC oscillates → grid orders fill on both sides → each round trip captures the grid spacing as profit. More volatility = more fills = more profit.

### Profit Per Fill

```
Grid spacing: $200
Position size: 0.001 BTC
Gross profit per round trip: $0.20
Maker fees (0.01% × 2 sides): ~$0.012
Net profit per round trip: ~$0.19
```

## Trend Filter: EMA(50) on 15-Minute Candles

The grid runs with a directional bias based on a 50-period EMA calculated on 15-minute candlestick closes.

### Logic

```
Price > EMA → Bullish bias
  Long grid:  3 levels (full allocation)
  Short grid: 2 levels (reduced)

Price < EMA → Bearish bias
  Short grid: 3 levels (full allocation)
  Long grid:  2 levels (reduced)

Price ≈ EMA → Neutral
  Both sides: equal levels
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
  At -$59,000: hedge lock with opposite position (maker order)
  Tries limit orders first, intelligent exit

Layer 2 — Exchange stop-loss (dumb but reliable):
  At -$58,800: market sell order sitting in exchange matching engine
  Fires only if bot fails to act (API down, bot crash)

Layer 3 — Low leverage (airbag):
  5x leverage, liquidation at ~$48,000
  Survives even if both Layer 1 and Layer 2 fail temporarily
```

### Hedge Lock Mechanism (Black Swan Protection)

Instead of a hard stop-loss that realizes a loss permanently, the bot opens an equal opposite position to freeze the loss.

```
Normal stop-loss:
  Price hits SL → market sell → loss is REALIZED and FINAL
  If price bounces back, you already sold. Missed recovery.

Hedge lock:
  Price hits threshold → open equal short (maker order)
  Loss is FROZEN but NOT realized
  Wait up to 48 hours for bounce:
    - Bounce happens → close short, ride long back up (potential full recovery)
    - No bounce in 48h → close both, accept loss (same outcome as stop-loss)
```

**Hedge lock wins when:** price bounces (most common for BTC), avoids selling at worst moment, avoids taker fees.

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
┌─────────────────┐
│  WebSocket Feed  │
│  BTC/USDT price  │
└────────┬────────┘
         │
┌────────▼────────┐
│  Trend Filter   │
│  EMA(50) 15min  │
└────────┬────────┘
         │
   ┌─────┼─────────────┐
   │     │              │
┌──▼──┐ ┌▼─────┐  ┌────▼─────┐
│Long │ │Short │  │ Circuit  │
│Grid │ │Grid  │  │ Breaker  │
│Maker│ │Maker │  │ Monitor  │
└──┬──┘ └┬─────┘  └────┬─────┘
   │     │              │
   └─────┼──────────────┘
         │
┌────────▼────────┐
│   Hedge Lock    │
│  (if triggered) │
└────────┬────────┘
         │
┌────────▼────────┐
│  PnL Tracker    │
│  + Rebalancer   │
└─────────────────┘
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

### Grid Rebalancing

When price moves significantly, the grid needs to re-center:

- If price moves above all short grid levels → cancel stale long orders, place new grid around current price
- If price moves below all long grid levels → cancel stale short orders, place new grid around current price
- Rebalance only on EMA update (every 15-min candle) to avoid excessive API calls

## Expected Performance

### Per-Trade Metrics

```
Win rate per trade: ~85%
Profit per winning round trip: ~$0.19 (at $100 account)
Loss per losing trade: capped by circuit breakers
```

### Monthly Estimates (not compounding)

```
Conservative (low volatility):  $18-$36/month  (MRR 20-35%)
Average month:                  $48-$90/month  (MRR 50-90%)
Great month (high volatility):  $112-$187/month (MRR 110-180%)
Bad month (black swan):         -$5 to +$10    (MRR -5% to +10%)
```

### Growth Projection (with compounding)

```
Month 1:  $100  → $150
Month 2:  $150  → $210
Month 3:  $210  → $180  (bad month)
Month 4:  $180  → $260
Month 5:  $260  → $370
Month 6:  $370  → $320  (pullback)
Month 7:  $320  → $450
Month 8:  $450  → $600
Month 12: ~$800-$1,500

Target: $100 → $500 in 8-12 months
Realistic ARR: 400-500% (accounting for bad months)
```

### Important Caveats

- These estimates assume average BTC volatility. Low-volatility periods will underperform.
- You will have losing months. The strategy wins in aggregate, not on every trade.
- Percentage returns decrease as account size grows (fewer fills relative to capital, orderbook depth limits).
- Past BTC volatility patterns may not continue.

## Three Outcomes

```
1. BTC bounces around (70% of days) → grid fills both sides → profit ✓
2. BTC trends slowly (20% of days)  → trend filter helps, reduced profit
3. BTC crashes/pumps hard (10%)     → hedge lock + circuit breaker → small locked loss
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