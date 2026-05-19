package parser

// setDefaults pass default values into the configuration of a scenario.
func (s *Scenario) setDefaults() {
	if s.NetworkRules.Genesis == nil {
		s.NetworkRules.Genesis = make(networkRules)
	}
	if s.NetworkRules.Genesis["MAX_EPOCH_DURATION"] == "" {
		s.NetworkRules.Genesis["MAX_EPOCH_DURATION"] = "15s"
	}
}
