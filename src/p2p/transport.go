package p2p

import (
        "fmt"
        "net"
        "time"

        "github.com/mamanga1/web5-mesh/src/crypto"
)

type TransportUDP struct {
        conn         *net.UDPConn
        port         int
        readTimeout  time.Duration
        writeTimeout time.Duration
        localAddr    *net.UDPAddr
        sessionKey   [32]byte
        sessionReady bool
}

func NewTransportUDP(port int, readTimeout, writeTimeout time.Duration) (*TransportUDP, error) {
        if readTimeout == 0 {
                readTimeout = 10 * time.Second
        }
        if writeTimeout == 0 {
                writeTimeout = 5 * time.Second
        }

        addr := &net.UDPAddr{IP: net.IPv4(0, 0, 0, 0), Port: port}
        conn, err := net.ListenUDP("udp", addr)
        if err != nil {
                return nil, fmt.Errorf("failed to listen UDP on port %d: %w", port, err)
        }

        return &TransportUDP{
                conn:         conn,
                port:         port,
                readTimeout:  readTimeout,
                writeTimeout: writeTimeout,
                localAddr:    conn.LocalAddr().(*net.UDPAddr),
                sessionReady: false,
        }, nil
}

func (t *TransportUDP) LocalAddr() *net.UDPAddr {
        return t.localAddr
}

// SetSessionKey establece la clave para cifrado
func (t *TransportUDP) SetSessionKey(key [32]byte) {
        t.sessionKey = key
        t.sessionReady = true
}

// IsSessionReady devuelve si la sesión está cifrada
func (t *TransportUDP) IsSessionReady() bool {
        return t.sessionReady
}

func (t *TransportUDP) ReadFrom() ([]byte, *net.UDPAddr, error) {
        buf := make([]byte, 65536)
        t.conn.SetReadDeadline(time.Now().Add(t.readTimeout))
        n, addr, err := t.conn.ReadFromUDP(buf)
        if err != nil {
                return nil, nil, err
        }

        data := buf[:n]

        // Si la sesión está cifrada, intentar descifrar
        if t.sessionReady {
                decrypted, err := crypto.DecryptBytes(data, t.sessionKey)
                if err != nil {
                        // Si falla el descifrado, devolver datos sin procesar
                        return data, addr, nil
                }
                return decrypted, addr, nil
        }

        return data, addr, nil
}

func (t *TransportUDP) WriteTo(data []byte, addr *net.UDPAddr) error {
        t.conn.SetWriteDeadline(time.Now().Add(t.writeTimeout))

        var finalData []byte

        // Si la sesión está cifrada, cifrar el mensaje
        if t.sessionReady {
                encrypted, err := crypto.EncryptBytes(data, t.sessionKey)
                if err != nil {
                        return err
                }
                finalData = encrypted
        } else {
                finalData = data
        }

        _, err := t.conn.WriteToUDP(finalData, addr)
        return err
}

func (t *TransportUDP) Close() error {
        return t.conn.Close()
}
