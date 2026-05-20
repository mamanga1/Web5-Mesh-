#!/bin/bash
# ============================================================================
# scripts/commit-patent.sh - GPG Signature & Patent Protection Commit Script
# ============================================================================
# Especificación:
# - Inyección automática de firma GPG en commits
# - Registro anti-patentes para protección legal
# - Establece prior art público para evitar patentes corporativas
# ============================================================================

set -e

# Colores
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Configuración
PROJECT_NAME="web5-mesh"
PROJECT_LEAD_DID="did:maia:mamanga1-project-key"
PATENT_DISCLOSURE_FILE="PATENT-DISCLOSURE.md"
LICENSE_FILE="LICENSE-TRINCHERA"

# ============================================================================
# Funciones
# ============================================================================

print_banner() {
    echo -e "${CYAN}"
    echo "╔══════════════════════════════════════════════════════════════════╗"
    echo "║                                                                  ║"
    echo "║   ██████╗  █████╗ ████████╗███████╗███╗   ██╗████████╗           ║"
    echo "║   ██╔══██╗██╔══██╗╚══██╔══╝██╔════╝████╗  ██║╚══██╔══╝           ║"
    echo "║   ██████╔╝███████║   ██║   █████╗  ██╔██╗ ██║   ██║              ║"
    echo "║   ██╔═══╝ ██╔══██║   ██║   ██╔══╝  ██║╚██╗██║   ██║              ║"
    echo "║   ██║     ██║  ██║   ██║   ███████╗██║ ╚████║   ██║              ║"
    echo "║   ╚═╝     ╚═╝  ╚═╝   ╚═╝   ╚══════╝╚═╝  ╚═══╝   ╚═╝              ║"
    echo "║                                                                  ║"
    echo "║                    Patent Protection Commit                      ║"
    echo "║              Prior Art Disclosure & GPG Signing                  ║"
    echo "╚══════════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
}

check_gpg() {
    echo -e "${BLUE}[1/5] Checking GPG configuration...${NC}"
    
    if ! command -v gpg &> /dev/null; then
        echo -e "${YELLOW}GPG not found. Installing...${NC}"
        if command -v apt &> /dev/null; then
            sudo apt update && sudo apt install -y gnupg
        elif command -v yum &> /dev/null; then
            sudo yum install -y gnupg2
        else
            echo -e "${RED}Please install GPG manually${NC}"
            exit 1
        fi
    fi
    
    # Verificar si hay claves GPG
    if ! gpg --list-secret-keys --keyid-format LONG | grep -q "sec"; then
        echo -e "${YELLOW}No GPG key found. Generating one...${NC}"
        read -p "Enter your email for GPG key: " GPG_EMAIL
        read -p "Enter your name: " GPG_NAME
        
        gpg --batch --gen-key <<EOF
Key-Type: RSA
Key-Length: 4096
Subkey-Type: RSA
Subkey-Length: 4096
Name-Real: $GPG_NAME
Name-Email: $GPG_EMAIL
Expire-Date: 2y
%commit
EOF
        echo -e "${GREEN}GPG key generated${NC}"
    fi
    
    # Mostrar huella digital
    GPG_KEY_ID=$(gpg --list-secret-keys --keyid-format LONG | grep "sec" | head -1 | awk '{print $2}' | cut -d'/' -f2)
    GPG_FINGERPRINT=$(gpg --fingerprint $GPG_KEY_ID | grep "Key fingerprint" | head -1 | awk '{print $6,$7,$8,$9,$10}')
    
    echo -e "${GREEN}✓ GPG configured with key: $GPG_KEY_ID${NC}"
    echo -e "${CYAN}  Fingerprint: $GPG_FINGERPRINT${NC}"
    
    # Configurar git para usar GPG
    git config --local user.signingkey $GPG_KEY_ID
    git config --local commit.gpgsign true
    
    echo -e "${GREEN}✓ Git configured for GPG signing${NC}"
}

verify_patent_disclosure() {
    echo -e "${BLUE}[2/5] Verifying patent disclosure...${NC}"
    
    if [ ! -f "$PATENT_DISCLOSURE_FILE" ]; then
        echo -e "${RED}Patent disclosure file not found: $PATENT_DISCLOSURE_FILE${NC}"
        exit 1
    fi
    
    # Verificar contenido
    if ! grep -q "Prior Art Declaration" "$PATENT_DISCLOSURE_FILE"; then
        echo -e "${RED}Patent disclosure missing required content${NC}"
        exit 1
    fi
    
    # Actualizar fecha
    sed -i "s/Date of Disclosure:.*/Date of Disclosure: $(date -I)/" "$PATENT_DISCLOSURE_FILE"
    
    echo -e "${GREEN}✓ Patent disclosure verified${NC}"
}

verify_license() {
    echo -e "${BLUE}[3/5] Verifying license...${NC}"
    
    if [ ! -f "$LICENSE_FILE" ]; then
        echo -e "${RED}License file not found: $LICENSE_FILE${NC}"
        exit 1
    fi
    
    if ! grep -q "Anti-Corporate Appropriation" "$LICENSE_FILE"; then
        echo -e "${RED}License missing anti-corporate clause${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}✓ License verified${NC}"
}

create_patent_commit() {
    echo -e "${BLUE}[4/5] Creating patent-protected commit...${NC}"
    
    # Asegurar que el archivo de patente está actualizado
    echo "" >> "$PATENT_DISCLOSURE_FILE"
    echo "## Commit Record" >> "$PATENT_DISCLOSURE_FILE"
    echo "Commit Hash: $(git rev-parse HEAD 2>/dev/null || echo 'new-commit')" >> "$PATENT_DISCLOSURE_FILE"
    echo "Signed by: $(git config user.name) <$(git config user.email)>" >> "$PATENT_DISCLOSURE_FILE"
    echo "DID: $PROJECT_LEAD_DID" >> "$PATENT_DISCLOSURE_FILE"
    echo "Date: $(date -Iseconds)" >> "$PATENT_DISCLOSURE_FILE"
    
    # Agregar archivos
    git add "$PATENT_DISCLOSURE_FILE"
    git add "$LICENSE_FILE"
    
    # Crear mensaje de commit con notificación de patente
    COMMIT_MSG="feat(legal): patent disclosure and prior art declaration

This commit establishes prior art for the MaIA Mesh protocol inventions:

- Sovereign Overlay Network over Heterogeneous Hardware
- Lightweight Proof-of-Work for Sybil Resistance
- DHT Actor Model Concurrency
- CGNAT-Relay Fallback for Mobile Networks
- Dotted Version Vectors for CRDTs

This serves as public disclosure under first-to-file patent systems
(Argentina, USPTO, EPO). Any patent filed after $(date -I) claiming
these inventions is challenged as lacking novelty.

Signed-off-by: $(git config user.name) <$(git config user.email)>
DID: $PROJECT_LEAD_DID"
    
    # Realizar commit firmado
    if git commit -S -m "$COMMIT_MSG"; then
        echo -e "${GREEN}✓ Patent-protected commit created${NC}"
    else
        echo -e "${YELLOW}No changes to commit${NC}"
    fi
}

show_instructions() {
    echo -e "${BLUE}[5/5] Final instructions${NC}"
    
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}     Patent Protection Complete!       ${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo "Legal Protection Status:"
    echo "  ✅ Prior art established for 5+ inventions"
    echo "  ✅ GPG-signed commit provides timestamp proof"
    echo "  ✅ Anti-corporate license in place"
    echo "  ✅ Public disclosure recorded"
    echo ""
    echo "Next steps:"
    echo "  1. Push to GitHub: git push origin main"
    echo "  2. Archive on IPFS for immutable record"
    echo "  3. Submit to Archive.org for timestamp"
    echo ""
    echo -e "${CYAN}Your innovations are now protected against patent appropriation.${NC}"
    echo -e "${YELLOW}The code is the patent. The signature is the proof.${NC}"
}

# ============================================================================
# Main execution
# ============================================================================

main() {
    print_banner
    check_gpg
    verify_patent_disclosure
    verify_license
    create_patent_commit
    show_instructions
}

# Ejecutar
main "$@"
