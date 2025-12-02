package listener

import (
	"embed"
	"encoding/json"
	"fmt"
	"strconv"
)

//go:embed assets/*.json
var assets embed.FS

var (
	eventCodes     map[string]string
	operationCodes map[string]string
)

func init() {
	ec, err := assets.ReadFile("assets/eventCodes.json")
	if err != nil {
		panic(fmt.Sprintf("failed to read event codes: %v", err))
	}

	oc, err := assets.ReadFile("assets/operationCodes.json")
	if err != nil {
		panic(fmt.Sprintf("failed to read operation codes: %v", err))
	}

	if err = json.Unmarshal(ec, &eventCodes); err != nil {
		panic(fmt.Sprintf("failed to unmarshal event codes: %v", err))
	}

	if err = json.Unmarshal(oc, &operationCodes); err != nil {
		panic(fmt.Sprintf("failed to unmarshal operation codes: %v", err))
	}
}

func sanitizeValues(v any) any {
	switch val := v.(type) {
	case map[interface{}]interface{}:
		m := make(map[string]interface{}, len(val))
		for k, v2 := range val {
			m[fmt.Sprintf("%v", k)] = sanitizeValues(v2)
		}
		return m

	case map[byte]any:
		m := make(map[string]interface{}, len(val))
		for k, v2 := range val {
			m[strconv.Itoa(int(k))] = sanitizeValues(v2)
		}
		return m

	case map[string]interface{}:
		for k, v2 := range val {
			val[k] = sanitizeValues(v2)
		}
		return val

	case []interface{}:
		for i := range val {
			val[i] = sanitizeValues(val[i])
		}
		return val

	case []uint8: // treat as bytes, convert to list of ints
		ints := make([]int, len(val))
		for i, b := range val {
			ints[i] = int(b)
		}
		return ints

	default:
		return val
	}
}

func toMessage(msg reliableMessage, params map[uint8]any) *Message {
	vals := sanitizeValues(params)

	var messageType string
	var name string
	var ok bool

	switch msg.messageType {
	case eventDataType:
		messageType = "Event"
		_, ok := params[252]
		if !ok {
			name = "Move"
			break
		}
		name, ok = eventCodes[fmt.Sprintf("%v", params[252])]
		if !ok {
			fmt.Println("Unknown event code:", params[252])
			name = fmt.Sprintf("Unknown (%v)", params[252])
		}
	case operationRequest:
		messageType = "OperationRequest"
		name, ok = operationCodes[fmt.Sprintf("%v", params[253])]
		if !ok {
			fmt.Println("Unknown operation request code:", params[253])
			name = fmt.Sprintf("Unknown (%v)", params[253])
		}
	case operationResponse:
		messageType = "OperationResponse"
		name, ok = operationCodes[fmt.Sprintf("%v", params[253])]
		if !ok {
			fmt.Println("Unknown operation response code:", params[253])
			name = fmt.Sprintf("Unknown (%v)", params[253])
		}
	}

	m := &Message{
		Type: messageType,
		Name: name,
		Data: vals,
	}

	return m
}
