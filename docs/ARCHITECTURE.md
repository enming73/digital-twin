# Architecture

## Browser demo

The independent React/R3F browser demo owns its existing TypeScript virtual-time simulator, in-memory event log, replay reducer and presentation UI. It does not depend on the W2 process or a broker. Its known visual and replay limitations are in `README.md` and `docs/ACCEPTANCE.md`.

## W2 independent simulator

```
scenario + seed -> Go virtual-time engine -> ordered events -> NDJSON OR MQTT QoS 1 -> subscriber
```

`simulator/internal/scenario` validates line configuration; `engine` controls board ownership and virtual boundaries independently of wall time; `runner` paces those boundaries; `publisher` stamps wall-clock emission time and serializes the versioned synthetic envelope. `contracts/event.schema.json` and `docs/EVENT_CONTRACT.md` describe the external format. Publisher calls block until PUBACK: this is a bounded one-event in-flight queue and applies backpressure rather than dropping events. A failed acknowledgement retries the same serialized bytes and ID. Duplicates are possible; events and pending work are not durable across process crashes. On shutdown, the runner attempts to finish the current publish for up to three seconds and reports an error if the run is interrupted; remaining generated events in the batch are not claimed delivered.

For local development Compose exposes Mosquitto only at `127.0.0.1:1883`; anonymous clients inside its network are allowed. This is not production authentication, encryption or network security. W3 will own consumer validation/deduplication, gap detection and ingestion timestamps; W4 may add an optional UI data source, while the browser local demo remains available. No backend, browser/MQTT integration or live equipment is implemented in W2.
