# bridging-go

We have two datacenters, one is private and another is cloud. Mainly for security/privary concerns, we are not allowed to have inbound connections to our private DC. Also we should not transfer sensitive data to cloud. So we come up with this solution: cloud serves the website frontend (nodejs like), a bridge forwards requests from frontend / replies from private DC.

## Use cases

1. You are going to serve a public website, but critical business services are running on-premise, and you are not allowed to have inbound connections to on-premise infrastructure.
2. You are hosting a website, and you want to use your own PC to save cloud bill.

## How it works

Bridge is the one running on cloud, Gateway running on private DC.
Bridge listens to a special websocket `/bridge` which is supposed to be connected by Gateway.
Bridge wraps HTTP and other websocket requests, forwards them over `/bridge` to Gateway.
Gateway unwraps the requests and routes them to the correct downstream services, routing is done with a special header `bridging-base-url` from frontend.
Gateway will then forward reply from downstream services to Bridge, who sends the reply to client finally.

### HTTP

Add `bridging-base-url` to HTTP headers.
`"bridging-base-url"=<the netloc(IP/port) of the service in private DC>`

### WebSocket

Add `bridging-base-url` to query parameters.
URL: `ws[s]://<public IP/port>/<path>?bridging-base-url=<IP/port of the service in private DC>`

## Run

First, start Bridge on cloud DC:

```
go run ./cmd/bridge
```

An env file sample can be found at [.env.sample](cmd/bridge/.env.sample).

After Bridge is up on cloud, start the Gateway on private DC:

```
go run ./cmd/gateway <config.json>
```

A config file sample can be found at [sample.json](cmd/gateway/sample.json).

- Make sure `bridge_token` in Gateway config file is the same as `BRIDGE_TOKEN` in env file.
- `whitelist` configures the resources on private DC that can be accessed on cloud.

## Securities

1. Gateway implements firewall (whitelists).
2. `/bridge` is protected with private token.

## Benefits

1. No additional deployment or changes to existing business services, no troublesome migration of these services to another DC.
2. Data flowed in/out to/from private DC is precisely managed.
3. Minimal requirement/effort on maintaining the cloud DC.

## Limitations

1. It bridges HTTP and websocket only, which is its nature and by design.
2. Messages over `/bridge` are gzipped, but duplicate traffic (into cloud) could become the bottleneck of this setup.
