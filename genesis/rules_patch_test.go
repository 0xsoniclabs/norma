package genesis

import (
	"encoding/json"
	"math/big"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/0xsoniclabs/sonic/opera"
)

func TestNetworkRulesPatch_HasSameFieldsAsOperaRules(t *testing.T) {
	assertFieldParity(t,
		reflect.TypeFor[opera.Rules](),
		reflect.TypeFor[NetworkRulesPatch](),
		"Name", "NetworkID")

	assertFieldParity(t,
		reflect.TypeFor[opera.DagRules](),
		reflect.TypeFor[DagPatch]())

	assertFieldParity(t,
		reflect.TypeFor[opera.EmitterRules](),
		reflect.TypeFor[EmitterPatch]())

	assertFieldParity(t,
		reflect.TypeFor[opera.EpochsRules](),
		reflect.TypeFor[EpochsPatch]())

	assertFieldParity(t,
		reflect.TypeFor[opera.BlocksRules](),
		reflect.TypeFor[BlocksPatch]())

	assertFieldParity(t,
		reflect.TypeFor[opera.EconomyRules](),
		reflect.TypeFor[EconomyPatch]())

	assertFieldParity(t,
		reflect.TypeFor[opera.GasRules](),
		reflect.TypeFor[GasPatch]())

	assertFieldParity(t,
		reflect.TypeFor[opera.GasPowerRules](),
		reflect.TypeFor[GasPowerPatch]())

	assertFieldParity(t,
		reflect.TypeFor[opera.Upgrades](),
		reflect.TypeFor[UpgradesPatch]())
}

func assertFieldParity(t *testing.T, srcType, patchType reflect.Type, excludedFromSrc ...string) {
	t.Helper()

	srcFields := collectFieldNames(srcType, asSet(excludedFromSrc))
	patchFields := collectFieldNames(patchType, nil)

	if !reflect.DeepEqual(srcFields, patchFields) {
		t.Fatalf(
			"field mismatch for %s vs %s\nsource fields: %v\npatch fields: %v",
			srcType.Name(),
			patchType.Name(),
			srcFields,
			patchFields,
		)
	}
}

func asSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

func collectFieldNames(typ reflect.Type, excluded map[string]bool) []string {
	fields := make([]string, 0, typ.NumField())
	for field := range typ.Fields() {
		name := field.Name
		if excluded != nil && excluded[name] {
			continue
		}
		fields = append(fields, name)
	}
	sort.Strings(fields)
	return fields
}

func TestDurationMarshalJSON_DecodesAsInt64(t *testing.T) {

	tests := []time.Duration{
		0,
		time.Millisecond,
		10 * time.Millisecond,
		time.Second,
		15 * time.Second,
	}

	for _, value := range tests {
		t.Run(value.String(), func(t *testing.T) {
			in := Duration(value)
			b, err := in.MarshalJSON()
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}

			var out int64
			if err := json.Unmarshal(b, &out); err != nil {
				t.Fatalf("decode failed: %v", err)
			}

			if got, want := out, int64(value); got != want {
				t.Fatalf("unexpected decoded duration: got %d, want %d", got, want)
			}
		})
	}
}

func TestBigIntValueMarshalJSON_DecodesAsBigInt(t *testing.T) {

	tests := []*big.Int{
		big.NewInt(0),
		big.NewInt(1),
		big.NewInt(-1),
		big.NewInt(1234567890),
		new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil).Sub(new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil), big.NewInt(1)), // 2^256 - 1
	}

	for _, value := range tests {
		t.Run(value.String(), func(t *testing.T) {
			in := BigIntValue(*value)

			b, err := in.MarshalJSON()
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}

			var out big.Int
			if err := json.Unmarshal(b, &out); err != nil {
				t.Fatalf("decode failed: %v", err)
			}

			if got, want := out.String(), value.String(); got != want {
				t.Fatalf("unexpected decoded big int: got %s, want %s", got, want)

			}
		})
	}
}
