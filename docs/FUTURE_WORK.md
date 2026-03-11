# Future Work

Items that are out of scope for the initial release but worth pursuing later. These are not tracked in `IMPLEMENTATION_TASKS.md` and should not be mentioned during regular task planning unless explicitly requested.

---

## UDP Tracker Protocol (BEP 15)

Implement the UDP announce/scrape protocol for lower latency and higher connection throughput. The HTTP tracker covers all functionality; UDP is a performance optimization for high-traffic deployments.

**Key requirements:**
- Connection handshake with protocol_id verification and connection_id with 2-minute TTL
- Announce and scrape sharing the same service layer as the HTTP tracker
- Compact response only (UDP has no room for non-compact)
- IPv4 and IPv6 support

**Why deferred:** HTTP tracker is fully functional and sufficient for the current scale. UDP adds operational complexity (separate port, stateless protocol, connection ID cache) with marginal benefit until the tracker handles thousands of concurrent announces per second.
