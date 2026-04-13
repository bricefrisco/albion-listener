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

// toInt converts any numeric type the Protocol18 deserializer may produce to int.
func toInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case byte:
		return int(val), true
	case int16:
		return int(val), true
	case uint16:
		return int(val), true
	case int32:
		return int(val), true
	case uint32:
		return int(val), true
	case int64:
		return int(val), true
	case uint64:
		return int(val), true
	}
	return 0, false
}

func resolveOperationCode(opCode byte, params map[byte]interface{}) int {
	if v, ok := params[253]; ok {
		if code, ok := toInt(v); ok {
			return code
		}
	}
	return int(opCode)
}

func resolveEventCode(code byte, params map[byte]interface{}) int {
	if v, ok := params[252]; ok {
		if c, ok := toInt(v); ok {
			return c
		}
	}
	return int(code)
}

func toRequestMessage(opCode byte, params map[byte]interface{}) *Message {
	code := resolveOperationCode(opCode, params)
	key := strconv.Itoa(code)
	name, ok := operationCodes[key]
	if !ok {
		fmt.Println("Unknown operation request code:", code)
		name = fmt.Sprintf("Unknown (%v)", code)
	}
	return &Message{
		Type: "OperationRequest",
		Name: name,
		Data: sanitizeValues(params),
	}
}

func toResponseMessage(opCode byte, params map[byte]interface{}) *Message {
	code := resolveOperationCode(opCode, params)
	key := strconv.Itoa(code)
	name, ok := operationCodes[key]
	if !ok {
		fmt.Println("Unknown operation response code:", code)
		name = fmt.Sprintf("Unknown (%v)", code)
	}
	return &Message{
		Type: "OperationResponse",
		Name: name,
		Data: sanitizeValues(params),
	}
}

func toEventMessage(code byte, params map[byte]interface{}) *Message {
	c := resolveEventCode(code, params)
	key := strconv.Itoa(c)
	name, ok := eventCodes[key]
	if !ok {
		fmt.Println("Unknown event code:", c)
		name = fmt.Sprintf("Unknown (%v)", c)
	}
	return &Message{
		Type: "Event",
		Name: name,
		Data: sanitizeValues(params),
	}
}
