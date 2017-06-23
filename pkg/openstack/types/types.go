package types

type Network struct {
	Name      string
	Uid       string
	TenantID  string
	SegmentID int32
	Subnets   []*Subnet
	// Status of network
	// Valid value: Initializing, Active, Pending, Failed, Terminating
	Status string
}

// Subnet is a representaion of a subnet
type Subnet struct {
	Name       string
	Uid        string
	Cidr       string
	Gateway    string
	Tenantid   string
	Dnsservers []string
	Routes     []*Route
}

// Route is a representation of an advanced routing rule.
type Route struct {
	Name            string
	Nexthop         string
	DestinationCIDR string
}
