#!/bin/bash
# ============================================================================
# scripts/deploy.sh - Production Deployment Script for MaIA Mesh Node
# ============================================================================
# Especificación:
# - Despliegue automatizado para Xeon, TV boxes y dispositivos móviles
# - Configuración de systemd para inicio automático
# - Creación de directorios con permisos seguros
# - Validación de dependencias y recursos
# ============================================================================

set -e

# Colores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuración
NODE_MODE=${1:-"full"}
NODE_NAME=${2:-"maia-mesh-node"}
DATA_DIR=${3:-"/var/lib/maia-mesh"}
INSTALL_DIR="/opt/maia-mesh"
SERVICE_NAME="maia-mesh"

# ============================================================================
# Funciones de utilidad
# ============================================================================

print_banner() {
    echo -e "${BLUE}"
    echo "╔══════════════════════════════════════════════════════════════════╗"
    echo "║                                                                  ║"
    echo "║   ███╗   ███╗ █████╗ ██╗ █████╗     ███╗   ███╗███████╗███████╗██╗  ██╗"
    echo "║   ████╗ ████║██╔══██╗██║██╔══██╗    ████╗ ████║██╔════╝██╔════╝██║  ██║"
    echo "║   ██╔████╔██║███████║██║███████║    ██╔████╔██║█████╗  ███████╗███████║"
    echo "║   ██║╚██╔╝██║██╔══██║██║██╔══██║    ██║╚██╔╝██║██╔══╝  ╚════██║██╔══██║"
    echo "║   ██║ ╚═╝ ██║██║  ██║██║██║  ██║    ██║ ╚═╝ ██║███████╗███████║██║  ██║"
    echo "║   ╚═╝     ╚═╝╚═╝  ╚═╝╚═╝╚═╝  ╚═╝    ╚═╝     ╚═╝╚══════╝╚══════╝╚═╝  ╚═╝"
    echo "║                                                                  ║"
    echo "║                    Sovereign Web5 Mesh Network                    ║"
    echo "║                         Production Deployer                       ║"
    echo "╚══════════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
}

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_root() {
    if [ "$EUID" -ne 0 ]; then
        log_error "This script must be run as root"
        exit 1
    fi
}

check_dependencies() {
    log_info "Checking dependencies..."
    
    # Verificar Go
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed. Please install Go 1.21+"
        exit 1
    fi
    
    go_version=$(go version | awk '{print $3}' | sed 's/go//')
    log_info "Go version: $go_version"
    
    # Verificar git
    if ! command -v git &> /dev/null; then
        log_error "Git is not installed"
        exit 1
    fi
    
    # Verificar systemd
    if ! command -v systemctl &> /dev/null; then
        log_warn "systemd not found, service installation will be skipped"
    fi
    
    log_info "All dependencies satisfied"
}

check_resources() {
    log_info "Checking system resources..."
    
    # RAM
    total_ram=$(free -m | awk '/^Mem:/ {print $2}')
    log_info "Total RAM: ${total_ram}MB"
    
    if [ "$total_ram" -lt 1024 ]; then
        log_warn "Low RAM detected: ${total_ram}MB (recommended: 2048MB+)"
    fi
    
    # Disco
    disk_free=$(df -m $DATA_DIR 2>/dev/null | awk 'NR==2 {print $4}' || echo "0")
    if [ "$disk_free" -eq 0 ]; then
        # Probar en /var/lib
        disk_free=$(df -m /var/lib | awk 'NR==2 {print $4}')
    fi
    log_info "Free disk space: ${disk_free}MB"
    
    if [ "$disk_free" -lt 10240 ]; then
        log_warn "Low disk space: ${disk_free}MB (recommended: 20480MB+)"
    fi
    
    # CPU
    cpu_cores=$(nproc)
    log_info "CPU cores: $cpu_cores"
}

create_directories() {
    log_info "Creating directories..."
    
    # Directorios principales
    mkdir -p $INSTALL_DIR
    mkdir -p $DATA_DIR
    mkdir -p $DATA_DIR/dht
    mkdir -p $DATA_DIR/storage
    mkdir -p $DATA_DIR/cache
    mkdir -p $DATA_DIR/logs
    
    # Directorios para BadgerDB
    mkdir -p $DATA_DIR/badger
    
    # Permisos seguros
    chmod 750 $DATA_DIR
    chmod 750 $INSTALL_DIR
    
    # Crear usuario del sistema si no existe
    if ! id -u "maia" &>/dev/null; then
        useradd -r -s /bin/false -d $DATA_DIR maia
        log_info "Created system user 'maia'"
    fi
    
    # Asignar propietario
    chown -R maia:maia $DATA_DIR
    chown -R maia:maia $INSTALL_DIR
    
    log_info "Directories created: $DATA_DIR, $INSTALL_DIR"
}

build_binary() {
    log_info "Building MaIA Mesh binary..."
    
    cd $INSTALL_DIR
    
    # Clonar o copiar código
    if [ ! -d "$INSTALL_DIR/src" ]; then
        if [ -d "/tmp/web5-mesh" ]; then
            cp -r /tmp/web5-mesh/* $INSTALL_DIR/
        else
            log_error "Source code not found. Please copy source to $INSTALL_DIR first"
            exit 1
        fi
    fi
    
    # Compilar
    export GO111MODULE=on
    export CGO_ENABLED=1
    
    BUILD_TIME=$(date -u +%Y%m%d_%H%M%S)
    VERSION="2.0.0-production"
    
    go build -tags=production \
        -ldflags="-s -w -X main.Version=$VERSION -X main.BuildTime=$BUILD_TIME" \
        -o $INSTALL_DIR/maia-mesh \
        $INSTALL_DIR/src/core/main.go
    
    if [ $? -eq 0 ]; then
        log_info "Binary built successfully: $INSTALL_DIR/maia-mesh"
    else
        log_error "Build failed"
        exit 1
    fi
    
    # Verificar binario
    if [ -f "$INSTALL_DIR/maia-mesh" ]; then
        chmod 755 $INSTALL_DIR/maia-mesh
        chown maia:maia $INSTALL_DIR/maia-mesh
        log_info "Binary size: $(ls -lh $INSTALL_DIR/maia-mesh | awk '{print $5}')"
    fi
}

create_config() {
    log_info "Creating configuration..."
    
    cat > $DATA_DIR/config.json << EOF
{
  "node": {
    "name": "$NODE_NAME",
    "mode": "$NODE_MODE",
    "started_at": "$(date -Iseconds)"
  },
  "network": {
    "udp_port": 4242,
    "tcp_port": 4243,
    "max_connections": 1024,
    "handshake_timeout": 30,
    "heartbeat_interval": 15
  },
  "storage": {
    "data_dir": "$DATA_DIR",
    "replication_factor": 3
  },
  "performance": {
    "max_goroutines": 1000,
    "enable_metrics": true,
    "metrics_port": 2112
  },
  "crypto": {
    "pow_difficulty": 16,
    "enable_noise": true
  }
}
EOF
    
    chmod 640 $DATA_DIR/config.json
    chown maia:maia $DATA_DIR/config.json
    log_info "Configuration created: $DATA_DIR/config.json"
}

create_systemd_service() {
    if ! command -v systemctl &> /dev/null; then
        log_warn "systemd not found, skipping service creation"
        return
    fi
    
    log_info "Creating systemd service..."
    
    cat > /etc/systemd/system/$SERVICE_NAME.service << EOF
[Unit]
Description=MaIA Mesh Sovereign Node
Documentation=https://github.com/mamanga1/web5-mesh
After=network.target nss-lookup.target
Wants=network-online.target

[Service]
Type=simple
User=maia
Group=maia
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/maia-mesh --mode=$NODE_MODE --data-dir=$DATA_DIR
Restart=on-failure
RestartSec=10
TimeoutStopSec=30
LimitNOFILE=65536

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=$DATA_DIR
PrivateTmp=true
ProtectHome=true

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=$SERVICE_NAME

[Install]
WantedBy=multi-user.target
EOF
    
    systemctl daemon-reload
    log_info "Systemd service created: /etc/systemd/system/$SERVICE_NAME.service"
}

start_service() {
    log_info "Starting MaIA Mesh node..."
    
    if command -v systemctl &> /dev/null; then
        systemctl enable $SERVICE_NAME
        systemctl start $SERVICE_NAME
        
        sleep 3
        
        if systemctl is-active --quiet $SERVICE_NAME; then
            log_info "Service started successfully"
            systemctl status $SERVICE_NAME --no-pager
        else
            log_error "Service failed to start"
            journalctl -u $SERVICE_NAME -n 20 --no-pager
            exit 1
        fi
    else
        log_warn "systemd not available, run manually: $INSTALL_DIR/maia-mesh"
        $INSTALL_DIR/maia-mesh --mode=$NODE_MODE --data-dir=$DATA_DIR &
    fi
}

show_instructions() {
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}     MaIA Mesh Deployment Complete!     ${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo "Node Information:"
    echo "  Mode: $NODE_MODE"
    echo "  Data Directory: $DATA_DIR"
    echo "  Install Directory: $INSTALL_DIR"
    echo ""
    echo "Useful Commands:"
    echo "  Check status: systemctl status $SERVICE_NAME"
    echo "  View logs: journalctl -u $SERVICE_NAME -f"
    echo "  Stop node: systemctl stop $SERVICE_NAME"
    echo "  Restart node: systemctl restart $SERVICE_NAME"
    echo ""
    echo "Metrics endpoint: http://localhost:2112/metrics"
    echo "Health check: http://localhost:2112/health"
    echo ""
    echo -e "${YELLOW}Your node is now part of the sovereign MaIA Mesh network!${NC}"
}

# ============================================================================
# Main execution
# ============================================================================

main() {
    print_banner
    
    check_root
    check_dependencies
    check_resources
    create_directories
    build_binary
    create_config
    create_systemd_service
    start_service
    show_instructions
}

# Ejecutar main
main "$@"
