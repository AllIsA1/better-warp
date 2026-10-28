# Better WARP

A plugin for the [LucX-UI](https://github.com/AlexeyLCP/lucx-ui) plugin system: **Cloudflare WARP over AmneziaWG** as a protocol of the
subscription. Every subscriber gets WARP nodes in the same four forms as an AmneziaWG node, with settings, an account, and a check and
a scanner that find the quickest Cloudflare endpoint.

It needs **[Better Cores](https://github.com/AllIsA1/better-cores) 0.2.0 or newer** (the plugin system installs and turns it on first) and the
plugin system **host-9** or newer (the permission `peer:better-cores`, see below).

## WARP as an inbound protocol

On a panel build that has it (`v3.10.2-lucx.320-plugins.3` and later) **WARP is a protocol in Inbounds → Add inbound**, next to olcRTC, VLESS and the
rest. The inbound listens on nothing and runs nothing: it holds the settings of the WARP nodes (up to three endpoints, MTU, DNS, IPv6, keepalive,
the junk-packet preset and the AmneziaWG fields; the name of the nodes is the inbound's remark), and **the clients attached to it get those nodes** in their subscription. Several WARP inbounds
are several sets of nodes; a switched-off inbound gives nothing. An inbound with no endpoint of its own uses the endpoints this plugin keeps (the
scan's result).

The plugin reads the inbounds (`api:inbounds:read`, new in 1.1.0: confirm it when updating), registers a device for every active subscription that is
on one, and writes the lines. The WARP page then keeps the account (WARP+), the devices and the scanner; the rest of its settings are the inbounds'.
**With no WARP inbound** (or on a panel without the protocol, or if the permission is missing) the plugin works as before from its own page, as
described below.

## What a subscriber gets

For each endpoint (one to three) the subscription carries one node, as three lines in the order the subscription page groups them:

1. the older `amneziawg://<key>@host:port?…#name` link,
2. `vpn://…` (Amnezia's own),
3. `amneziawg://<base64url config>#name` (the config itself, which INCY imports).

The panel's subscription page shows them as **one row with a switch** (`vpn://`, `amneziawg://`, the older link, `.conf`; the `.conf` is read out
of the `vpn://` link), exactly as for an AmneziaWG node. The same lines go into the raw subscription, so a client that fetches it gets them.
Nothing is added to a switched-off, expired or out-of-traffic client, as in the panel.

The traffic goes from the client **straight to Cloudflare**. It does not pass this server, so the panel's traffic counters and limits do not
see it; the subscription's expiry and switch-off apply to the lines, not to a config someone already imported.

## The WARP protocol

WARP is plain WireGuard to Cloudflare's endpoints. What AmneziaWG adds that Cloudflare accepts is **junk packets before the handshake**
(`Jc`, `Jmin`, `Jmax`), which a plain WireGuard server ignores; they get the handshake past networks that drop the usual WireGuard one.
`S1`, `S2` and `H1`–`H4` other than their defaults make the endpoint drop the handshake, so the presets only set the junk (Standard: `Jc 4`,
`40–70`; Light; Strong) and the other fields are in an "advanced" block with a warning. The defaults are the config that is known to work:
MTU 1280 (WARP's own), Cloudflare's DNS, keepalive 25, `0.0.0.0/0, ::/0`, the endpoint `engage.cloudflareclient.com:2408`.

## Account

- **Main account**: a free anonymous WARP device registered from this server (Account → Register). The checks and scans use it; in shared
  mode every subscription gets it. A **WARP+ license key** can be applied to it (the key's devices share its data allowance; Cloudflare
  lets one key serve a few devices only, so WARP+ is for the shared mode).
- **Devices of subscriptions** (the default mode): each active subscription gets a free device of its own, registered in the background
  (a few per pass, spaced out, backing off for minutes when Cloudflare limits registrations). A subscription whose device is not there yet gets
  no WARP lines until it is. Devices of subscriptions that were removed from the panel are deleted at Cloudflare after a few passes.
  Account → *Delete all devices* gives everyone new keys.
- **Who gets it**: every active subscription, or only the ones ticked on the page.

WARP is **off until you turn it on** ("Give WARP to subscribers"): installing the plugin changes nobody's subscription.

## Speed: check and scan

Cloudflare answers on many addresses and ports (`162.159.192/193/195.0/24`, `188.114.96–99.0/24`, `2606:4700:d0/d1::/48`, ports 2408, 500,
1701, 4500 and about fifty more) and they are not equally quick from where you are. The engine in Better Cores:

- **Check**: brings a real AmneziaWG tunnel up to an endpoint with the account's key, waits for the handshake, asks the exit
  (`cdn-cgi/trace`: `warp=on|plus`, the exit's location, the round trip) and, if asked, downloads for a few seconds through four streams.
- **Scan**: sends each of a few hundred candidate endpoints a real handshake initiation made with the account's key (a WARP endpoint answers
  only a key it knows), keeps the quickest, asks the quickest dozen five times for a steady round trip and loss, and with *download* ranks
  the best few by speed. Use one result, or the best three (one node each).
- **Scan by itself**: every N hours, with the best endpoints kept as the subscription's endpoints. A scan that finds nothing changes nothing.

It measures what **this server** sees. The quickest endpoint for the server is often a good one for clients nearby and not always for clients
far away; the page says so. The engine runs in userspace inside Better Cores (it touches no interface or route of the machine) under the
plugin's CPU limit, so the speed it reports on a fast link is a floor.

## Permissions

| Permission | Why |
|---|---|
| `seam:sub` (dangerous) | to add the lines to the subscription answers |
| `api:clients:read` | to know which subscriptions exist and may be served |
| `network` (dangerous) | Cloudflare's registration API (`api.cloudflareclient.com`), nothing else |
| `ui:menu` | the WARP page in the sidebar |
| `peer:better-cores` (dangerous) | to call the WARP engine of Better Cores; Better Cores offers it nothing else |

`peer:<id>` is new in the plugin system **host-9**: a plugin that lists another in `requires` may call that plugin's `/peer/…` paths through
the gateway. An older host refuses to install the plugin. If Better Cores is missing, off, or too old, the page says so; the subscription lines
still work, only the check and the scanner do not.

## Data

`state.json` in the plugin's data directory holds the settings and every device's private key and Cloudflare token (mode 0600). Back it up
with the panel, and move it when the panel moves. A device's key is in the subscriber's config by design.

## Build

`make build` writes `bin/<arch>/betterwarp` (commit `bin/`: the panel downloads the repository as it is). `make test` runs the tests.
The build is reproducible (`-trimpath -buildvcs=false`, CGO off): with Go 1.26.8, `make build` leaves `bin/` unchanged.

MIT, see `LICENSE`.
