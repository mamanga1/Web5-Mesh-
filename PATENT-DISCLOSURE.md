# Patent Disclosure & Prior Art Declaration - iAP2P / MaIA Mesh

**Date of Disclosure:** May 20, 2026
**Project Lead:** Mamanga (DID: did:maia:mamanga1-project-key)
**Repository:** github.com/mamanga1/web5-mesh

## Summary of Novel Inventions

This document serves as public prior art disclosure for the following novel
technical inventions implemented in the web5-mesh protocol stack:

### 1. Sovereign Overlay Network over Heterogeneous Hardware

A Kademlia-based DHT overlay network designed to run on mixed hardware
architectures (x86_64 Xeon servers, ARM64 mobile devices, ARMv7 TV boxes)
without central coordination.

**Key Claims:**
- Automatic hardware profiling and adaptive buffer sizing
- Churn-optimized k=16 bucket configuration
- XOR distance routing with stale node pruning

### 2. Lightweight Proof-of-Work for Sybil Resistance

Hashcash-style puzzle attached to DID generation preventing mass identity
creation attacks.

**Key Claims:**
- Difficulty 16-20 bits adjustable per network conditions
- Nonce verification without storing puzzle state
- Integration with reputation system for graduated trust

### 3. DHT Actor Model Concurrency

Lock-free DHT operations using channel-based Actor pattern eliminating
sync.RWMutex deadlocks.

**Key Claims:**
- Single-threaded bucket mutation per actor
- Priority-based message queues (lookup > route_update > join)
- Arena allocation for GC pressure reduction

### 4. CGNAT-Relay Fallback for Mobile Networks

UDP hole punching with automatic fallback to relay nodes when symmetric NAT
detected.

**Key Claims:**
- 15-second keepalive for carrier-grade NAT
- STUN-based external IP discovery
- Decentralized relay selection via reputation scoring

### 5. Dotted Version Vectors for CRDTs

Conflict-free Replicated Data Types using dotted version vectors for
deterministic merge after network partitions.

**Key Claims:**
- Vector clock truncation by causality
- Memory-efficient event ID mapping
- Timestamp + signature tie-breaking

## Legal Purpose

This disclosure is filed under the "first-to-file" patent systems of
Argentina, United States, and European Union to establish prior art as of the
date above. Any patent filed after this date claiming these inventions is
hereby challenged as lacking novelty.

## Contact for Prior Art Verification

Project Repository: https://github.com/mamanga1/web5-mesh
Archive.org Backup: https://web.archive.org/web/*/github.com/mamanga1/web5-mesh

**Signed by Project Lead:**
Mamanga
DID: did:maia:mamanga1-project-key
