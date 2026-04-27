# Interview Guide

## One Sentence

This is a small Go microservice order system focused on asynchronous stock deduction, order state transitions, message idempotency, and eventual consistency.

## How To Explain The Project

Start from the business flow, not from the technology stack:

```text
After login, a user creates an order through the gateway.
The order service saves the order as pending_stock and sends a stock deduction message.
The product service consumes the message, deducts stock, and sends the result back.
The order service consumes the result and changes the order to confirmed or failed.
The user can query the final order status.
```

## Why The Order Starts As pending_stock

Creating an order only means the system has accepted the request. Stock deduction is asynchronous, so the system cannot immediately say the order is finally confirmed.

That is why the first status is:

```text
pending_stock
```

Then it becomes:

```text
confirmed
```

or:

```text
failed
```

## Why RabbitMQ Is Used

RabbitMQ decouples order creation from stock deduction.

Without MQ:

```text
order-service -> calls product-service synchronously -> waits for stock result
```

With MQ:

```text
order-service -> writes order -> publishes message -> returns order_id
product-service -> consumes message -> deducts stock -> publishes result
order-service -> consumes result -> updates status
```

This makes the order API faster and keeps service responsibilities clearer.

## How Product Query Avoids Cache Penetration

The product query path uses both a Bloom filter and null cache.

```text
1. Bloom filter checks whether a product ID might exist.
2. Redis product cache checks normal cached data.
3. Redis null cache blocks repeated queries for missing products.
4. MySQL is only queried when the request passes the first three checks.
```

This keeps repeated invalid product requests away from MySQL and makes the cache layer more reliable.

## How The System Avoids Overselling

The product service uses Redis Lua to atomically check and deduct cached stock:

```text
if stock >= 1:
  stock = stock - 1
else:
  reject
```

Then MySQL is updated with a condition:

```sql
WHERE id = ? AND stock > 0
```

Redis handles fast atomic pre-deduction. MySQL is the persistent safety layer.

In the code, Redis Lua returns three possible values:

```text
1  -> cached stock was deducted
0  -> insufficient stock
-1 -> stock cache missing
```

The product service handles these branches separately:

```text
1  -> update MySQL and publish success
0  -> publish failed result
-1 -> reload stock cache and requeue the message
```

This makes the stock flow easier to explain than simply saying "Redis deducts stock".

## How Duplicate Messages Are Handled

RabbitMQ may deliver the same message more than once. The product service uses Redis idempotency keys:

```text
processed_order:<order_id>
```

If this key already exists, the product service ignores the duplicate message and does not deduct stock again.

The key is written after the stock result is published. This avoids marking a message as processed before the order service has a chance to receive the final result.

## What Consul Does

The internal services register themselves with Consul. The gateway reads service addresses from Consul instead of hardcoding all gRPC addresses.

This is a basic service discovery pattern.

## What Sentinel Does

Sentinel is used at the gateway to limit the create-order API. It protects downstream services from too many order requests in a short time.

This is a supporting feature, not the core project story.

## Project Limitations

Be honest about what is not included:

- The project has no payment flow.
- The order state machine is intentionally small.
- The order service is lightly layered, and the product service has extracted stock and MQ concepts but is not fully layered yet.
- There are only minimal unit tests.
- Services are not fully containerized yet.

This is acceptable because the project goal is to clearly show one complete microservice business flow.

## Strong Summary

Use this summary in interviews:

```text
The main design point is that order creation and stock deduction are separated.
The order service owns the order lifecycle, the product service owns stock, and RabbitMQ connects them asynchronously.
The order status makes the asynchronous result visible to users.
Redis Lua and MySQL conditional updates protect stock deduction, and Redis idempotency keys prevent duplicate message processing.
```
