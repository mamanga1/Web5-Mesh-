package p2p

import (
    "log"
    "net"
    
    "github.com/pion/stun"
)

type NATTraversal struct {
    transport   *TransportUDP
    PublicIP    net.IP
    PublicPort  int
    stunServer  string
}

func NewNATTraversal(transport *TransportUDP, stunServer string) *NATTraversal {
    return &NATTraversal{
        transport:  transport,
        stunServer: stunServer,
    }
}

func (n *NATTraversal) DiscoverPublicIP() error {
    conn, err := net.Dial("udp", n.stunServer)
    if err != nil {
        return err
    }
    defer conn.Close()

    message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
    if _, err := conn.Write(message.Raw); err != nil {
        return err
    }

    buf := make([]byte, 1024)
    byteCount, err := conn.Read(buf)
    if err != nil {
        return err
    }

    res := &stun.Message{Raw: buf[:byteCount]}
    if err := res.Decode(); err != nil {
        return err
    }

    var xorAddr stun.XORMappedAddress
    if err := xorAddr.GetFrom(res); err != nil {
        return err
    }

    n.PublicIP = xorAddr.IP
    n.PublicPort = xorAddr.Port

    log.Printf("[NAT] Public IP: %s:%d", n.PublicIP.String(), n.PublicPort)
    return nil
}
