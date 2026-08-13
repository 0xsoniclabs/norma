package genesis

import (
	"encoding/json"
	"math/big"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/0xsoniclabs/sonic/opera"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestNetworkRulesPatch_HasSameFieldsAsOperaRules(t *testing.T) {
	assertFieldParity(t,
		reflect.TypeOf(opera.Rules{}),
		reflect.TypeOf(NetworkRulesPatch{}),
		"Name", "NetworkID")

	assertFieldParity(t,
		reflect.TypeOf(opera.DagRules{}),
		reflect.TypeOf(DagPatch{}))

	assertFieldParity(t,
		reflect.TypeOf(opera.EmitterRules{}),
		reflect.TypeOf(EmitterPatch{}))

	assertFieldParity(t,
		reflect.TypeOf(opera.EpochsRules{}),
		reflect.TypeOf(EpochsPatch{}))

	assertFieldParity(t,
		reflect.TypeOf(opera.BlocksRules{}),
		reflect.TypeOf(BlocksPatch{}))

	assertFieldParity(t,
		reflect.TypeOf(opera.EconomyRules{}),
		reflect.TypeOf(EconomyPatch{}))

	assertFieldParity(t,
		reflect.TypeOf(opera.GasRules{}),
		reflect.TypeOf(GasPatch{}))

	assertFieldParity(t,
		reflect.TypeOf(opera.GasPowerRules{}),
		reflect.TypeOf(GasPowerPatch{}))

	assertFieldParity(t,
		reflect.TypeOf(opera.Upgrades{}),
		reflect.TypeOf(UpgradesPatch{}))
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
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
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

func TestNetworkRulesPatch_UnmarshalYAML_RejectsUnknownKeys(t *testing.T) {
	tests := map[string]struct {
		yaml string
		want []string
	}{
		"unknown top level key": {
			yaml: "Blocks: {}\nBloks:\n  MaxBlockGas: 1\n",
			want: []string{`line 2`, `unknown network rule "Bloks"`},
		},
		"unknown nested key": {
			yaml: "Blocks:\n  MaxBlockGass: 1\n",
			want: []string{`line 2`, `unknown network rule "Blocks.MaxBlockGass"`},
		},
		"unknown deeply nested key": {
			yaml: "Economy:\n  ShortGasPower:\n    AlocPerSec: 1\n",
			want: []string{`unknown network rule "Economy.ShortGasPower.AlocPerSec"`},
		},
		"dotted key": {
			// A dotted key is one key to yaml, naming no field of the schema.
			yaml: "Economy.Gas.MaxEventGas: 1\n",
			want: []string{`unknown network rule "Economy.Gas.MaxEventGas"`},
		},
		"dotted key with an unknown leaf": {
			yaml: "Blocks.ThisKeyDoesNotExist: 123\n",
			want: []string{`unknown network rule "Blocks.ThisKeyDoesNotExist"`},
		},
		"every unknown key is reported": {
			yaml: "Blocks:\n  Foo: 1\nUpgrades:\n  Bar: true\n",
			want: []string{
				`unknown network rule "Blocks.Foo"`,
				`unknown network rule "Upgrades.Bar"`,
			},
		},
		"unknown key inside a merged mapping": {
			yaml: "Blocks: &base\n  Nonsense: 1\nEconomy:\n  <<: *base\n",
			want: []string{`unknown network rule "Blocks.Nonsense"`},
		},
		"unknown key below a self decoding leaf": {
			yaml: "Economy:\n  MinGasPrice:\n    Nonsense: 1\n",
			// The value is not a mapping the schema describes, so the
			// error comes from the leaf's own unmarshalling.
			want: []string{`big integer must be a scalar value`},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var patch NetworkRulesPatch
			err := yaml.Unmarshal([]byte(test.yaml), &patch)
			require.Error(t, err, "unknown rule keys must be rejected")
			for _, want := range test.want {
				require.Contains(t, err.Error(), want)
			}
		})
	}
}

func TestNetworkRulesPatch_UnmarshalYAML_AcceptsEveryFieldOfTheSchema(t *testing.T) {
	// A patch naming every field of the schema, so that the key check cannot
	// reject a key any scenario may legitimately use.
	rules := opera.FakeNetRules(opera.GetSonicUpgrades())
	full, err := NewRulesPatchFromOperaRules(rules)
	require.NoError(t, err)

	data, err := yaml.Marshal(full)
	require.NoError(t, err)

	var patch NetworkRulesPatch
	require.NoError(t, yaml.Unmarshal(data, &patch),
		"a patch of the full rule set must be accepted, got the patch:\n%s",
		string(data))
	require.NoError(t, ValidateNetworkRulesPatch(patch))
}
