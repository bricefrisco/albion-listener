package listener

import (
	"bytes"
	"encoding/binary"
)

const (
	photonHeaderLength   = 12
	commandHeaderLength  = 12
	fragmentHeaderLength = 20
)

const (
	cmdDisconnect     = byte(4)
	cmdSendReliable   = byte(6)
	cmdSendUnreliable = byte(7)
	cmdSendFragment   = byte(8)
)

const (
	msgRequest     = byte(2)
	msgResponse    = byte(3)
	msgEvent       = byte(4)
	msgResponseAlt = byte(7)
	msgEncrypted   = byte(131)
)

type segmentedPackage struct {
	totalLength  int
	bytesWritten int
	payload      []byte
}

type photonParser struct {
	pendingSegments map[int]*segmentedPackage
	onRequest       func(operationCode byte, params map[byte]interface{})
	onResponse      func(operationCode byte, returnCode int16, debugMessage string, params map[byte]interface{})
	onEvent         func(code byte, params map[byte]interface{})
	onEncrypted     func()
}

func newPhotonParser(
	onRequest func(byte, map[byte]interface{}),
	onResponse func(byte, int16, string, map[byte]interface{}),
	onEvent func(byte, map[byte]interface{}),
) *photonParser {
	return &photonParser{
		pendingSegments: make(map[int]*segmentedPackage),
		onRequest:       onRequest,
		onResponse:      onResponse,
		onEvent:         onEvent,
	}
}

func (p *photonParser) receivePacket(payload []byte) bool {
	if len(payload) < photonHeaderLength {
		return false
	}

	offset := 2 // skip peerId (2 bytes)
	flags := payload[offset]
	offset++
	commandCount := int(payload[offset])
	offset++
	offset += 8 // skip timestamp (4) + challenge (4)

	if flags == 1 {
		if p.onEncrypted != nil {
			p.onEncrypted()
		}
		return false
	}

	for i := 0; i < commandCount; i++ {
		var ok bool
		offset, ok = p.handleCommand(payload, offset)
		if !ok {
			return false
		}
	}
	return true
}

func (p *photonParser) handleCommand(src []byte, offset int) (int, bool) {
	if !available(src, offset, commandHeaderLength) {
		return offset, false
	}

	cmdType := src[offset]
	offset++
	offset++ // channelId
	offset++ // commandFlags
	offset++ // reserved byte
	cmdLen := int(binary.BigEndian.Uint32(src[offset:]))
	offset += 4
	offset += 4 // reliableSequenceNumber
	cmdLen -= commandHeaderLength

	if cmdLen < 0 || !available(src, offset, cmdLen) {
		return offset, false
	}

	switch cmdType {
	case cmdDisconnect:
		return offset + cmdLen, true
	case cmdSendUnreliable:
		if cmdLen < 4 {
			return offset + cmdLen, false
		}
		offset += 4
		cmdLen -= 4
		newOffset, _ := p.handleSendReliable(src, offset, cmdLen)
		return newOffset, true
	case cmdSendReliable:
		newOffset, _ := p.handleSendReliable(src, offset, cmdLen)
		return newOffset, true
	case cmdSendFragment:
		return p.handleSendFragment(src, offset, cmdLen), true
	default:
		return offset + cmdLen, true
	}
}

func (p *photonParser) handleSendReliable(src []byte, offset, cmdLen int) (int, bool) {
	if cmdLen < 2 || !available(src, offset, cmdLen) {
		return offset + cmdLen, false
	}

	offset++            // signal byte
	msgType := src[offset]
	offset++
	cmdLen -= 2

	if !available(src, offset, cmdLen) {
		return offset + cmdLen, false
	}

	if msgType == msgEncrypted {
		if p.onEncrypted != nil {
			p.onEncrypted()
		}
		return offset + cmdLen, true
	}

	data := src[offset : offset+cmdLen]
	offset += cmdLen

	switch msgType {
	case msgRequest:
		p.dispatchRequest(data)
	case msgResponse, msgResponseAlt:
		p.dispatchResponse(data)
	case msgEvent:
		p.dispatchEvent(data)
	}

	return offset, true
}

func (p *photonParser) dispatchRequest(data []byte) {
	if len(data) < 1 {
		return
	}
	opCode := data[0]
	params := deserializeParameterTable(data[1:])
	if p.onRequest != nil {
		p.onRequest(opCode, params)
	}
}

func (p *photonParser) dispatchResponse(data []byte) {
	if len(data) < 3 {
		return
	}
	opCode := data[0]
	returnCode := int16(binary.LittleEndian.Uint16(data[1:3]))

	buf := bytes.NewBuffer(data[3:])
	debugMsg := ""

	if buf.Len() > 0 {
		tc, _ := buf.ReadByte()
		val := deserialize(buf, tc)
		switch v := val.(type) {
		case string:
			debugMsg = v
		case []string:
			// Albion embeds market-order data as a typed string array where the
			// debug message would normally be. Surface it as params[0].
			params := map[byte]interface{}{0: v}
			if p.onResponse != nil {
				p.onResponse(opCode, returnCode, "", params)
			}
			return
		}
	}

	params := readParameterTable(buf)
	if p.onResponse != nil {
		p.onResponse(opCode, returnCode, debugMsg, params)
	}
}

func (p *photonParser) dispatchEvent(data []byte) {
	if len(data) < 1 {
		return
	}
	code := data[0]
	params := deserializeParameterTable(data[1:])
	if p.onEvent != nil {
		p.onEvent(code, params)
	}
}

func (p *photonParser) handleSendFragment(src []byte, offset, cmdLen int) int {
	if cmdLen < fragmentHeaderLength || !available(src, offset, fragmentHeaderLength) {
		return offset + cmdLen
	}

	startSeq := int(binary.BigEndian.Uint32(src[offset:]))
	offset += 4
	cmdLen -= 4
	offset += 4 // fragmentCount
	cmdLen -= 4
	offset += 4 // fragmentNumber
	cmdLen -= 4
	totalLen := int(binary.BigEndian.Uint32(src[offset:]))
	offset += 4
	cmdLen -= 4
	fragOffset := int(binary.BigEndian.Uint32(src[offset:]))
	offset += 4
	cmdLen -= 4

	fragLen := cmdLen
	if fragLen < 0 || !available(src, offset, fragLen) {
		return offset + fragLen
	}

	seg, ok := p.pendingSegments[startSeq]
	if !ok {
		seg = &segmentedPackage{
			totalLength: totalLen,
			payload:     make([]byte, totalLen),
		}
		p.pendingSegments[startSeq] = seg
	}

	end := fragOffset + fragLen
	if end <= len(seg.payload) {
		copy(seg.payload[fragOffset:end], src[offset:offset+fragLen])
	}
	offset += fragLen
	seg.bytesWritten += fragLen

	if seg.bytesWritten >= seg.totalLength {
		delete(p.pendingSegments, startSeq)
		p.handleSendReliable(seg.payload, 0, len(seg.payload))
	}

	return offset
}

func available(src []byte, offset, count int) bool {
	return count >= 0 && offset >= 0 && len(src)-offset >= count
}
