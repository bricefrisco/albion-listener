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
	// Read event and operation codes from JSON files
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

func toMessage(msg map[uint8]any) *Message {
	vals := sanitizeValues(msg)

	var eventType string
	var name string
	var ok bool
	if msg[252] != nil {
		eventType = "event"
		name, ok = eventCodes[fmt.Sprintf("%v", msg[252])]
		if !ok {
			fmt.Println("Unknown operation code:", msg[252])
			name = fmt.Sprintf("Unknown (%v)", msg[252])
		}
	} else if msg[253] != nil {
		eventType = "operation"
		name, ok = operationCodes[fmt.Sprintf("%v", msg[253])]
		if !ok {
			fmt.Println("Unknown operation code:", msg[253])
			name = fmt.Sprintf("Unknown (%v)", msg[253])
		}
	} else {
		eventType = "event"
		name = "Move"
	}

	m := &Message{
		Type: eventType,
		Name: name,
		Data: vals,
	}

	return m
}
