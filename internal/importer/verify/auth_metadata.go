package verify

// Purpose: compare auth metadata without rounding JSON numbers or expanding exponents.
// Inputs: source and independently read target metadata. Outputs: semantic equality.
// Constraints: absent metadata equals an empty object; strings remain distinct from numbers.
import (
	"bytes"
	"encoding/json"
	"math/big"
	"reflect"
	"strings"
)

func sameMetadata(want, got map[string]any) bool {
	w, we := canonicalMetadata(want)
	g, ge := canonicalMetadata(got)
	return we == nil && ge == nil && reflect.DeepEqual(w, g)
}

func canonicalMetadata(in map[string]any) (any, error) {
	if in == nil {
		in = map[string]any{}
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	return canonicalMetadataValue(value), nil
}

func canonicalMetadataValue(value any) any {
	switch v := value.(type) {
	case json.Number:
		return canonicalNumber(v.String())
	case map[string]any:
		for key, item := range v {
			v[key] = canonicalMetadataValue(item)
		}
	case []any:
		for i, item := range v {
			v[i] = canonicalMetadataValue(item)
		}
	}
	return value
}

// metadataNumber keeps normalized numbers distinct from JSON strings.
type metadataNumber struct{ digits, exponent string }

// canonicalNumber stores a coefficient and decimal exponent, using space proportional
// to the input. JSON decoding has already validated the number's lexical form.
func canonicalNumber(number string) metadataNumber {
	mantissa, exponent := number, "0"
	if i := strings.IndexAny(number, "eE"); i >= 0 {
		mantissa, exponent = number[:i], number[i+1:]
	}
	power, _ := new(big.Int).SetString(exponent, 10)
	negative := strings.HasPrefix(mantissa, "-")
	mantissa = strings.TrimPrefix(mantissa, "-")
	if point := strings.IndexByte(mantissa, '.'); point >= 0 {
		power.Sub(power, big.NewInt(int64(len(mantissa)-point-1)))
		mantissa = mantissa[:point] + mantissa[point+1:]
	}
	mantissa = strings.TrimLeft(mantissa, "0")
	if mantissa == "" {
		return metadataNumber{digits: "0", exponent: "0"}
	}
	digits := strings.TrimRight(mantissa, "0")
	power.Add(power, big.NewInt(int64(len(mantissa)-len(digits))))
	if negative {
		digits = "-" + digits
	}
	return metadataNumber{digits: digits, exponent: power.String()}
}
