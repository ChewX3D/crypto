# Conversation Context — Trading Bot Project

This document captures all decisions, reasoning, and technical details from the initial strategy discussion. Use this as context when working on implementation.

## Project Summary

Building a BTC/USDT perpetual futures grid trading bot in Go for the WhiteBit exchange. The bot uses hedge mode, a trend filter, and multi-layer risk management. The full strategy is documented in `STRATEGY.md`.

## Developer Profile

- Senior Backend Golang Engineer
- Trading with ~\$100, goal is to reach \$500 without topping up
- Ready for risks but not for losing everything
- Prefers maker-only orders (0.01% fee on WhiteBit)
- Will use Claude Code for implementation

## Key Decisions Made and Why

### Why BTC/USDT Only

User's reasoning: BTC events affect every crypto pair anyway. ETH/USDT and others correlate heavily with BTC during crashes. Running one pair simplifies everything — one orderbook to understand, one WebSocket stream, one set of grid parameters. Diversification across pairs adds complexity without meaningful decorrelation during black swans.

### Why Hedge Mode

Normal mode only allows long OR short. Flipping requires closing one and opening the other — two taker fills during fast moves. Hedge mode holds both simultaneously, enabling the hedge lock mechanism (freeze losses instead of realizing them) and independent management of both sides.

### Why Maker-Only Orders

WhiteBit fees: 0.01% maker vs 0.055% taker. That's 5.5x difference. On a grid bot doing 20+ trades/day, this is the difference between profitable and not. All orders must use `post_only: true` so they get rejected rather than filled as taker.

This low maker fee is a genuine competitive edge — it allows tighter grid spacing (\$100-200) that would be unprofitable on Binance/Bybit/OKX (0.02% maker). Most YouTube/forum advice about minimum grid spacing assumes higher fees and doesn't apply here.

### Why 5x Leverage

\$100 account needs leverage just to meet minimum order sizes. 5x gives \$500 buying power, enough for 3 grid levels each side at ~\$60/position. Higher leverage (10x+) increases liquidation risk during crashes. At 5x with BTC at \$60,000, liquidation is around \$48,000 — huge buffer.

### Why EMA(50) on 15-Minute Candles

- SMA is too slow — confirms trends too late, grid accumulates wrong-side positions
- DEMA/HMA are too fast — flip grid bias constantly, causing unnecessary order churn
- EMA is the sweet spot — fast enough to catch real trends, smooth enough to ignore noise
- 15-minute timeframe balances responsiveness with stability
- WhiteBit doesn't provide calculated indicators — only raw kline data. EMA is calculated locally. It's 3 lines of Go code.

### Why Grid Trading (Not Other Strategies)

For a \$100 account on a single exchange:
- DCA: not really trading, just accumulation
- Cross-exchange arbitrage: needs accounts on multiple exchanges, more capital
- Funding rate arbitrage: returns too small at \$100
- Market making: needs deep capital
- Grid trading: works well with small capital, profits from volatility (which BTC has plenty of), simple to implement, pairs well with hedge mode

### Why Custom Bot Instead of Existing Platforms

At \$100 account size:
- 3Commas/Pionex charge \$50+/month — that's 50% of capital annually just for the platform
- No existing bot supports the hedge lock mechanism (freeze loss with opposite position instead of stop-loss)
- Existing bots often fill as taker despite claiming limit orders
- No existing bot supports the specific circuit breaker logic (pause 24h, pause 7d, etc.)
- No dynamic trend filter adjusting grid bias
- Developer is a senior Go engineer — building the bot is a weekend project

## Strategy Details Not in STRATEGY.md

### Hedge Lock vs Stop-Loss (Full Reasoning)

Stop-loss realizes loss permanently. If BTC bounces back, you already sold and missed recovery.

Hedge lock opens equal opposite position, freezing loss without realizing it. You then have up to 48 hours to decide:
- If bounce: close the hedge, ride original position back to profit
- If no bounce in 48h: close both, accept loss (same outcome as stop-loss + small funding fees)

Worst case for hedge lock ≈ stop-loss result + minor funding fees.
Best case for hedge lock = full recovery and profit.

Hedge lock wins specifically for BTC because BTC almost always bounces to some degree after crashes. For random altcoins that can go to zero, stop-loss would be better.

The hedge lock uses a TAKER order (market) in emergency — this is the one exception to maker-only. Acceptable because it happens rarely and the cost is far less than the loss it prevents.

### Rebalancing Logic (Full Detail)

Grid becomes "dead" when price trends away — all orders on one side are stale. The bot handles this with two processing speeds:

**Fast loop (every WebSocket tick):**
- Monitor order fills → place take-profits
- Monitor circuit breaker thresholds
- Detect flash crash (price gaps 3+ grid levels) → emergency rebalance immediately

**Slow loop (every 15-minute candle close):**
- Update EMA
- Check if price is outside grid
- If 1 level beyond: shift grid by 1 level (gradual, no position closes)
- If 2+ levels beyond: full rebalance (cancel stale orders, hedge lock or close losing positions, place new grid)

Why not react to every tick: price spikes beyond grid and comes back within seconds are common. Rebalancing on every tick would cause constant unnecessary order cancellations. Tying to 15-min candles naturally filters noise.

### Grid Spacing Scaling

The constraint at \$100 is margin, not fees:

```
$100 account, 5x leverage, $500 buying power
BTC at $60,000, position 0.001 BTC = $60 per position

$100 grid, 5 levels each side = 10 × $60 = $600 → can't afford
$200 grid, 3 levels each side = 6 × $60 = $360 → fits
$300 grid, 3 levels each side = 6 × $60 = $360 → comfortable
```

Tighten grid as account grows:
- \$100 account → \$200 spacing, 3 levels
- \$200 account → \$150 spacing, 4 levels
- \$500 account → \$100 spacing, 5 levels

Minimum profitable spacing at 0.01% maker: ~\$120 (0.02% round trip fee on ~\$60,000 BTC). Anything above this is profitable per fill.

### Why Strategy Won't Scale Linearly Past \$10k

Not a strategy problem — a market microstructure problem:
1. Order visibility: 0.001 BTC is invisible, 1 BTC is visible to other bots who front-run
2. Orderbook depth: on WhiteBit, your large orders become a significant part of the book
3. Fill competition: fewer fills relative to capital at larger sizes
4. Exchange risk: too much capital on one exchange

Solution at scale: multiple pairs, multiple exchanges, multiple strategies. Not relevant until well past the \$500 goal.

### Performance Expectations

Win rate per trade: ~85%
Net profit per round trip: ~\$0.19 (at \$100 account, \$200 grid)
Fills per day: 8-15 average (varies with volatility)
Monthly return: 20-90% depending on volatility (not compounding)
Realistic path: \$100 → \$500 in 8-12 months with compounding

Bad months will happen. The strategy wins in aggregate across months, not on every trade or every month.

## Technical Implementation Notes

### WhiteBit API

- REST API for orders, balances, klines
- WebSocket for real-time price and kline streams
- Auth: API key + secret, HMAC-SHA512, nonce-based
- No official Go SDK — build REST/WS client from scratch
- Docs: https://docs.whitebit.com/
- Key endpoint for klines: `GET /api/v4/public/kline?market=BTC_USDT&interval=15m&limit=50`
- Must set `post_only: true` on all limit orders
- Check minimum order sizes for BTC/USDT futures before implementation

### Go-Specific Considerations

- Use goroutines for concurrent WebSocket streams
- `time.Ticker` for rate limiting API calls
- Strong typing for order structs (don't use map[string]interface{})
- SQLite or Postgres for state persistence (grid state, open positions, PnL history)
- Must survive restarts — on startup, read state from DB, reconcile with exchange positions
- Exponential backoff for WebSocket reconnection
- Paper trading mode: same code path, simulated order matching against real market data

### Order Types to Track

Each order needs metadata to distinguish its role:

```
GridOrder:       initial grid placement (buy below / sell above)
TakeProfitOrder: placed when grid order fills (sells the position at +1 spacing)
HedgeLockOrder:  emergency opposite position to freeze loss
StopLossOrder:   exchange-side safety net (wider than bot's threshold)
```

### Three-Layer Stop-Loss System

```
Layer 1 — Bot logic:     smart exit (hedge lock, maker orders). First to act.
Layer 2 — Exchange SL:   dumb market order sitting in matching engine. Fires if bot fails.
Layer 3 — Low leverage:  liquidation price far away. Survives if both above fail.
```

Layer 2 (exchange SL) is MORE reliable than Layer 1 because it's already in the matching engine. Layer 1 is smarter but requires API to be reachable. Both together cover each other's weaknesses.

### Circuit Breaker Levels

```
Level 1: Unrealized PnL < -3% → stop new positions, hedge lock open ones
Level 2: Daily PnL < -5% → close all, pause 24h
Level 3: Weekly drawdown > 12% → pause 7 days, Telegram alert
```

## Backtesting Approach

### Data Source

WhiteBit provides kline (candle) data down to 1-second resolution via both REST and WebSocket APIs. Available intervals: 1s, 2s, 3s, 4s, 5s, 6s, 10s, 12s, 15s, 20s, 30s, 1m, 2m, 3m, 5m, 15m, 30m, 1h, 2h, 4h, 6h, 12h, 1d, 2d, 3d, 1w, 1M.

At 1-second resolution, BTC typically moves \$0.10-\$5.00 per candle. With \$200 grid spacing, price almost never crosses a grid level more than once within a single 1-second candle. This makes 1s candles accurate enough for grid bot backtesting without needing tick-level trade data.

Data volume: one day = 86,400 candles, one month ≈ 2.6M, one year ≈ 31.5M. At ~100 bytes per candle struct, one year ≈ 3 GB in memory — manageable in Go.

### Pessimistic Candle Assumptions

Even with 1-second candles, apply pessimistic assumptions to avoid overfitting to historical data:

- **One fill per level per candle:** if a candle's high/low range crosses a grid level, count it as exactly one fill, never multiple. In reality price might briefly touch a level and reverse before the order could fill.
- **Worst-case fill ordering:** when a candle crosses multiple grid levels, assume the fill sequence that produces the least profit. For example, if a candle touches both a long entry and its take-profit, assume only the entry filled (not the round trip).
- **No partial fills:** treat every fill as either fully executed or not executed at all. Do not assume partial fills that would inflate fill counts.
- **Fees on every fill:** always deduct the full maker fee (0.01%) on every fill, even though some real fills might benefit from fee rebates or promotions.

If the strategy is profitable under these pessimistic rules, it will be profitable in live trading. If it's only profitable under optimistic assumptions (counting multiple fills per candle, assuming best-case ordering), the real-world performance will disappoint.

## Open Questions for Implementation

- What are WhiteBit's exact minimum order sizes for BTC/USDT futures?
- What are WhiteBit's API rate limits? (need to size the rate limiter)
- What WebSocket channels are available for order fill notifications?
- Does WhiteBit support `post_only` on futures orders? (verify in docs)
- What's the funding rate payment interval for BTC/USDT perps on WhiteBit?
- How does WhiteBit handle hedge mode via API? (separate position IDs? dual side parameter?)

## Implementation Priority

1. WhiteBit API client (REST + WebSocket + auth)
2. Core grid loop (place orders, detect fills, place take-profits, restore grid)
3. State persistence (survive restarts)
4. EMA trend filter
5. Rebalancing logic (two-speed processing)
6. Circuit breakers
7. Hedge lock mechanism
8. Exchange-side stop-loss placement
9. Telegram notifications
10. Paper trading mode
11. Backtesting against historical kline data
