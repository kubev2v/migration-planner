package duckdb_parser

// NetworkTopologyQuery returns shared interface, port-group and switch relationships.
func NetworkTopologyQuery() (string, error) {
	return NewBuilder().buildQuery("network_topology", networkTopologyQuery(), nil)
}

func networkTopologyQuery() string {
	return mustGetTemplate("network_topology")
}
