package reconstruct_loki

import (
	"slices"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func applyJSONStringAttribute(stream []byte, pathPrefix, keyPrefix, jsonStr string, skip []string) []byte {
	if !gjson.Valid(jsonStr) {
		return stream
	}
	return flattenToStream(stream, pathPrefix, keyPrefix, gjson.Parse(jsonStr), skip)
}

func applyJSONBytesAttributes(stream []byte, pathPrefix string, jsonBytes []byte, skip []string) []byte {
	if !gjson.ValidBytes(jsonBytes) {
		return stream
	}
	return flattenToStream(stream, pathPrefix, "", gjson.ParseBytes(jsonBytes), skip)
}

func flattenToStream(stream []byte, pathPrefix, keyPrefix string, value gjson.Result, skip []string) []byte {
	switch value.Type {
	case gjson.JSON:
		switch {
		case value.IsObject():
			value.ForEach(func(key, val gjson.Result) bool {
				newKey := key.String()
				if keyPrefix != "" {
					newKey = keyPrefix + "__" + key.String()
				}
				stream = flattenToStream(stream, pathPrefix, newKey, val, skip)
				return true
			})
		case value.IsArray():
			value.ForEach(func(key, val gjson.Result) bool {
				stream = flattenToStream(stream, pathPrefix, keyPrefix+"_"+key.String(), val, skip)
				return true
			})
		}
	default:
		if len(skip) > 0 && slices.Contains(skip, keyPrefix) {
			return stream
		}
		stream, _ = sjson.SetBytes(stream, pathPrefix+"."+keyPrefix, value.String())
	}
	return stream
}
