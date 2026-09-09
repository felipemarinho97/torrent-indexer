package engine

// ParseFunc transforms a raw extracted string value into a usable value.
// Examples: decoding an obfuscated magnet link, base64-decoding a field, etc.
// It is more general than a "decoder" — any field can reference a ParseFunc.
type ParseFunc func(string) (string, error)

// parseFuncRegistry holds named ParseFunc functions registered by callers.
var parseFuncRegistry = map[string]ParseFunc{}

// RegisterParseFunc registers a named ParseFunc so that YAML definitions can
// reference it via the `parse_function` field on a FieldSelectorItem.
// Subsequent calls with the same name overwrite the previous registration.
func RegisterParseFunc(name string, fn ParseFunc) {
	parseFuncRegistry[name] = fn
}
