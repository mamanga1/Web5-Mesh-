package p2p

import (
    "log"
    "net"
    "time"
    
    "github.com/pion/stun"
)

type NATTraversal struct {
    transport   *TransportUDP
    publicIP    net.IP
    publicPort  int
    stunServer  string
}

func NewNATTraversal(transport *TransportUDP, stunServer string) *NATTraversal {
    return &NATTraversal{
        transport:  transport,
        stunServer: stunServer,
    }
}

func (n *NATTraversal) DiscoverPublicIP() error {
    c, err := stun.Dial("udp", n.stunServer)
    if err != nil {
        return err
    }
    defer c.Close()
    
    message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
    if err := c.Do(message, func(res stun.Event) {
        if res.Error != nil {
            return
        }
        var xorAddr stun.XORMappedAddress
        if err := xorAddr.GetFrom(res.Message); err != nil {
            return
        }
        n.publicIP = xorAddr.IP
        n.publicPort = xorAddr.Port
    }); err != nil {
        return err
    }
    
    log.Printf("[NAT] Public IP: %s:%d", n.publicIP.String(), n.publicPort)
    return nil
}

func (n *NATTraversal) HolePunch(targetIP string, targetPort int) error {
    addr := &net.UDPAddr{IP: net.ParseIP(targetIP), Port: targetPort}
    
    for i := 0; i < 5; i++ {
        n.transport.WriteTo([]byte("SYN"), addr)
        time.Sleep(50 * time.Millisecond)
    }
    
    log.Printf("[NAT] Hole punch attempted to %s:%d", targetIP, targetPort)
    return nil
}
