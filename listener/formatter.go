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

func toRequestMessage(opCode byte, params map[byte]interface{}) *Message {
	key := strconv.Itoa(int(opCode))
	name, ok := operationCodes[key]
	if !ok {
		fmt.Println("Unknown operation request code:", opCode)
		name = fmt.Sprintf("Unknown (%v)", opCode)
	}
	return &Message{
		Type: "OperationRequest",
		Name: name,
		Data: sanitizeValues(params),
	}
}

func toResponseMessage(opCode byte, params map[byte]interface{}) *Message {
	key := strconv.Itoa(int(opCode))
	name, ok := operationCodes[key]
	if !ok {
		fmt.Println("Unknown operation response code:", opCode)
		name = fmt.Sprintf("Unknown (%v)", opCode)
	}
	return &Message{
		Type: "OperationResponse",
		Name: name,
		Data: sanitizeValues(params),
	}
}

func toEventMessage(code byte, params map[byte]interface{}) *Message {
	key := strconv.Itoa(int(code))
	name, ok := eventCodes[key]
	if !ok {
		fmt.Println("Unknown event code:", code)
		name = fmt.Sprintf("Unknown (%v)", code)
	}
	return &Message{
		Type: "Event",
		Name: name,
		Data: sanitizeValues(params),
	}
}
