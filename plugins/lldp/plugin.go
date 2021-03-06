package main

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/RackHD/inservice/plugins/lldp/grpc/lldp"
	"github.com/RackHD/inservice/plugins/lldp/neighbors"
	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// LLDPPlugin implements the Plugin interface.
type LLDPPlugin struct {
	address     string
	port        int
	interfaces  []string
	promiscuous bool
	timeout     time.Duration
	start       chan bool
	stop        chan bool
	packets     chan neighbors.Packet
	wg          *sync.WaitGroup
	Neighbors   neighbors.Neighbors
}

// NewLLDPPlugin initializes a new LLDPPlugin struct.
func NewLLDPPlugin(address string, port int, interfaces []string) (*LLDPPlugin, error) {
	if ip := net.ParseIP(address); ip == nil {
		return nil, fmt.Errorf("IP Address Not Valid")
	}
	if 0 >= port || port >= 65535 {
		return nil, fmt.Errorf("Invalid Port Selection")
	}
	ifaces := []net.Interface{}
	for _, iface := range interfaces {
		netDev, err := net.InterfaceByName(iface)
		if err != nil {
			log.Println("No interface named: ", iface)
			continue
		}
		ifaces = append(ifaces, *netDev)
	}
	if len(ifaces) < 1 {
		return nil, fmt.Errorf("No valid interfaces selected")
	}
	start := make(chan bool)
	wg := &sync.WaitGroup{}
	n, err := neighbors.NewNeighbors()
	if err != nil {
		log.Println("Error initializing neighbors")
	}
	n.NetDevList = ifaces //hitting some linter error where this cant be an argument
	return &LLDPPlugin{
		address:     address,
		port:        port,
		interfaces:  interfaces,
		promiscuous: true,
		timeout:     30 * time.Second,
		start:       start,
		stop:        nil,
		wg:          wg,
		Neighbors:   *n,
	}, nil
}

// Start handles plugin startup. This creates goroutines to handle the packet capture
// and serve the gRPC API or REST API
func (p *LLDPPlugin) Start(ctx context.Context) error {
	log.Println("LLDP Plugin Started.")
	p.stop = make(chan bool)
	p.wg.Add(1)
	go p.Capture()
	p.wg.Add(1)
	go p.Serve()
	time.Sleep(10 * time.Second)
	p.start <- true
	return nil
}

// Stop closes a channel that should stop all capture
func (p *LLDPPlugin) Stop(ctx context.Context) error {
	log.Println("LLDP Plugin Stopped.")
	close(p.stop)
	p.wg.Wait()
	return nil
}

// Status is a dummy function for now until a better status mechanism is identified
func (p *LLDPPlugin) Status(ctx context.Context) error {
	log.Println("LLDP Plugin Stopped.")
	return nil
}

// Serve creates the gRPC and REST endpoints for external model access
func (p *LLDPPlugin) Serve() {
	listenAddr := fmt.Sprintf("%s:%d", p.address, p.port)
	listen, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("Failed to Listen: %v", err)
	}

	server := grpc.NewServer()
	lldp.RegisterLldpServer(server, &p.Neighbors)

	server.Serve(listen)
}

// Capture opens an LLDP hook for each interface and processes packets.
func (p *LLDPPlugin) Capture() {
	log.Println("Started Capture")
	p.packets = make(chan neighbors.Packet, 1000)
	defer close(p.packets)
	for {
		select {
		case <-p.start:
			// create all interfaces
			for _, iface := range p.Neighbors.NetDevList {
				p.wg.Add(1)
				go p.openInterface(iface)
			}
			// process captured packets
			p.wg.Add(1)
			go func(p *LLDPPlugin) {
				for {
					select {
					case <-p.stop:
						log.Println("Stopping aggregator")
						p.wg.Done()
						return
					case recPacket := <-p.packets:
						p.wg.Add(1)
						go p.Neighbors.ProcessPacket(p.wg, recPacket)
					}
				}
			}(p)
		case <-p.stop:
			p.wg.Done()
			log.Println("Waited for all processes to stop")
			return
		}
	}
}

// openInterface creates a handle for each interface in the configuration and pushes
// packet to the global processing channel
func (p *LLDPPlugin) openInterface(iface net.Interface) error {
	log.Println("Opened interface: ", iface.Name)
	handle, err := pcap.OpenLive(iface.Name, 65536, p.promiscuous, p.timeout)
	if err != nil {
		return err
	}
	defer handle.Close()
	err = handle.SetBPFFilter("ether proto 0x88cc")
	if err != nil {
		return err
	}
	src := gopacket.NewPacketSource(handle, handle.LinkType())
	in := src.Packets()
	for {
		var packet gopacket.Packet
		select {
		case <-p.stop:
			log.Println("Exiting: ", iface.Name)
			handle.Close()
			p.wg.Done()
			return nil
		case packet = <-in:
			p.packets <- neighbors.Packet{Iface: iface, Packet: packet}
		}
	}
}
