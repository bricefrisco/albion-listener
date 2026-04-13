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

type Listener struct {
	messages chan *Message
	parser   *photonParser
}

func NewListener(messages chan *Message) *Listener {
	l := &Listener{
		messages: messages,
	}
	l.parser = newPhotonParser(l.onRequest, l.onResponse, l.onEvent)
	return l
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

	var payload []byte
	if udpLayer := packet.Layer(layers.LayerTypeUDP); udpLayer != nil {
		payload = udpLayer.(*layers.UDP).Payload
	} else if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
		payload = tcpLayer.(*layers.TCP).Payload
	}

	if len(payload) == 0 {
		return
	}

	l.parser.receivePacket(payload)
}

func (l *Listener) onRequest(opCode byte, params map[byte]interface{}) {
	l.messages <- toRequestMessage(opCode, params)
}

func (l *Listener) onResponse(opCode byte, _ int16, _ string, params map[byte]interface{}) {
	l.messages <- toResponseMessage(opCode, params)
}

func (l *Listener) onEvent(code byte, params map[byte]interface{}) {
	l.messages <- toEventMessage(code, params)
}
