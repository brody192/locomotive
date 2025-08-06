package reconstruct_loki

import (
	"github.com/tidwall/gjson"
)

func jsonBytesToAttributes(prefix string, json []byte) map[string]string {
	if !gjson.ValidBytes(json) {
		return nil
	}

	parsed := gjson.ParseBytes(json)
	result := make(map[string]string)

	flatten(prefix, parsed, result)

	return result
}

func jsonToAttributes(prefix string, json string) map[string]string {
	if !gjson.Valid(json) {
		return nil
	}

	parsed := gjson.Parse(json)
	result := make(map[string]string)

	flatten(prefix, parsed, result)

	return result
}

func flatten(prefix string, value gjson.Result, result map[string]string) {
	switch value.Type {
	case gjson.JSON:
		switch {
		case value.IsObject():
			value.ForEach(func(key, val gjson.Result) bool {
				newKey := key.String()

				if prefix != "" {
					newKey = prefix + "__" + key.String()
				}

				flatten(newKey, val, result)

				return true
			})
		case value.IsArray():
			value.ForEach(func(key, val gjson.Result) bool {
				index := key.String()

				newKey := prefix + "_" + index

				flatten(newKey, val, result)

				return true
			})
		}
	default:
		result[prefix] = value.String()
	}
}
