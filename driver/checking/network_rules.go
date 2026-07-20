package checking

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/genesis"
	"gopkg.in/yaml.v3"
)

// defaultNetworkRulesTimeout bounds how long Check waits for a rule update to
// propagate to all nodes. Rules apply per node at the epoch seal, so a node
// can briefly lag the one observed by waitForEpoch.
const defaultNetworkRulesTimeout = 30 * time.Second

// networkRulesPollInterval is the delay between convergence polls. Var so tests
// can shorten it.
var networkRulesPollInterval = 500 * time.Millisecond

func init() {
	RegisterNetworkCheck("networkRules", func(net driver.Network, monitor *monitoring.Monitor) Checker {
		return &networkRulesChecker{net: net, timeout: defaultNetworkRulesTimeout}
	})
}

// networkRulesChecker validates that configured network-rules members are
// already applied on all non-failing nodes.
type networkRulesChecker struct {
	net          driver.Network
	rulesPatch   genesis.NetworkRulesPatch
	configureErr error
	// timeout bounds convergence polling. Zero means a single attempt.
	timeout time.Duration
}

// Configure returns a copy of the checker with an optional rules patch from config.
func (c *networkRulesChecker) Configure(config CheckerConfig) Checker {
	if config == nil {
		return c
	}

	configured := &networkRulesChecker{
		net:          c.net,
		rulesPatch:   c.rulesPatch,
		configureErr: c.configureErr,
		timeout:      c.timeout,
	}

	if d, exist := config["duration"]; exist {
		configured.timeout = time.Duration(d.(int64))
	}

	rules, exists := config["rules"]
	if !exists {
		return configured
	}

	patch, err := decodeNetworkRulesPatch(rules)
	if err != nil {
		configured.configureErr = err
		return configured
	}

	configured.rulesPatch = patch
	configured.configureErr = nil
	return configured
}

func (c *networkRulesChecker) Check(ctx context.Context) error {
	if c.configureErr != nil {
		return c.configureErr
	}
	if reflect.DeepEqual(c.rulesPatch, genesis.NetworkRulesPatch{}) {
		return nil
	}

	if c.timeout <= 0 {
		return c.checkOnce(ctx)
	}

	deadline := time.Now().Add(c.timeout)
	logged := false
	for {
		err := c.checkOnce(ctx)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		if !logged {
			slog.Info("network rules not yet applied on all nodes; waiting for convergence", "timeout", c.timeout)
			logged = true
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(networkRulesPollInterval):
		}
	}
}

func (c *networkRulesChecker) checkOnce(ctx context.Context) error {
	nodes := c.net.GetActiveNodes()
	slog.Info("checking applied network rules for nodes", "count", len(nodes))

	expectedFailures := make(map[string]struct{})
	gotFailures := make(map[string]struct{})
	for _, node := range nodes {
		if node.IsExpectedFailure() {
			expectedFailures[node.GetLabel()] = struct{}{}
		}

		rpcClient, err := node.DialRpc(ctx)
		if err != nil {
			if node.IsExpectedFailure() {
				gotFailures[node.GetLabel()] = struct{}{}
				continue
			}
			return fmt.Errorf("failed to dial node RPC %s: %w", node.GetLabel(), err)
		}

		rules, err := rpcClient.GetNetworkRules("latest")
		rpcClient.Close()
		if err != nil {
			if node.IsExpectedFailure() {
				gotFailures[node.GetLabel()] = struct{}{}
				continue
			}
			return fmt.Errorf("failed to fetch network rules on node %s: %w", node.GetLabel(), err)
		}

		patched := rules
		if err := genesis.ApplyNetworkRulesPatch(&patched, c.rulesPatch); err != nil {
			return fmt.Errorf("failed to apply configured network rules patch: %w", err)
		}
		if !reflect.DeepEqual(rules, patched) {
			if node.IsExpectedFailure() {
				gotFailures[node.GetLabel()] = struct{}{}
				continue
			}
			details := explainRulesMismatch(rules, patched)
			return fmt.Errorf("applied network rules mismatch on node %s: %s", node.GetLabel(), details)
		}
	}

	if got, want := gotFailures, expectedFailures; !maps.Equal(got, want) {
		return fmt.Errorf("unexpected failure set to validate network rules, got %v, want %v", got, want)
	}

	return nil
}

func decodeNetworkRulesPatch(raw any) (genesis.NetworkRulesPatch, error) {
	body, err := yaml.Marshal(raw)
	if err != nil {
		return genesis.NetworkRulesPatch{}, fmt.Errorf("failed to parse rules config: %w", err)
	}

	var patch genesis.NetworkRulesPatch
	if err := yaml.Unmarshal(body, &patch); err != nil {
		return genesis.NetworkRulesPatch{}, fmt.Errorf("failed to decode rules patch: %w", err)
	}

	if err := genesis.ValidateNetworkRulesPatch(patch); err != nil {
		return genesis.NetworkRulesPatch{}, fmt.Errorf("invalid network rules patch: %w", err)
	}

	return patch, nil
}

func explainRulesMismatch(actual, expected any) string {
	diffs := make([]string, 0)
	collectRuleDiffs("", reflect.ValueOf(actual), reflect.ValueOf(expected), &diffs)
	if len(diffs) == 0 {
		return "unable to extract differing fields"
	}
	sort.Strings(diffs)
	return strings.Join(diffs, "; ")
}

func collectRuleDiffs(path string, actual, expected reflect.Value, out *[]string) {
	if !actual.IsValid() && !expected.IsValid() {
		return
	}
	if !actual.IsValid() || !expected.IsValid() {
		*out = append(*out, fmt.Sprintf("%s: got=%s want=%s", pathOrRoot(path), valueToString(actual), valueToString(expected)))
		return
	}

	if actual.Type() != expected.Type() {
		*out = append(*out, fmt.Sprintf("%s: got=%s (%s) want=%s (%s)", pathOrRoot(path), valueToString(actual), actual.Type(), valueToString(expected), expected.Type()))
		return
	}

	switch actual.Kind() {
	case reflect.Pointer, reflect.Interface:
		if actual.IsNil() && expected.IsNil() {
			return
		}
		if actual.IsNil() || expected.IsNil() {
			*out = append(*out, fmt.Sprintf("%s: got=%s want=%s", pathOrRoot(path), valueToString(actual), valueToString(expected)))
			return
		}
		collectRuleDiffs(path, actual.Elem(), expected.Elem(), out)
		return
	case reflect.Struct:
		for i := 0; i < actual.NumField(); i++ {
			field := actual.Type().Field(i)
			if !field.IsExported() {
				continue
			}

			nextPath := field.Name
			if path != "" {
				nextPath = path + "." + field.Name
			}
			collectRuleDiffs(nextPath, actual.Field(i), expected.Field(i), out)
		}
		return
	case reflect.Map:
		if actual.IsNil() && expected.IsNil() {
			return
		}
		if actual.IsNil() || expected.IsNil() {
			*out = append(*out, fmt.Sprintf("%s: got=%s want=%s", pathOrRoot(path), valueToString(actual), valueToString(expected)))
			return
		}

		keys := make(map[string]reflect.Value, actual.Len()+expected.Len())
		for _, key := range actual.MapKeys() {
			keys[fmt.Sprintf("%v", key.Interface())] = key
		}
		for _, key := range expected.MapKeys() {
			keys[fmt.Sprintf("%v", key.Interface())] = key
		}

		ordered := make([]string, 0, len(keys))
		for key := range keys {
			ordered = append(ordered, key)
		}
		sort.Strings(ordered)

		for _, key := range ordered {
			mapKey := keys[key]
			nextPath := key
			if path != "" {
				nextPath = path + "." + key
			}
			collectRuleDiffs(nextPath, actual.MapIndex(mapKey), expected.MapIndex(mapKey), out)
		}
		return
	case reflect.Slice, reflect.Array:
		if actual.Len() != expected.Len() {
			*out = append(*out, fmt.Sprintf("%s: got=len(%d) want=len(%d)", pathOrRoot(path), actual.Len(), expected.Len()))
			return
		}
		for i := 0; i < actual.Len(); i++ {
			nextPath := fmt.Sprintf("%s[%d]", pathOrRoot(path), i)
			if path != "" {
				nextPath = fmt.Sprintf("%s[%d]", path, i)
			}
			collectRuleDiffs(nextPath, actual.Index(i), expected.Index(i), out)
		}
		return
	}

	if reflect.DeepEqual(actual.Interface(), expected.Interface()) {
		return
	}

	*out = append(*out, fmt.Sprintf("%s: got=%s want=%s", pathOrRoot(path), valueToString(actual), valueToString(expected)))
}

func pathOrRoot(path string) string {
	if path == "" {
		return "<root>"
	}
	return path
}

func valueToString(value reflect.Value) string {
	if !value.IsValid() {
		return "<missing>"
	}
	if (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface || value.Kind() == reflect.Map || value.Kind() == reflect.Slice) && value.IsNil() {
		return "<nil>"
	}
	if value.CanInterface() {
		return fmt.Sprintf("%v", value.Interface())
	}
	return value.Type().String()
}
