package listener

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"log"
	"sync"
)

type Message struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Data any    `json:"data"`
}

var buffer = newFragmentBuffer()

type Listener struct {
	messages chan *Message
}

func NewListener(messages chan *Message) *Listener {
	return &Listener{
		messages: messages,
	}
}

func (l *Listener) Run() {
	interfaces, err := getPhysicalInterfaces()
	if err != nil {
		log.Fatalln(err)
	}

	var wg sync.WaitGroup

	for _, iface := range interfaces {
		wg.Add(1)

		go func(iface string) {
			log.Println("Listening on network interface", iface)
			defer wg.Done()

			handle, err := pcap.OpenLive(iface, 2048, false, pcap.BlockForever)
			if err != nil {
				log.Fatalln("interface", iface, err)
			}
			defer handle.Close()

			err = handle.SetBPFFilter("port 5056")
			if err != nil {
				log.Fatalln("interface", iface, err)
				return
			}

			layers.RegisterUDPPortLayerType(layers.UDPPort(5056), photonLayerType)
			layers.RegisterTCPPortLayerType(layers.TCPPort(5056), photonLayerType)

			source := gopacket.NewPacketSource(handle, handle.LinkType())
			packets := source.Packets()

			for packet := range packets {
				if packet == nil {
					break
				}
				l.processPacket(packet)
			}
		}(iface)
	}
}

func (l *Listener) processPacket(packet gopacket.Packet) {
	ipLayer := packet.Layer(layers.LayerTypeIPv4)

	if ipLayer == nil {
		return
	}

	ipv4 := ipLayer.(*layers.IPv4)

	if ipv4.SrcIP == nil {
		return
	}

	layer := packet.Layer(photonLayerType)
	if layer == nil {
		return
	}
	content, _ := layer.(photonLayer)

	for _, command := range content.commands {
		switch command.commandType {
		case sendReliableType:
			l.onReliableCommand(&command)
		case sendUnreliableType:
			var s = make([]byte, len(command.data)-4)
			copy(s, command.data[4:])
			command.data = s
			command.length -= 4
			command.commandType = 6
			l.onReliableCommand(&command)
		case sendReliableFragmentType:
			msg, _ := command.reliableFragment()
			result := buffer.offer(msg)
			if result != nil {
				l.onReliableCommand(result)
			}
		}
	}
}

func (l *Listener) onReliableCommand(command *photonCommand) {
	msg, err := command.reliableMessage()
	if err != nil {
		log.Println(err)
		return
	}

	params, err := decodeReliableMessage(msg)
	if err != nil {
		log.Println(err)
		return
	}

	l.messages <- toMessage(msg, params)
}
