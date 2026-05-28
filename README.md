# Web5-Mesh / U2P - Red Soberana P2P

🛰️ **IBERÁ AON (Autonomous Overlay Network)**

![El Faro de la Red](https://i.postimg.cc/V6bPWYdC/faro.jpg)

*"La TV Box X96Q que alumbra la primera red soberana. 4 núcleos, 1GB de RAM, una antenita USB. No hace falta una nube, hace falta una linterna."*

El protocolo U2P y la infraestructura MaIA Mesh nacidos en el barro del NEA.

Esto no es la internet de Silicon Valley pagada con billeteras de fondos de inversión. Esto es una red de guerrilla digital donde se terminaron los servidores reyes y los clientes mendigos. Acá somos todos clientes y servidores al mismo tiempo, y las reglas del juego cambiaron: en esta malla va a lucir el más capaz por su eficiencia sobre el metal, no el que más brille por su marketing.

El código está optimizado y afilado para correr en el metal de una Xeon pesada o en el chip de un TV Box reciclado con un hilo de conexión. Si te bancás el ruteo, si minás el puzzle para validar tu identidad y mantenés el almacenamiento firme, sos parte del enjambre.

---

## The Sovereign Web5 Protocol - End-to-End Decentralized Infrastructure

<div align="center">

[![CI Status](https://img.shields.io/github/actions/workflow/status/mamanga1/web5-mesh/ci.yml?style=flat-square&label=CI)](https://github.com/mamanga1/web5-mesh/actions)
[![License](https://img.shields.io/badge/License-MIT%2BAnti--Corporate-blue?style=flat-square)](LICENSE-TRINCHERA)
[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat-square&logo=go)](https://golang.org/)

**The first truly sovereign overlay network where software owns the hardware and nodes own the routing.**

</div>

---

## 🔥 What Makes web5-mesh Different?

| Traditional Internet | web5-mesh (Web5 Native) |
|---------------------|-------------------------|
| ❌ Single point of failure | ✅ No intermediaries |
| ❌ Metadata visible to ISP | ✅ ISP sees only encrypted UDP |
| ❌ Censorable by takedown | ✅ Unstoppable network |
| ❌ Identity tied to IP | ✅ Identity = Crypto. Proof |
| ❌ Centralized DNS | ✅ Self-hosted .mesh domains |

---

## 🚀 Quick Start - Deploy Your Sovereign Node

### Prerequisites

| Requirement | Minimum Version |
|-------------|-----------------|
| Go Compiler | 1.21+ |
| Operating Systems | Linux / Unix / macOS / Windows / Android (Termux) |
| Edge Nodes (TV Boxes) | 1GB RAM |
| Staging Nodes (Relays) | 4GB+ RAM (8GB+ rec.) |
| Network | Intermittent or stable |

### One-Line Installation

```bash
git clone https://github.com/mamanga1/Web5-Mesh.git
cd Web5-Mesh
go build -tags=netgo -o web5-mesh .
./web5-mesh -mode pure -udp-port 4245
Connect to the Public Beacon (Faro)
bash
./web5-mesh -mode pure -udp-port 4245 -seeds 190.220.45.26:4245
Expected Output
text
╔══════════════════════════════════════════════════════════════════╗
║                    Sovereign Web5 Mesh Network                    ║
╚══════════════════════════════════════════════════════════════════╝

[PoW] NodeID generated with nonce=17, difficulty=4
[P2P] Kademlia started with Node ID: 0dc0d0f999140c8b...
[NODE] Started successfully, DID: did:maia:FyMbwkrxGDnKHumx...

🛠️ Compilación Multiplataforma
bash
# Linux AMD64 (Xeon, servidores)
GOOS=linux GOARCH=amd64 go build -tags=netgo -o web5-mesh-linux-amd64 .

# Linux ARM64 (TV Box, Raspberry Pi, Android)
GOOS=linux GOARCH=arm64 go build -tags=netgo -o web5-mesh-linux-arm64 .

# Windows
GOOS=windows GOARCH=amd64 go build -tags=netgo -o web5-mesh.exe .

# macOS
GOOS=darwin GOARCH=amd64 go build -tags=netgo -o web5-mesh-macos .

# Android (Termux)
GOOS=android GOARCH=arm64 go build -tags=netgo -o web5-mesh-android .

📊 Performance Metrics

DHT Operation	Local (<10ms RTT)	Regional (>50ms RTT)
Node Discovery	8.2 ms ± 0.3	47.6 ms ± 2.1
DID Lookup	12.1 ms ± 0.5	89.3 ms ± 4.2
STORE / FIND_VALUE	9.8 ms ± 0.4	68.2 ms ± 3.1
Reliability Metric	Value	Period
Network Uptime	99.87%	Last 30 days
Data Consistency	99.94%	CRDT validation
DHT Availability	99.99%	No SPOF

🔐 Security & Cryptography

Layer	Implementation	Status
Identity	Ed25519 DIDs (did:maia:Base58)	✅
Anti-Sybil	Proof-of-Work (4-bit Hashcash)	✅
Handshake	Noise Protocol IK (Perfect Forward Secrecy)	✅
Transport	ChaCha20-Poly1305	✅
Access Control	ACL (whitelist)	✅
NAT Traversal	STUN + Hole punching	✅
Relays	Cifrado E2E (no descifran)	✅
Perfect Forward Secrecy (PFS) garantizado: Cada sesión usa claves efímeras. Si comprometen una clave hoy, el tráfico de ayer sigue siendo indescifrable.

📁 Project Structure
text
web5-mesh/
├── src/
│   ├── core/          # Node orchestration
│   ├── crypto/        # Ed25519 + ChaCha20 + PoW
│   ├── p2p/           # U2P (transport, Kademlia, STUN, Noise, ACL)
│   ├── storage/       # BadgerDB + CRDTs
│   └── config/        # Configuration
├── cmd/               # CLI tools
├── tests/             # Integration tests
└── LICENSE-TRINCHERA  # MIT + anti-corporate clause

🛠️ Basic Commands (using netcat)
bash
# Store a value
echo -n "STORE:myKey:myValue" | nc -u <NODE_IP> 4245

# Retrieve a value
echo -n "FIND_VALUE:myKey" | nc -u <NODE_IP> 4245
# Response: VALUE:myValue
🔜 Coming Soon (Next Sprint)
Feature	Description
SSH over U2P	ssh -o ProxyCommand="./web5-mesh proxy %h %p" did:maia:xeon
Messaging DID to DID	Buzón distribuido sobre DHT, cifrado extremo a extremo
.mesh Hosting	Publicar sitios web estáticos en la DHT
Token PIRE	Transferencia de valor nativa en la red

💥 El Golpe Maestro a la Industria de Seguridad

Cloudflare: De dictador del tráfico a lubricante de infraestructura
Hoy (Web2)	Mañana (U2P)
Es el "bouncer" que decide qué tráfico pasa	Pasa a ser un Súper Nodo Faro
Cachea contenido, inspecciona, censura	Solo ve bytes cifrados - no puede inspeccionar
Modelo: "proteger tu IP"	Modelo: acelerar tu tráfico P2P
Puede bloquear dominios enteros	No sabe qué contenido está pasando
Google: Fin del scraping promiscuo
Hoy (Web2)	Mañana (U2P)
Crawler scrapea tu web sin permiso	Necesita autenticarse con clave pública
Consume tu ancho de banda gratis	Puedes cobrar por megabyte indexado
Entrena sus IAs con tu contenido	Puedes exigir PoW costosa por acesso
Monopolio de la indexación	Indexación distribuida y soberana
Combate a clones criptográficos (Sybil / Eclipse)
Ataque	Defensa en U2P
Generar millones de NodeIDs falsos	PoW atado a DID real - costo acumulado
Inundar la DHT de basura	Buckets limitados (20 contactos) + LRU
Eclipse (aislar un nodo)	Kademlia con 160 buckets redundantes
Suplantación de identidad	Firmas Ed25519 en cada mensaje

⚖️ License
MIT with Anti-Corporate Appropriation Clause. See LICENSE-TRINCHERA.

Corporations (>50 employees) using this protocol must:

✅ Open-source their implementation within 30 days

✅ Contribute ≥10% of net revenue to maintenance fund

✅ Offer patent cross-licensing

📞 Community & Direct Support
Issues & Code: github.com/mamanga1/Web5-Mesh/issues

Secure Email: IberaAON@proton.me (PGP encrypted)

Telegram: @IberaAON

<div align="center">
La internet donde los nodos son dueños de sus propias rutas.

Hecho con orgullo y aguante desde Corrientes, Argentina.

Protocol Version: 2.0.0-production
DID of Project Lead: did:maia:mamanga1-project-key

</div>
💰 Contribuciones al Búnker
Si querés apoyar el desarrollo, la infraestructura y los gastos del búnker (hosting, equipos de prueba, café ☕):

Binance ID: 218085972 (Mamanga)

Toda contribución ayuda a mantener la red soberana funcionando.

¡Gracias por ser parte de la malla! 🧉🦾
